#
#   kit/session.cgi - who is this? (MIT, docs/cgi-api.md). No level needed: this is how the
#   frame learns whether it has a session. An invalid one is a 200 with session.valid false.
#
source $KIT_HOME/lib/kit/cgi.tcl

kit_main {
    kit_check_session
    set s [list valid b $KIT_SESSION(valid)]
    if {$KIT_SESSION(valid)} {
        lappend s level s $KIT_SESSION(level) user z $KIT_SESSION(user) via s $KIT_SESSION(via)
        if {![string equal $KIT_SESSION(lang) ""]} { lappend s lang s $KIT_SESSION(lang) }
    }
    kit_ok [list session j [kit_json_obj $s] \
        platform j [kit_platform_json] \
        kit j [kit_json_obj [list version s $KIT_VERSION requestHeader s $KIT_REQUEST_HEADER]] \
        permissions j [kit_permissions_json]]
}
