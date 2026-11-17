// Package backup is an addon's part in the system backup for a SQLite database: the snapshot the
// manifest's backup hook writes into the staging directory, the restore that puts a snapshot back
// where the database lives, and the .nobackup tag on the live database's directory.
//
// On openccu-lite from 1.3.0-beta.0 the system runs the manifest's hook before every backup's tar,
// takes the staging and empties it afterwards; after a restore the unit's restore program finds the
// snapshot there (occulited's docs/manifest-format.md, "backup"). A CCU3, OpenCCU and an older
// openccu-lite run no hook: there the addon writes the snapshot into the staging itself, once a day,
// so the firmware's own backup carries a copy at most a day old (SystemRunsHook tells them apart).
//
// The package needs no SQLite driver: it works on the addon's own *sql.DB. The snapshot is SQLite's
// VACUUM INTO - a consistent copy made inside one read transaction, safe while the addon writes -
// checked with PRAGMA integrity_check before it replaces the previous one.
package backup

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// NoBackup is the tag file the system backup (createBackup.sh, tar --exclude-tag) honours.
const NoBackup = ".nobackup"

// SQLite is one database's snapshot.
type SQLite struct {
	// Staging is the manifest's backup.staging as an absolute path (/usr/local/addons/<id>/<staging>).
	Staging string
	// Name is the snapshot's file name in Staging ("battery.db").
	Name string
	// DataDir is the addon's own data directory in its tree, the database's default location: it is
	// tagged .nobackup while a snapshot covers the database, so the tar does not also take the live
	// file (possibly torn while the addon writes), and untagged when a snapshot fails, so the backup
	// then takes the live file as it lies rather than nothing. The marker lives here too. Never the
	// staging or a directory that holds it.
	DataDir string
}

// Path is the snapshot's path.
func (s SQLite) Path() string { return filepath.Join(s.Staging, s.Name) }

func (s SQLite) markerPath() string { return filepath.Join(s.DataDir, ".snapshot-"+s.Name+".json") }

// marker names the last snapshot this system wrote, so a snapshot in the staging can be told from
// one a restore brought back: Restore leaves the system's own snapshot alone (a backup may be
// taking it right now).
type marker struct {
	Size  int64 `json:"size"`
	MTime int64 `json:"mtime_ns"`
}

// Write makes the snapshot: VACUUM INTO a temporary file in Staging, PRAGMA integrity_check on it,
// fsync, then rename over the previous snapshot. On success DataDir is tagged .nobackup; on any
// failure the temporary file is removed, the previous snapshot is left as it was, and DataDir's tag
// is removed, so the backup takes the live file instead.
func (s SQLite) Write(ctx context.Context, db *sql.DB) (err error) {
	if s.Staging == "" || s.Name == "" || s.DataDir == "" {
		return errors.New("backup: Staging, Name and DataDir are required")
	}
	defer func() {
		if err != nil {
			s.Untag()
		}
	}()
	if err := os.MkdirAll(s.Staging, 0o750); err != nil {
		return err
	}
	tmp := s.Path() + ".tmp"
	os.Remove(tmp)
	defer os.Remove(tmp) // a no-op after the rename
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "VACUUM INTO ?", tmp); err != nil {
		return fmt.Errorf("backup: VACUUM INTO: %w", err)
	}
	if err := check(ctx, conn, tmp); err != nil {
		return err
	}
	if err := syncFile(tmp); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.Path()); err != nil {
		return err
	}
	syncDir(s.Staging)
	st, err := os.Stat(s.Path())
	if err != nil {
		return err
	}
	b, _ := json.Marshal(marker{Size: st.Size(), MTime: st.ModTime().UnixNano()})
	if err := os.MkdirAll(s.DataDir, 0o750); err != nil {
		return err
	}
	if err := writeFile(s.markerPath(), b); err != nil {
		return err
	}
	return s.Tag()
}

