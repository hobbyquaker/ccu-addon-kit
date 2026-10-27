#
#   kit/alive.cgi - {"running": true|false} for the 503 page (MIT, docs/cgi-api.md). No
#   session: it answers nothing but whether the rc.d script's status exits 0.
#
source $KIT_HOME/lib/kit/cgi.tcl

kit_main {
    set r [kit_run_stdout 5 $KIT(rc) status]
    kit_header
    kit_body "[kit_json_obj [list running b [expr {[lindex $r 0] == 0}]]]\n"
}
