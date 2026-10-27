#!/bin/sh
#
#   Runs the frame tests' stub server in the kit's Tcl 8.6 test image (ccu-addon-kit, MIT):
#     sh test/frame/stub.sh <port>
#   Playwright starts it and stops it with TERM; the container goes with it (and
#   playwright-teardown.mjs removes it should the script have been killed harder).
#
cd "$(dirname "$0")/../.." || exit 1
PORT=${1:-18080}
NAME=kit-pw-$PORT
docker rm -f "$NAME" >/dev/null 2>&1
docker run -d --rm --init --name "$NAME" -p "127.0.0.1:$PORT:8080" -v "$PWD:/kit:ro" -w /kit \
    ccu-addon-kit-tcl86 node test/frame/stub-server.mjs 8080 >/dev/null || exit 1
trap 'docker rm -f "$NAME" >/dev/null 2>&1; exit 0' INT TERM EXIT
docker logs -f "$NAME" &
wait