// check runs PRAGMA integrity_check on the copy, attached to the addon's own connection.
func check(ctx context.Context, conn *sql.Conn, file string) error {
	if _, err := conn.ExecContext(ctx, "ATTACH DATABASE ? AS kit_snapshot", file); err != nil {
		return fmt.Errorf("backup: attaching the copy: %w", err)
	}
	defer conn.ExecContext(context.Background(), "DETACH DATABASE kit_snapshot")
	var res string
	if err := conn.QueryRowContext(ctx, "PRAGMA kit_snapshot.integrity_check(1)").Scan(&res); err != nil {
		return fmt.Errorf("backup: integrity_check: %w", err)
	}
	if res != "ok" {
		return fmt.Errorf("backup: the copy is not intact: %s", res)
	}
	return nil
}

// Tag puts .nobackup into DataDir.
func (s SQLite) Tag() error {
	if s.DataDir == "" {
		return nil
	}
	if err := os.MkdirAll(s.DataDir, 0o750); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(s.DataDir, NoBackup), os.O_CREATE|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}
	return f.Close()
}

// Untag removes DataDir's .nobackup.
func (s SQLite) Untag() {
	if s.DataDir != "" {
		os.Remove(filepath.Join(s.DataDir, NoBackup))
	}
}

// Tagged says whether DataDir carries .nobackup.
func (s SQLite) Tagged() bool {
	_, err := os.Stat(filepath.Join(s.DataDir, NoBackup))
	return err == nil
}

// Age is how old the snapshot in Staging is, and whether there is one this system wrote itself.
func (s SQLite) Age() (time.Duration, bool) {
	st, err := os.Stat(s.Path())
	if err != nil || !s.own(st) {
		return 0, false
	}
	return time.Since(st.ModTime()), true
}

func (s SQLite) own(st os.FileInfo) bool {
	b, err := os.ReadFile(s.markerPath())
	if err != nil {
		return false
	}
	var m marker
	return json.Unmarshal(b, &m) == nil && m.Size == st.Size() && m.MTime == st.ModTime().UnixNano()
}

// Result is what Restore did.
type Result string

const (
	// Nothing: no snapshot in the staging - every ordinary start.
	Nothing Result = "nothing"
	// Own: the snapshot is the one this system wrote last (a backup may be taking it): left alone.
	Own Result = "own"
	// Kept: the live database is newer than the restored snapshot and stays.
	Kept Result = "kept"
	// Restored: the snapshot was copied to the live database's place.
	Restored Result = "restored"
)

// Restore puts a restored snapshot back where the database lives, as the system's contract says:
// the live file, when it exists and is newer than the snapshot (its -wal counts), stays; otherwise
// the snapshot is copied there - through a temporary file in the same directory, the old -wal and
// -shm removed, then renamed into place. The snapshot itself stays in the staging; Clear removes it
// (openccu-lite, after a restore) - a system that runs no hook keeps it as its daily copy.
// The caller stops nothing: Restore runs before the database is opened.
func (s SQLite) Restore(live string) (Result, error) {
	st, err := os.Stat(s.Path())
	if err != nil {
		return Nothing, nil
	}
	if s.own(st) {
		return Own, nil
	}
	if lt, ok := liveTime(live); ok && lt.After(st.ModTime()) {
		return Kept, nil
	}
	if err := os.MkdirAll(filepath.Dir(live), 0o750); err != nil {
		return Nothing, err
	}
	tmp := live + ".restoring"
	if err := copyFile(s.Path(), tmp); err != nil {
		os.Remove(tmp)
		return Nothing, err
	}
	os.Remove(live + "-wal")
	os.Remove(live + "-shm")
	if err := os.Rename(tmp, live); err != nil {
		os.Remove(tmp)
		return Nothing, err
	}
	syncDir(filepath.Dir(live))
	return Restored, nil
}

// Clear empties the snapshot (and a leftover temporary file) from the staging.
func (s SQLite) Clear() error {
	os.Remove(s.Path() + ".tmp")
	if err := os.Remove(s.Path()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// liveTime is the newest modification time of the database and its -wal.
func liveTime(live string) (time.Time, bool) {
	st, err := os.Stat(live)
	if err != nil {
		return time.Time{}, false
	}
	t := st.ModTime()
	if w, err := os.Stat(live + "-wal"); err == nil && w.ModTime().After(t) {
		t = w.ModTime()
	}
	return t, true
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func writeFile(path string, b []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o640); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

func syncFile(path string) error {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

func syncDir(dir string) {
	if f, err := os.Open(dir); err == nil {
		f.Sync()
		f.Close()
	}
}
