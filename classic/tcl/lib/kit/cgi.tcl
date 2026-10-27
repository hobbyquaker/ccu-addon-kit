#
#   ccu-addon-kit: the CGI runtime for the kit's own CGIs and an addon's tab CGIs (MIT).
#   Tcl 8.2 compatible. Needs KIT_HOME (the addon's directory, set by the generated stub).
#
#     kit_main {body}            runs body; a Tcl error becomes a 500 internal, never a bare 500
#     kit_query q / kit_form f   (form.tcl)
#     kit_request_is_post        (form.tcl)
#     kit_require read|write|service|log
#                                checks the request for that class (method, session, CSRF,
#                                level); on failure answers the JSON error and exits
#     kit_ok {name t value ...}  {"ok":true,...} (types as kit_json_obj) and exits
#     kit_fail status code message ?{name t value ...}?
#                                {"ok":false,"error":{"code","message"},...} and exits
#     kit_json_string / kit_json_obj / kit_json_list   (json.tcl)
#     kit_run seconds cmd args   -> {exitcode output}, through bin/kit-run, stdin /dev/null
#     kit_run_stdout seconds cmd args   the same with stderr dropped
#     kit_read_file path ?max?   "" when missing; the last max bytes when longer
#     kit_versions               the versions file as a name/value list
#     kit_log level text         logger -t <id>-kit -p daemon.<level>
#     kit_header ?status? ?type? ?headers?   the response header (once)
#

if {[info exists KIT_CGI_LOADED]} { return }
set KIT_CGI_LOADED 1

if {![info exists KIT_HOME]} { set KIT_HOME [file dirname [file dirname [file dirname [info script]]]] }

source $KIT_HOME/lib/kit/json.tcl
source $KIT_HOME/lib/kit/form.tcl
source $KIT_HOME/lib/kit/session.tcl
source $KIT_HOME/lib/kit/addon.tcl

set KIT_VERSION unknown
catch {
    set fd [open $KIT_HOME/lib/kit/VERSION r]
    set KIT_VERSION [lindex [split [string trim [read $fd]] " "] 0]
    close $fd
}

# defaults for what addon.tcl does not set
foreach {k v} [list addonDir $KIT_HOME rc /usr/local/etc/config/rc.d/$KIT(id) \
        versionFile $KIT_HOME/versions commands {start stop restart reload} logs {} github "" \
        perm,read any perm,service any perm,log any perm,write any] {
    if {![info exists KIT($k)]} { set KIT($k) $v }
}
unset k v

set KIT_HEADER_SENT 0

proc kit__status_line {status} {
    switch -- $status {
        200 { return "200 OK" }
        302 { return "302 Found" }
        400 { return "400 Bad Request" }
        401 { return "401 Unauthorized" }
        403 { return "403 Forbidden" }
        404 { return "404 Not Found" }
        405 { return "405 Method Not Allowed" }
        409 { return "409 Conflict" }
        500 { return "500 Internal Server Error" }
        503 { return "503 Service Unavailable" }
        504 { return "504 Gateway Timeout" }
    }
    return "$status Status"
}

proc kit_header {{status 200} {type "application/json; charset=utf-8"} {headers {}}} {
    global KIT_HEADER_SENT
    if {$KIT_HEADER_SENT} { return }
    set KIT_HEADER_SENT 1
    fconfigure stdout -translation binary
    set h ""
    if {$status != 200} { append h "Status: [kit__status_line $status]\r\n" }
    append h "Content-Type: $type\r\n"
    append h "Cache-Control: no-store\r\n"
    append h "X-Content-Type-Options: nosniff\r\n"
    foreach {name value} $headers {
        # a header value never carries a line break
        append h "$name: [string map [list "\r" " " "\n" " "] $value]\r\n"
    }
    append h "\r\n"
    puts -nonewline stdout $h
    fconfigure stdout -translation lf -encoding utf-8
}

proc kit_body {text} {
    puts -nonewline stdout $text
    flush stdout
}

proc kit_ok {{triples {}} {status 200} {headers {}}} {
    kit_header $status "application/json; charset=utf-8" $headers
    kit_body "[kit_json_obj [concat [list ok b 1] $triples]]\n"
    exit 0
}

proc kit_fail {status code message {triples {}} {headers {}}} {
    kit_header $status "application/json; charset=utf-8" $headers
    set err [kit_json_obj [list code s $code message s $message]]
    kit_body "[kit_json_obj [concat [list ok b 0 error j $err] $triples]]\n"
    exit 0
}

