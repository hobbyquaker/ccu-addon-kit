#!/bin/sh
#
#   Optional helpers for an rc.d script that follows the ccu-addon-kit service convention v1
#   (MIT). Source it from the script: . $ADDON_DIR/lib/kit/rc-helpers.sh
#   POSIX sh, busybox 1.29 compatible.
#
#   Descendants <pid>     <pid> and every process below it, one per line (one awk pass over
#                         /proc/*/stat; the comm field may hold spaces and is cut off first)
#   RssTreeKb <pid>       VmRSS summed over Descendants, in KiB
#   PidsJson <pid>        the Descendants as a JSON array, for the status line's "pids"
#   JsonStr <text>        JSON string content: control characters dropped, \ and " escaped
#

Descendants() {
    for f in /proc/[0-9]*/stat; do
        sed -e 's/^\([0-9]*\) (.*) [A-Za-z] \([0-9]*\) .*$/\1 \2/' "$f" 2>/dev/null
    done | awk -v root="$1" '
        { parent[$1] = $2; pids[NR] = $1 }
        END {
            keep[root] = 1; print root
            changed = 1
            while (changed) {
                changed = 0
                for (i = 1; i <= NR; i++) {
                    p = pids[i]
                    if (!(p in keep) && (parent[p] in keep)) { keep[p] = 1; print p; changed = 1 }
                }
            }
        }'
}

RssTreeKb() {
    total=0
    for p in $(Descendants "$1"); do
        kb=$(awk '/^VmRSS:/ {print $2}' "/proc/$p/status" 2>/dev/null)
        total=$((total + ${kb:-0}))
    done
    echo $total
}

PidsJson() {
    printf '['
    sep=''
    for p in $(Descendants "$1"); do printf '%s%s' "$sep" "$p"; sep=','; done
    printf ']'
}

JsonStr() { printf '%s' "$1" | tr -d '\000-\037' | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g'; }
