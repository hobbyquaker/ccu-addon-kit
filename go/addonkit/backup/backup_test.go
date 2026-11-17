package backup

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeDriver stands in for SQLite: VACUUM INTO writes the "database" content to the target file,
// integrity_check answers what the test sets. The real statements run against modernc.org/sqlite
// in the addons' own tests (ccu-addon-battery, ccu-addon-history).
type fakeDriver struct {
	mu        sync.Mutex
	content   string
	vacuumErr error
	integrity string
	stmts     []string
}

func (d *fakeDriver) Open(string) (driver.Conn, error) { return &fakeConn{d: d}, nil }

type fakeConn struct{ d *fakeDriver }

func (c *fakeConn) Prepare(q string) (driver.Stmt, error) { return &fakeStmt{c: c, q: q}, nil }
func (c *fakeConn) Close() error                          { return nil }
func (c *fakeConn) Begin() (driver.Tx, error)             { return nil, errors.New("no transactions") }

type fakeStmt struct {
	c *fakeConn
	q string
}

func (s *fakeStmt) Close() error  { return nil }
func (s *fakeStmt) NumInput() int { return strings.Count(s.q, "?") }

func (s *fakeStmt) Exec(args []driver.Value) (driver.Result, error) {
	d := s.c.d
	d.mu.Lock()
	defer d.mu.Unlock()
	d.stmts = append(d.stmts, s.q)
	if strings.HasPrefix(s.q, "VACUUM INTO") {
		if d.vacuumErr != nil {
			return nil, d.vacuumErr
		}
		target := args[0].(string)
		if _, err := os.Stat(target); err == nil {
			return nil, errors.New("output file already exists")
		}
		return driver.RowsAffected(0), os.WriteFile(target, []byte(d.content), 0o600)
	}
	return driver.RowsAffected(0), nil
}

func (s *fakeStmt) Query(args []driver.Value) (driver.Rows, error) {
	d := s.c.d
	d.mu.Lock()
	defer d.mu.Unlock()
	d.stmts = append(d.stmts, s.q)
	return &fakeRows{vals: []string{d.integrity}}, nil
}

type fakeRows struct {
	vals []string
	i    int
}

func (r *fakeRows) Columns() []string { return []string{"integrity_check"} }
func (r *fakeRows) Close() error      { return nil }
func (r *fakeRows) Next(dest []driver.Value) error {
	if r.i >= len(r.vals) {
		return io.EOF
	}
	dest[0] = r.vals[r.i]
	r.i++
	return nil
}

var registerOnce sync.Once
var current *fakeDriver

type switchDriver struct{}

func (switchDriver) Open(n string) (driver.Conn, error) { return current.Open(n) }

