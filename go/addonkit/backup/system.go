package backup

import (
	"strconv"
	"strings"

	"github.com/hobbyquaker/ccu-addon-kit/go/addonkit"
)

// HookSince is the first openccu-lite that runs the manifest's backup hook (and the restore program).
const HookSince = "1.3.0-beta.0"

// SystemRunsHook says whether this system runs the manifest's backup hook before every backup:
// openccu-lite from HookSince. On a CCU3, OpenCCU and an older openccu-lite the addon writes its
// snapshot into the staging itself (once a day) and keeps it there.
func SystemRunsHook() bool { return RunsHook(addonkit.DefaultDetector) }

// RunsHook is SystemRunsHook with a detector's paths. An openccu-lite whose version cannot be read
// or compared counts as running the hook: every openccu-lite image since the hook writes a semantic
// version, and an unknown one is newer rather than older.
func RunsHook(d addonkit.Detector) bool {
	if d.Platform() != addonkit.OpenccuLite {
		return false
	}
	v := d.LiteVersion()
	c, ok := compare(v, HookSince)
	return !ok || c >= 0
}

// compare orders two semantic versions (MAJOR.MINOR.PATCH[-pre][+build]) by semver's rules; ok is
// false when either is of another shape.
func compare(a, b string) (int, bool) {
	an, ap, ok1 := parse(a)
	bn, bp, ok2 := parse(b)
	if !ok1 || !ok2 {
		return 0, false
	}
	for i := 0; i < 3; i++ {
		if an[i] != bn[i] {
			return sign(an[i] - bn[i]), true
		}
	}
	switch {
	case len(ap) == 0 && len(bp) == 0:
		return 0, true
	case len(ap) == 0:
		return 1, true
	case len(bp) == 0:
		return -1, true
	}
	for i := 0; i < len(ap) && i < len(bp); i++ {
		x, xe := strconv.Atoi(ap[i])
		y, ye := strconv.Atoi(bp[i])
		switch {
		case xe == nil && ye == nil:
			if x != y {
				return sign(x - y), true
			}
		case xe == nil:
			return -1, true // numeric identifiers sort before alphanumeric ones
		case ye == nil:
			return 1, true
		default:
			if c := strings.Compare(ap[i], bp[i]); c != 0 {
				return c, true
			}
		}
	}
	return sign(len(ap) - len(bp)), true
}

func parse(v string) ([3]int, []string, bool) {
	var n [3]int
	v, _, _ = strings.Cut(v, "+")
	num, pre, hasPre := strings.Cut(v, "-")
	parts := strings.Split(num, ".")
	if len(parts) != 3 {
		return n, nil, false
	}
	for i, p := range parts {
		x, err := strconv.Atoi(p)
		if err != nil || x < 0 {
			return n, nil, false
		}
		n[i] = x
	}
	if !hasPre {
		return n, nil, true
	}
	if pre == "" {
		return n, nil, false
	}
	return n, strings.Split(pre, "."), true
}

func sign(x int) int {
	switch {
	case x < 0:
		return -1
	case x > 0:
		return 1
	}
	return 0
}
