// Package addonkit is the openccu-lite side of ccu-addon-kit for an addon written in Go: the
// platform rule of the CCU addon handbook, the session check against openccu-lite's
// /api/auth/v1/state, the CSRF rules the kit's Tcl side applies (the same X-CCU-Addon-Request
// header), the addon's own API token, loopback listening, theme and language hand-over, and the
// status line of the service convention. Nothing in it is a frame: on openccu-lite the system's
// own Addons page does service, logs and memory.
package addonkit

import (
	"bufio"
	"os"
	"strings"
)

// Kind is the firmware an addon runs on.
type Kind string

const (
	// OpenccuLite is openccu-lite: a LITE= line in /VERSION or an executable /usr/bin/occulited.
	OpenccuLite Kind = "openccu-lite"
	// OpenCCU is OpenCCU (formerly RaspberryMatic): neither marker, no CCU3 install hook.
	OpenCCU Kind = "openccu"
	// CCU3 is eQ-3's CCU3 firmware: /etc/init.d/S00InstallAddon exists.
	CCU3 Kind = "ccu3"
)

// Detector holds the paths the platform rule reads. The zero value is not usable; use
// DefaultDetector or a copy of it with paths changed for a test.
type Detector struct {
	VersionFile string // /VERSION
	Occulited   string // /usr/bin/occulited
	CCU3Hook    string // /etc/init.d/S00InstallAddon
}

// DefaultDetector reads the real paths.
var DefaultDetector = Detector{VersionFile: "/VERSION", Occulited: "/usr/bin/occulited", CCU3Hook: "/etc/init.d/S00InstallAddon"}

// Platform applies the handbook's one rule, evaluated on every call and never cached: a system is
// openccu-lite when /VERSION has a LITE= line or /usr/bin/occulited is an executable file.
// VARIANT=lite is written too but is not the marker. A CCU3 is told from OpenCCU by the install
// hook S00InstallAddon (chapter 02).
func Platform() Kind { return DefaultDetector.Platform() }

// IsOpenccuLite is Platform() == OpenccuLite.
func IsOpenccuLite() bool { return Platform() == OpenccuLite }

// Platform applies the rule with this detector's paths.
func (d Detector) Platform() Kind {
	if d.liteVersion() || d.executable(d.Occulited) {
		return OpenccuLite
	}
	if _, err := os.Stat(d.CCU3Hook); err == nil {
		return CCU3
	}
	return OpenCCU
}

func (d Detector) liteVersion() bool {
	f, err := os.Open(d.VersionFile)
	if err != nil {
		return false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if strings.HasPrefix(sc.Text(), "LITE=") {
			return true
		}
	}
	return false
}

func (d Detector) executable(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.Mode().IsRegular() && st.Mode()&0o111 != 0
}
