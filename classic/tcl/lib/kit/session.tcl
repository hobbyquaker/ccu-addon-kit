#
#   ccu-addon-kit: the session check for classic CCU addons (MIT). Tcl 8.2 compatible.
#
#   Standalone: it sources nothing of the kit, so an addon can take it on its own.
#
#     kit_platform              -> ccu3 | openccu | openccu-lite
#     kit_is_openccu_lite       -> 1/0, evaluated per request, never cached
#     kit_session_check $sid    -> 1/0; fills the global array KIT_SESSION:
#                                    valid 0/1, level admin|user|guest|unknown, user <name>,
#                                    via header|sid, lang de|en|auto|""
#     kit_level_ok $needed      -> 1/0 for any|user|admin against KIT_SESSION(level)
#     kit_csrf_ok               -> "" when the request may write, else the reason:
#                                    method | header | origin | fetch-site
#
#   Where a session comes from (docs/session.md):
#   - CCU3 and OpenCCU: the WebUI opens the page as ?sid=@xxxxxxxxxx@; ReGa confirms it through
#     tclrega.so, Write(system.GetSessionVarStr('<id>')), whose answer is uid;upl;user;first;last;
#     (UPL 8 admin, 2 user, 1 guest) for a live session and "" otherwise.
#   - openccu-lite: lighttpd's gate adds X-Occulite-Session (and strips a client's copy); the
#     system confirms it with GET /api/auth/v1/state. The header is read on openccu-lite only:
#     a CCU passes a client's header through untouched. A confirmed header decides alone; ?sid=
#     is the fallback (asked at /state, then the tclrega shim, which knows the user name only:
#     level unknown).
#
#   Test hooks: with KIT_TEST in the environment (lighttpd never sets a variable without the
#   HTTP_ prefix from a request) KIT_VERSION_FILE, KIT_OCCULITED, KIT_S00, KIT_USERPROFILES and
#   KIT_STATE_URL replace /VERSION, /usr/bin/occulited, /etc/init.d/S00InstallAddon,
#   /etc/config/userprofiles and http://127.0.0.1/api/auth/v1/state.
#

if {[info exists KIT_SESSION_LOADED]} { return }
set KIT_SESSION_LOADED 1

set KIT_REQUEST_HEADER X-CCU-Addon-Request

proc kit__testpath {name default} {
    global env
    if {[info exists env(KIT_TEST)] && [info exists env($name)]} {
        return $env($name)
    }
    return $default
}

proc kit_is_openccu_lite {} {
    set file [kit__testpath KIT_VERSION_FILE /VERSION]
    if {[file readable $file]} {
        set fd [open $file r]
        set content [read $fd]
        close $fd
        foreach line [split $content "\n"] {
            if {[regexp {^LITE=} $line]} { return 1 }
        }
    }
    return [file executable [kit__testpath KIT_OCCULITED /usr/bin/occulited]]
}

proc kit_platform {} {
    if {[kit_is_openccu_lite]} { return openccu-lite }
    if {[file exists [kit__testpath KIT_S00 /etc/init.d/S00InstallAddon]]} { return ccu3 }
    return openccu
}

proc kit__session_reset {} {
    global KIT_SESSION
    catch {unset KIT_SESSION}
    array set KIT_SESSION {valid 0 level "" user "" via "" lang ""}
}
kit__session_reset

# --- openccu-lite: /api/auth/v1/state --------------------------------------------------------

proc kit__state_url {} {
    return [kit__testpath KIT_STATE_URL http://127.0.0.1/api/auth/v1/state]
}

# the body of a 200 from /state for this id, "" when the system cannot be asked
proc kit__state_body {id} {
    if {[catch {package require http}]} { return "" }
    if {[catch {http::geturl [kit__state_url] -headers [list Authorization "Bearer $id"] -timeout 5000} tok]} {
        return ""
    }
    set body ""
    if {[string equal [http::status $tok] ok] && [regexp {^HTTP/[0-9.]+ 200( |$)} [http::code $tok]]} {
        set body [http::data $tok]
    }
    http::cleanup $tok
    return $body
}

# Is `id` a live session of this system? Fills level and user on success. Only the two shapes
# the system hands out are ever sent: 26 characters of base32, or ten alphanumerics (the ids of
# older images, the shape of the legacy alias). An API token (olt_...) is refused by its shape;
# a session the answer does not name with the same sid (a token has none) is refused.
proc kit__lite_confirm {id via} {
    global KIT_SESSION
    if {![regexp {^[A-Z2-7]{26}$} $id] && ![regexp {^[A-Za-z0-9]{10}$} $id]} { return 0 }
    set state [kit__state_body $id]
    if {![regexp {"authenticated"[ ]*:[ ]*true} $state]} { return 0 }
    # JSON escapes every quote inside a string, so a user name cannot fake a key, and
    # "legacy_sid" does not match: the quote before the s is part of the pattern
    if {![regexp {"sid"[ ]*:[ ]*"([A-Za-z0-9]+)"} $state all stateSid] || ![string equal $stateSid $id]} {
        return 0
    }
    set level unknown
    if {[regexp {"level"[ ]*:[ ]*"([a-z]+)"} $state all l]} {
        switch -- $l {
            administer { set level admin }
            configure - operate { set level user }
            read { set level guest }
        }
    } elseif {[regexp {"role"[ ]*:[ ]*"([a-z]+)"} $state all r]} {
        switch -- $r {
            admin { set level admin }
            user { set level user }
        }
    }
    set user ""
    regexp {"user"[ ]*:[ ]*"([^"\\]*)"} $state all user
    set KIT_SESSION(valid) 1
    set KIT_SESSION(level) $level
    set KIT_SESSION(user) $user
    set KIT_SESSION(via) $via
    return 1
}

