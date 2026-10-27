#
#   kit/service.cgi - status and actions of the addon's service (MIT, docs/cgi-api.md).
#
#     GET  ?cmd=status          (read)  the rc.d script's status line verbatim, X-Kit-Exit and
#                                       the memory headers
#     POST cmd=start|stop|restart|reload   (service + CSRF)  {"ok","cmd","exit","output"}
#
source $KIT_HOME/lib/kit/cgi.tcl

proc kit__meminfo {} {
    set total ""
    set avail ""
    foreach line [split [kit_read_file [kit__testpath KIT_MEMINFO /proc/meminfo] 16384] "\n"] {
        regexp {^MemTotal:[ ]+([0-9]+)} $line all total
        regexp {^MemAvailable:[ ]+([0-9]+)} $line all avail
    }
    return [list $total $avail]
}

set KIT_ACTIONS {start stop restart reload}
# the convention's limits plus slack (docs/service-convention.md)
array set KIT_LIMIT {start 90 stop 90 restart 180 reload 20}

kit_main {
    if {[kit_request_is_post]} {
        kit_form f
        set cmd ""
        if {[info exists f(cmd)]} { set cmd $f(cmd) }
        kit_require service
        if {[lsearch -exact $KIT_ACTIONS $cmd] < 0 || [lsearch -exact $KIT(commands) $cmd] < 0} {
            kit_fail 400 bad-request "unknown command"
        }
        set lock [kit_lock service]
        if {[string equal $lock ""]} {
            kit_fail 409 busy "another service action is running"
        }
        set r [list 1 ""]
        set rc [catch {set r [kit_run $KIT_LIMIT($cmd) $KIT(rc) $cmd]} err]
        kit_unlock $lock
        if {$rc} { error $err }
        set code [lindex $r 0]
        set out [lindex $r 1]
        kit_log info "service $cmd: exit $code"
        set fields [list cmd s $cmd exit n $code output s $out]
        if {$code == 124} {
            kit_fail 504 timeout "$cmd ran out of time" $fields
        }
        if {$code != 0} {
            kit_fail 500 failed "$cmd exited with $code" $fields
        }
        kit_ok $fields
    }
    kit_query q
    set cmd status
    if {[info exists q(cmd)]} { set cmd $q(cmd) }
    if {[lsearch -exact $KIT_ACTIONS $cmd] >= 0} {
        kit_fail 405 method "$cmd must be a POST" {} {Allow POST}
    }
    if {![string equal $cmd status]} {
        kit_fail 400 bad-request "unknown command"
    }
    kit_require read
    set r [kit_run_stdout 5 $KIT(rc) status]
    set code [lindex $r 0]
    set out [string trim [lindex $r 1]]
    if {$code == 124} {
        kit_fail 504 timeout "status ran out of time"
    }
    set mem [kit__meminfo]
    set headers [list X-Kit-Exit $code]
    if {![string equal [lindex $mem 0] ""]} { lappend headers X-Kit-Mem-Total-Kb [lindex $mem 0] }
    if {![string equal [lindex $mem 1] ""]} { lappend headers X-Kit-Mem-Available-Kb [lindex $mem 1] }
    kit_header 200 "application/json; charset=utf-8" $headers
    if {[string first "\n" $out] < 0 && [string equal [string index $out 0] "\{"]} {
        kit_body "$out\n"
    } else {
        kit_body "[kit_json_obj [list schema n 0 running b [expr {$code == 0}] raw s $out]]\n"
    }
}
