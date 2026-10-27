#
#   kit/version.cgi - the installed version and the release check (MIT, docs/cgi-api.md).
#
#     GET                       (read)  installed, components, kit, platform
#     GET ?cmd=check            (read)  the newest release from GitHub, cached
#     POST cmd=check&force=1    (read + CSRF)  the same, ignoring a cache older than 60 s
#
source $KIT_HOME/lib/kit/release.tcl

kit_main {
    if {[kit_request_is_post]} {
        kit_form f
        if {![info exists f(cmd)] || ![string equal $f(cmd) check]} {
            kit_fail 400 bad-request "unknown command"
        }
        # a forced check is a read that costs a request to GitHub: CSRF-checked like a write
        if {![kit_check_session]} { kit_fail 401 session-invalid "no valid session" }
        set reason [kit_csrf_ok]
        if {![string equal $reason ""]} { kit_fail 403 csrf "request refused: $reason" }
        kit_require read
        set force 0
        if {[info exists f(force)] && [string equal $f(force) 1]} { set force 1 }
        kit_ok [kit_release_fields $force]
    }
    kit_require read
    kit_query q
    if {[info exists q(cmd)]} {
        if {![string equal $q(cmd) check]} { kit_fail 400 bad-request "unknown command" }
        kit_ok [kit_release_fields 0]
    }
    set comps [list]
    foreach {k v} [kit_versions] {
        if {[string equal $k VERSION_ADDON]} { continue }
        if {[regexp {^(.+)_VERSION$} $k all name] || [regexp {^VERSION_(.+)$} $k all name]} {
            lappend comps [string tolower $name] s $v
        }
    }
    kit_ok [list installed z [kit_installed_version] components j [kit_json_obj $comps] \
        kit s $KIT_VERSION platform j [kit_platform_json]]
}