# --- CCU3 / OpenCCU: ReGa --------------------------------------------------------------------

set KIT_REGA_LOADED ""
proc kit__rega_answer {sidnr} {
    global KIT_REGA_LOADED
    if {[string equal $KIT_REGA_LOADED ""]} {
        if {[catch {load tclrega.so}]} {
            set KIT_REGA_LOADED 0
        } else {
            set KIT_REGA_LOADED 1
        }
    }
    if {!$KIT_REGA_LOADED} { return "" }
    # the ten checked characters are the only thing that ever goes into the script
    if {[catch {lindex [rega_script "Write(system.GetSessionVarStr('$sidnr'));"] 1} res]} {
        return ""
    }
    return $res
}

# the WebUI user's language: /etc/config/userprofiles/<user>.lang holds 0 auto, 1 de, 2 en
proc kit__user_lang {user} {
    if {![regexp {^[A-Za-z0-9_.@ -]+$} $user] || [string first .. $user] >= 0} { return "" }
    set file [kit__testpath KIT_USERPROFILES /etc/config/userprofiles]/$user.lang
    if {![file readable $file]} { return "" }
    set fd [open $file r]
    set v [string trim [read $fd 16]]
    close $fd
    switch -- $v {
        0 { return auto }
        1 { return de }
        2 { return en }
    }
    return ""
}

proc kit__rega_confirm {sid} {
    global KIT_SESSION
    if {![regexp {^@([0-9a-zA-Z]{10})@$} $sid all sidnr]} { return 0 }
    set answer [kit__rega_answer $sidnr]
    if {[string equal $answer ""]} { return 0 }
    set level unknown
    set user $answer
    if {[regexp {^([0-9]*);([0-9]+);([^;]*);} $answer all uid upl name]} {
        set user $name
        switch -- $upl {
            8 { set level admin }
            2 { set level user }
            1 { set level guest }
        }
    }
    set KIT_SESSION(valid) 1
    set KIT_SESSION(level) $level
    set KIT_SESSION(user) $user
    set KIT_SESSION(via) sid
    set KIT_SESSION(lang) [kit__user_lang $user]
    return 1
}

# --- the check ---------------------------------------------------------------------------------

proc kit_session_check {sid} {
    global env
    kit__session_reset
    if {[kit_is_openccu_lite]} {
        set header ""
        if {[info exists env(HTTP_X_OCCULITE_SESSION)]} { set header $env(HTTP_X_OCCULITE_SESSION) }
        if {![string equal $header ""] && [kit__lite_confirm $header header]} { return 1 }
        # the session itself in ?sid= (images before the header), never the header's id again
        set id $sid
        regexp {^@(.*)@$} $sid all id
        if {![string equal $id ""] && ![string equal $id $header] && [kit__lite_confirm $id sid]} {
            return 1
        }
    }
    return [kit__rega_confirm $sid]
}

# any: every valid session; user: user, admin and unknown (reading keeps working where the
# level cannot be confirmed); admin: admin only
proc kit_level_ok {needed} {
    global KIT_SESSION
    if {!$KIT_SESSION(valid)} { return 0 }
    switch -- $needed {
        any { return 1 }
        user {
            return [expr {[lsearch -exact {admin user unknown} $KIT_SESSION(level)] >= 0}]
        }
        admin { return [string equal $KIT_SESSION(level) admin] }
    }
    return 0
}

proc kit_csrf_ok {} {
    global env
    if {![info exists env(REQUEST_METHOD)] || ![string equal [string toupper $env(REQUEST_METHOD)] POST]} {
        return method
    }
    if {![info exists env(HTTP_X_CCU_ADDON_REQUEST)] || ![string equal $env(HTTP_X_CCU_ADDON_REQUEST) 1]} {
        return header
    }
    if {[info exists env(HTTP_ORIGIN)]} {
        set host ""
        if {[info exists env(HTTP_HOST)]} { set host [string tolower $env(HTTP_HOST)] }
        if {![regexp {^[a-zA-Z][a-zA-Z0-9+.-]*://([^/]+)$} $env(HTTP_ORIGIN) all ohost]} {
            return origin
        }
        if {[string equal $host ""] || ![string equal [string tolower $ohost] $host]} {
            return origin
        }
    }
    if {[info exists env(HTTP_SEC_FETCH_SITE)]} {
        set s [string tolower $env(HTTP_SEC_FETCH_SITE)]
        if {![string equal $s same-origin] && ![string equal $s none]} {
            return fetch-site
        }
    }
    return ""
}
