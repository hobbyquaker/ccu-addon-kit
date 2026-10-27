#!/bin/sh
#
#   The shell checks (ccu-addon-kit, MIT): shellcheck and busybox sh -n on every addon script,
#   then kit-run and rc-helpers.sh under busybox ash and the system sh. Runs in the tcl86 image:
#     docker run --rm -v "$PWD:/kit" -w /kit ccu-addon-kit-tcl86 sh test/sh/run.sh
#
cd "$(dirname "$0")/../.." || exit 1
failed=0
ok() { echo "ok   $*"; }
fail() { echo "FAIL $*"; failed=1; }

# busybox ash supports `local` (SC3043); the CCU scripts are busybox scripts
SCRIPTS="classic/sh/kit-run classic/sh/rc.d-reference classic/sh/rc-helpers.sh classic/sh/update_script-reference
example/kitdemo/addon_files/update_script example/kitdemo/addon_files/kitdemo/rc.d example/kitdemo/addon_files/kitdemo/bin/kitdemo
example/kitdemo/build.sh test/sh/run.sh test/gate.sh"
for f in $SCRIPTS; do
    if shellcheck -s sh -S warning -e SC3043 "$f"; then ok "shellcheck $f"; else fail "shellcheck $f"; fi
    if busybox sh -n "$f"; then ok "busybox sh -n $f"; else fail "busybox sh -n $f"; fi
done

for SH in "busybox sh" sh; do
    t0=$(date +%s)
    out=$($SH classic/sh/kit-run 5 sh -c 'echo out; echo err >&2; exit 7'); rc=$?
    [ $rc -eq 7 ] && [ "$out" = "out
err" ] && ok "[$SH] kit-run: exit code and merged output" || fail "[$SH] kit-run: exit $rc, output '$out'"
    out=$($SH classic/sh/kit-run -q 5 sh -c 'echo out; echo err >&2'); rc=$?
    [ $rc -eq 0 ] && [ "$out" = "out" ] && ok "[$SH] kit-run -q drops stderr" || fail "[$SH] kit-run -q: exit $rc, output '$out'"
    t0=$(date +%s)
    out=$($SH classic/sh/kit-run 1 sh -c 'sleep 30; echo late'); rc=$?
    t1=$(date +%s)
    [ $rc -eq 124 ] && [ $((t1 - t0)) -le 4 ] && [ -z "$out" ] && ok "[$SH] kit-run: timeout 124 in $((t1 - t0)) s, children killed" \
        || fail "[$SH] kit-run timeout: exit $rc after $((t1 - t0)) s, output '$out'"
    out=$($SH classic/sh/kit-run 1 sh -c 'trap "" TERM; sleep 30'); rc=$?
    t2=$(date +%s)
    [ $rc -eq 124 ] && [ $((t2 - t1)) -le 5 ] && ok "[$SH] kit-run: KILL after an ignored TERM" || fail "[$SH] kit-run KILL: exit $rc after $((t2 - t1)) s"
    $SH classic/sh/kit-run x true 2>/dev/null; [ $? -eq 2 ] && ok "[$SH] kit-run: usage error 2" || fail "[$SH] kit-run usage"

    # rc-helpers: a tree of three processes, one with a space in its name
    sh -c 'sleep 100 & cp /bin/sleep "/tmp/a b"; "/tmp/a b" 100 & wait' &
    root=$!
    sleep 0.5
    n=$($SH -c ". classic/sh/rc-helpers.sh; Descendants $root" | wc -l)
    rss=$($SH -c ". classic/sh/rc-helpers.sh; RssTreeKb $root")
    pids=$($SH -c ". classic/sh/rc-helpers.sh; PidsJson $root")
    js=$($SH -c '. classic/sh/rc-helpers.sh; JsonStr "a\"b\\c	d"')
    # shellcheck disable=SC2046
    kill $(cat /proc/$root/task/*/children 2>/dev/null) $root 2>/dev/null
    [ "$n" -eq 3 ] && ok "[$SH] Descendants finds the tree" || fail "[$SH] Descendants: $n processes"
    [ "$rss" -gt 0 ] && ok "[$SH] RssTreeKb $rss" || fail "[$SH] RssTreeKb '$rss'"
    case "$pids" in "[$root,"*"]") ok "[$SH] PidsJson $pids" ;; *) fail "[$SH] PidsJson '$pids'" ;; esac
    [ "$js" = 'a\"b\\cd' ] && ok "[$SH] JsonStr" || fail "[$SH] JsonStr '$js'"
done
rm -f "/tmp/a b"

[ $failed -eq 0 ] && echo "shell checks: passed" || echo "shell checks: FAILED"
exit $failed