proc kit_main {body} {
    global errorInfo
    set m [kit_request_method]
    if {![string equal $m GET] && ![string equal $m POST]} {
        kit_fail 405 method "method $m not allowed" {} {Allow {GET, POST}}
    }
    if {[catch {uplevel #0 $body} err]} {
        set first [lindex [split $errorInfo "\n"] 0]
        catch {kit_log err "internal: $first"}
        kit_fail 500 internal $first
    }
}

# the level a request class needs (kit.json permissions; default any)
proc kit_needed {cls} {
    global KIT
    if {[info exists KIT(perm,$cls)]} { return $KIT(perm,$cls) }
    return any
}

proc kit_sid {} {
    kit_query q
    if {[info exists q(sid)]} { return $q(sid) }
    return ""
}

set KIT_CHECKED 0
proc kit_check_session {} {
    global KIT_CHECKED KIT_SESSION
    if {!$KIT_CHECKED} {
        kit_session_check [kit_sid]
        set KIT_CHECKED 1
    }
    return $KIT_SESSION(valid)
}

proc kit_require {cls} {
    global KIT KIT_SESSION env
    set writes [expr {[string equal $cls write] || [string equal $cls service]}]
    if {$writes && ![kit_request_is_post]} {
        kit_fail 405 method "$cls requests must be POST" {} {Allow POST}
    }
    if {![kit_check_session]} {
        kit_fail 401 session-invalid "no valid session"
    }
    if {$writes} {
        set reason [kit_csrf_ok]
        if {![string equal $reason ""]} {
            set addr -
            if {[info exists env(REMOTE_ADDR)]} { set addr $env(REMOTE_ADDR) }
            catch {kit_log warning "csrf refused: $reason from $addr"}
            kit_fail 403 csrf "request refused: $reason"
        }
    }
    if {![kit_level_ok [kit_needed $cls]]} {
        if {[string equal $KIT_SESSION(level) unknown]} {
            kit_fail 403 forbidden "the level of this session could not be confirmed"
        }
        kit_fail 403 forbidden "the session's level ($KIT_SESSION(level)) is below [kit_needed $cls]"
    }
}

# --- helpers -----------------------------------------------------------------------------

proc kit_tmp {} {
    return [kit__testpath KIT_TMP /tmp]
}

proc kit__exec {cmd} {
    global errorCode
    if {[catch {eval exec $cmd < /dev/null} out]} {
        set code 1
        if {[string equal [lindex $errorCode 0] CHILDSTATUS]} {
            set code [lindex $errorCode 2]
        } elseif {[string equal [lindex $errorCode 0] CHILDKILLED]} {
            set code 137
        } elseif {[string equal [lindex $errorCode 0] POSIX]} {
            set code 127
        }
        regsub "\n?child process exited abnormally\$" $out "" out
        return [list $code $out]
    }
    return [list 0 $out]
}

proc kit_run {seconds args} {
    global KIT_HOME
    return [kit__exec [concat [list $KIT_HOME/bin/kit-run $seconds] $args]]
}

proc kit_run_stdout {seconds args} {
    global KIT_HOME
    return [kit__exec [concat [list $KIT_HOME/bin/kit-run -q $seconds] $args]]
}

proc kit_read_file {path {max 0}} {
    if {![file isfile $path] || ![file readable $path]} { return "" }
    set fd [open $path r]
    fconfigure $fd -translation binary
    if {$max > 0} {
        set size [file size $path]
        if {$size > $max} { seek $fd [expr {$size - $max}] }
        set data [read $fd $max]
    } else {
        set data [read $fd]
    }
    close $fd
    return [encoding convertfrom utf-8 $data]
}

# KEY=value lines, optionally "export " and quoted - what an rc.d script sources
proc kit_versions {} {
    global KIT
    set result [list]
    foreach line [split [kit_read_file $KIT(versionFile) 65536] "\n"] {
        set line [string trim $line]
        regsub {^export[ ]+} $line "" line
        if {[regexp {^([A-Za-z_][A-Za-z0-9_]*)=(.*)$} $line all key value]} {
            set value [string trim $value]
            if {[regexp {^"(.*)"$} $value all inner] || [regexp {^'(.*)'$} $value all inner]} {
                set value $inner
            }
            lappend result $key $value
        }
    }
    return $result
}

proc kit_log {level text} {
    global KIT
    set logger [kit__testpath KIT_LOGGER logger]
    catch {exec $logger -t $KIT(id)-kit -p daemon.$level $text < /dev/null}
}

# /VERSION's VERSION=, the firmware version on all three platforms
proc kit_firmware {} {
    set file [kit__testpath KIT_VERSION_FILE /VERSION]
    foreach line [split [kit_read_file $file 8192] "\n"] {
        if {[regexp {^VERSION=(.*)$} [string trim $line] all v]} { return [string trim $v "\""] }
    }
    return ""
}

proc kit_arch {} {
    global tcl_platform
    if {[info exists tcl_platform(machine)]} { return $tcl_platform(machine) }
    return ""
}

proc kit_platform_json {} {
    return [kit_json_obj [list kind s [kit_platform] firmware s [kit_firmware] arch s [kit_arch] \
        tcl s [info patchlevel]]]
}

# the evaluated permission table for this session
proc kit_permissions_json {} {
    set t [list]
    foreach cls {read service log write} {
        lappend t $cls b [kit_level_ok [kit_needed $cls]]
    }
    return [kit_json_obj $t]
}

# one directory as an atomic lock (mkdir fails when it exists); a lock older than five minutes
# is stale. Returns the path, or "" when it is held.
proc kit_lock {name} {
    global KIT
    set dir [kit_tmp]/kit-$KIT(id)-$name.lock
    if {[lindex [kit__exec [list mkdir $dir]] 0] == 0} { return $dir }
    if {[file isdirectory $dir] && [clock seconds] - [file mtime $dir] > 300} {
        catch {file delete -force $dir}
        if {[lindex [kit__exec [list mkdir $dir]] 0] == 0} { return $dir }
    }
    return ""
}

proc kit_unlock {dir} {
    if {![string equal $dir ""]} { catch {file delete -force $dir} }
}

# version comparison: -1, 0, 1. X.Y.Z with an optional +n (a build number) and -pre (older than
# its release; identifiers compared numerically or as text), a leading v ignored. A version that
# does not parse sorts below every one that does.
proc kit__version_parse {v} {
    set v [string trim $v]
    regsub {^[vV]} $v "" v
    if {![regexp {^([0-9]+)\.([0-9]+)\.([0-9]+)(.*)$} $v all a b c rest]} { return "" }
    set build 0
    set pre ""
    if {[regexp {\+([0-9]+)} $rest all n]} { set build $n }
    if {[regexp {\-([0-9A-Za-z.-]+)} $rest all p]} {
        # a build number after the prerelease is not part of it
        regsub {\+.*$} $p "" pre
    }
    return [list $a $b $c $build $pre]
}

proc kit__cmp_num {a b} {
    regsub {^0+([0-9])} $a {\1} a
    regsub {^0+([0-9])} $b {\1} b
    if {[string length $a] != [string length $b]} {
        if {[string length $a] < [string length $b]} { return -1 }
        return 1
    }
    return [string compare $a $b]
}

proc kit_version_compare {x y} {
    set px [kit__version_parse $x]
    set py [kit__version_parse $y]
    if {[string equal $px ""] || [string equal $py ""]} {
        if {[string equal $px ""] && [string equal $py ""]} { return 0 }
        if {[string equal $px ""]} { return -1 }
        return 1
    }
    foreach i {0 1 2} {
        set c [kit__cmp_num [lindex $px $i] [lindex $py $i]]
        if {$c != 0} { return $c }
    }
    set prex [lindex $px 4]
    set prey [lindex $py 4]
    if {![string equal $prex $prey]} {
        if {[string equal $prex ""]} { return 1 }
        if {[string equal $prey ""]} { return -1 }
        set lx [split $prex .]
        set ly [split $prey .]
        set n [llength $lx]
        if {[llength $ly] > $n} { set n [llength $ly] }
        for {set i 0} {$i < $n} {incr i} {
            if {$i >= [llength $lx]} { return -1 }
            if {$i >= [llength $ly]} { return 1 }
            set a [lindex $lx $i]
            set b [lindex $ly $i]
            set na [regexp {^[0-9]+$} $a]
            set nb [regexp {^[0-9]+$} $b]
            if {$na && $nb} {
                set c [kit__cmp_num $a $b]
            } elseif {$na} {
                set c -1
            } elseif {$nb} {
                set c 1
            } else {
                set c [string compare $a $b]
            }
            if {$c != 0} { return $c }
        }
    }
    return [kit__cmp_num [lindex $px 3] [lindex $py 3]]
}
