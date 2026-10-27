#!/bin/sh
#
#   The local gate (ccu-addon-kit, MIT): every CI job, in the test images, logs into one
#   directory and each exit code checked. Needs docker and node.
#     sh test/gate.sh [<log dir>]
#
cd "$(dirname "$0")/.." || exit 1
ROOT=$PWD
LOGS=${1:-${TMPDIR:-/tmp}/ccu-addon-kit-gate}
mkdir -p "$LOGS"
failed=0
step() {
    name=$1; shift
    if "$@" > "$LOGS/$name.log" 2>&1; then echo "ok   $name"; else echo "FAIL $name (log: $LOGS/$name.log)"; failed=1; fi
}
step lint-tcl82 node test/tcl/lint-tcl82.mjs
step cli node --test 'test/cli/*.test.mjs'
step image-tcl86 docker build -q -f test/docker/tcl86.Dockerfile -t ccu-addon-kit-tcl86 test/
step image-tcl82 docker build -q -f test/docker/tcl82.Dockerfile -t ccu-addon-kit-tcl82 test/
step image-e2e docker build -q -f test/e2e/Dockerfile -t ccu-addon-kit-e2e test/
step sh docker run --rm -v "$ROOT:/kit" -w /kit ccu-addon-kit-tcl86 sh test/sh/run.sh
step tcl86 docker run --rm -v "$ROOT:/kit" -w /kit ccu-addon-kit-tcl86 node --test 'test/tcl/*.test.mjs'
step tcl82 docker run --rm -v "$ROOT:/kit" -w /kit ccu-addon-kit-tcl82 node --test 'test/tcl/*.test.mjs'
step e2e node --test test/e2e/e2e.test.mjs
[ $failed -eq 0 ] && echo "gate: passed" || echo "gate: FAILED"
exit $failed