func open(t *testing.T, d *fakeDriver) *sql.DB {
	t.Helper()
	registerOnce.Do(func() { sql.Register("kitfake", switchDriver{}) })
	current = d
	db, err := sql.Open("kitfake", "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func setup(t *testing.T) (SQLite, string) {
	dir := t.TempDir()
	s := SQLite{Staging: filepath.Join(dir, "backup"), Name: "x.db", DataDir: filepath.Join(dir, "var")}
	return s, dir
}

func TestWrite(t *testing.T) {
	s, _ := setup(t)
	d := &fakeDriver{content: "v1", integrity: "ok"}
	db := open(t, d)
	if err := s.Write(context.Background(), db, ByAddon); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(s.Path()); string(b) != "v1" {
		t.Fatalf("snapshot %q", b)
	}
	if !s.Tagged() {
		t.Fatal("a written snapshot tags the data directory")
	}
	if age, by, ok := s.Own(); !ok || by != ByAddon || age > time.Minute {
		t.Fatalf("the snapshot is this system's own: %v %v %v", age, by, ok)
	}
	if _, err := os.Stat(s.Path() + ".tmp"); err == nil {
		t.Fatal("the temporary file is gone")
	}
	want := []string{"VACUUM INTO ?", "ATTACH DATABASE ? AS kit_snapshot", "PRAGMA kit_snapshot.integrity_check(1)", "DETACH DATABASE kit_snapshot"}
	if strings.Join(d.stmts, "|") != strings.Join(want, "|") {
		t.Fatalf("statements %q", d.stmts)
	}
	// a second snapshot replaces the first (VACUUM INTO refuses an existing target: the
	// temporary file is new each time)
	d.content = "v2"
	if err := s.Write(context.Background(), db, ByAddon); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(s.Path()); string(b) != "v2" {
		t.Fatalf("snapshot %q", b)
	}
	// a leftover temporary file of a killed run does not stop the next
	os.WriteFile(s.Path()+".tmp", []byte("torn"), 0o600)
	d.content = "v3"
	if err := s.Write(context.Background(), db, ByAddon); err != nil {
		t.Fatal(err)
	}
}

func TestWriteFailures(t *testing.T) {
	for name, d := range map[string]*fakeDriver{
		"vacuum":    {content: "new", vacuumErr: errors.New("disk I/O error"), integrity: "ok"},
		"integrity": {content: "new", integrity: "*** in database main ***\nPage 3 is never used"},
	} {
		t.Run(name, func(t *testing.T) {
			s, _ := setup(t)
			os.MkdirAll(s.Staging, 0o750)
			os.WriteFile(s.Path(), []byte("old"), 0o600)
			s.Tag()
			if err := s.Write(context.Background(), open(t, d), ByAddon); err == nil {
				t.Fatal("no error")
			}
			if b, _ := os.ReadFile(s.Path()); string(b) != "old" {
				t.Fatalf("the previous snapshot must stay: %q", b)
			}
			if s.Tagged() {
				t.Fatal("a failed snapshot untags the data directory: the backup takes the live file")
			}
			if _, err := os.Stat(s.Path() + ".tmp"); err == nil {
				t.Fatal("the temporary file is removed")
			}
		})
	}
	if err := (SQLite{}).Write(context.Background(), nil, ByAddon); err == nil {
		t.Fatal("an empty SQLite is refused")
	}
}

func TestRestore(t *testing.T) {
	ctx := context.Background()
	t.Run("nothing", func(t *testing.T) {
		s, dir := setup(t)
		if r, err := s.Restore(filepath.Join(dir, "var", "x.db")); r != Nothing || err != nil {
			t.Fatal(r, err)
		}
	})
	t.Run("own", func(t *testing.T) {
		s, dir := setup(t)
		live := filepath.Join(dir, "var", "x.db")
		if err := s.Write(ctx, open(t, &fakeDriver{content: "snap", integrity: "ok"}), ByHook); err != nil {
			t.Fatal(err)
		}
		os.Remove(live)
		if _, by, ok := s.Own(); !ok || by != ByHook {
			t.Fatal("own, by the hook")
		}
		if r, err := s.Restore(live); r != Mine || err != nil {
			t.Fatal(r, err)
		}
		if _, err := os.Stat(s.Path()); err != nil {
			t.Fatal("the system's own snapshot stays: a backup may be taking it")
		}
	})
	t.Run("missing live", func(t *testing.T) {
		s, dir := setup(t)
		live := filepath.Join(dir, "stick", "folder", "x.db")
		restored(t, s, "snap", time.Now())
		if _, _, ok := s.Own(); ok {
			t.Fatal("a restored snapshot is not this system's own")
		}
		if r, err := s.Restore(live); r != Restored || err != nil {
			t.Fatal(r, err)
		}
		if b, _ := os.ReadFile(live); string(b) != "snap" {
			t.Fatalf("live %q", b)
		}
		if _, err := os.Stat(s.Path()); err != nil {
			t.Fatal("Restore leaves the snapshot; Clear removes it")
		}
		if err := s.Clear(); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(s.Path()); err == nil {
			t.Fatal("cleared")
		}
		if err := s.Clear(); err != nil {
			t.Fatal("a second Clear is fine", err)
		}
	})
	t.Run("older live", func(t *testing.T) {
		s, dir := setup(t)
		live := filepath.Join(dir, "var", "x.db")
		os.MkdirAll(filepath.Dir(live), 0o750)
		for _, f := range []string{live, live + "-wal", live + "-shm"} {
			os.WriteFile(f, []byte("old"), 0o600)
			os.Chtimes(f, time.Now().Add(-2*time.Hour), time.Now().Add(-2*time.Hour))
		}
		restored(t, s, "snap", time.Now().Add(-time.Hour))
		if r, err := s.Restore(live); r != Restored || err != nil {
			t.Fatal(r, err)
		}
		if b, _ := os.ReadFile(live); string(b) != "snap" {
			t.Fatalf("live %q", b)
		}
		for _, f := range []string{live + "-wal", live + "-shm"} {
			if _, err := os.Stat(f); err == nil {
				t.Fatalf("%s must go with the old file", f)
			}
		}
	})
	t.Run("newer live", func(t *testing.T) {
		s, dir := setup(t)
		live := filepath.Join(dir, "var", "x.db")
		os.MkdirAll(filepath.Dir(live), 0o750)
		os.WriteFile(live, []byte("live"), 0o600)
		os.Chtimes(live, time.Now().Add(-3*time.Hour), time.Now().Add(-3*time.Hour))
		os.WriteFile(live+"-wal", []byte("wal"), 0o600) // written now: the live data is newer
		restored(t, s, "snap", time.Now().Add(-time.Hour))
		if r, err := s.Restore(live); r != Kept || err != nil {
			t.Fatal(r, err)
		}
		if b, _ := os.ReadFile(live); string(b) != "live" {
			t.Fatalf("live %q", b)
		}
	})
}

// restored puts a snapshot into the staging as a restore would: not this system's own.
func restored(t *testing.T, s SQLite, content string, mtime time.Time) {
	t.Helper()
	os.MkdirAll(s.Staging, 0o750)
	if err := os.WriteFile(s.Path(), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	os.Chtimes(s.Path(), mtime, mtime)
}

func TestTag(t *testing.T) {
	s, _ := setup(t)
	if s.Tagged() {
		t.Fatal("untagged at first")
	}
	if err := s.Tag(); err != nil || !s.Tagged() {
		t.Fatal(err)
	}
	s.Untag()
	if s.Tagged() {
		t.Fatal("untagged")
	}
	if err := (SQLite{}).Tag(); err != nil {
		t.Fatal(err)
	}
}

func TestSystemRunsHook(t *testing.T) {
	s, _ := setup(t)
	t.Setenv("INVOCATION_ID", "")
	if err := s.MarkRestoreRun(); err != nil {
		t.Fatal(err)
	}
	if s.SystemRunsHook() {
		t.Fatal("outside systemd: no")
	}
	t.Setenv("INVOCATION_ID", "aaaa")
	if s.SystemRunsHook() {
		t.Fatal("no mark: the restore program did not run")
	}
	if err := s.MarkRestoreRun(); err != nil {
		t.Fatal(err)
	}
	if !s.SystemRunsHook() {
		t.Fatal("the restore program ran in this start")
	}
	t.Setenv("INVOCATION_ID", "bbbb")
	if s.SystemRunsHook() {
		t.Fatal("a mark of an earlier start does not count")
	}
}
