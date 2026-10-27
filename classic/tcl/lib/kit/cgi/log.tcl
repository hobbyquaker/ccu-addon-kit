#
#   kit/log.cgi - log tail, download and debug bundle (MIT, docs/cgi-api.md). Level: log.
#
#     GET                              the sources
#     GET ?source=<id>&lines=200       the newest lines (max 2000) of the last 256 KiB
#     GET ?source=<id>&download=1      text/plain attachment, the last 4 MiB
#     GET ?bundle=1                    a text debug bundle
#
#   Sources come from kit.json's logs: a file (+ rotated), or a syslog tag (lines of
#   /var/log/messages with "<tag>[" or "<tag>:"). openccu-lite has no /var/log/messages: a
#   syslog source answers available false and the link to the system's log page.
#
source $KIT_HOME/lib/kit/cgi.tcl

set KIT_TAIL_BYTES 262144
set KIT_DOWNLOAD_BYTES 4194304

proc kit__messages {} { return [kit__testpath KIT_MESSAGES /var/log/messages] }

proc kit__source {id} {
    global KIT
    foreach s $KIT(logs) {
        array set src $s
        if {[string equal $src(id) $id]} { return $s }
        unset src
    }
    return ""
}

# {lines truncated} - the newest `want` lines of the last `bytes` bytes of a file
proc kit__tail {file bytes want} {
    if {![file isfile $file] || ![file readable $file]} { return [list {} 0] }
    set size [file size $file]
    set fd [open $file r]
    fconfigure $fd -translation binary
    set truncated 0
    if {$size > $bytes} {
        seek $fd [expr {$size - $bytes}]
        set truncated 1
    }
    set data [read $fd]
    close $fd
    if {$truncated} {
        set nl [string first "\n" $data]
        set data [string range $data [expr {$nl + 1}] end]
    }
    set data [encoding convertfrom utf-8 $data]
    if {[string equal [string index $data end] "\n"]} {
        set data [string range $data 0 [expr {[string length $data] - 2}]]
    }
    if {[string equal $data ""]} { return [list {} $truncated] }
    set lines [split $data "\n"]
    set n [llength $lines]
    if {$n > $want} {
        set lines [lrange $lines [expr {$n - $want}] end]
        set truncated 1
    }
    return [list $lines $truncated]
}

proc kit__tag_filter {lines tag} {
    set out [list]
    foreach l $lines {
        if {[string first "$tag\[" $l] >= 0 || [string first "$tag:" $l] >= 0} { lappend out $l }
    }
    return $out
}

# {lines truncated} of a source
proc kit__read_source {s want bytes} {
    array set src $s
    if {[string equal $src(kind) file]} {
        set r [kit__tail $src(file) $bytes $want]
        set lines [lindex $r 0]
        set truncated [lindex $r 1]
        if {[llength $lines] < $want && [info exists src(rotated)] && [file isfile $src(rotated)]} {
            set older [lindex [kit__tail $src(rotated) $bytes [expr {$want - [llength $lines]}]] 0]
            set lines [concat $older $lines]
            set truncated 1
        } elseif {[info exists src(rotated)] && [file isfile $src(rotated)]} {
            set truncated 1
        }
        return [list $lines $truncated]
    }
    set m [kit__messages]
    set big [expr {$bytes * 4}]
    set r [kit__tail $m $big 1000000]
    set lines [kit__tag_filter [lindex $r 0] $src(tag)]
    set truncated [lindex $r 1]
    if {[llength $lines] < $want && [file isfile $m.0]} {
        set older [kit__tag_filter [lindex [kit__tail $m.0 $big 1000000] 0] $src(tag)]
        set lines [concat $older $lines]
        set truncated 1
    }
    set n [llength $lines]
    if {$n > $want} {
        set lines [lrange $lines [expr {$n - $want}] end]
        set truncated 1
    }
    return [list $lines $truncated]
}

proc kit__source_available {s} {
    array set src $s
    if {[string equal $src(kind) file]} { return [file readable $src(file)] }
    if {[kit_is_openccu_lite]} { return 0 }
    return [file readable [kit__messages]]
}

proc kit__source_json {s} {
    global KIT
    array set src $s
    set t [list id s $src(id) label j [kit_json_i18n $src(label)] kind s $src(kind) \
        available b [kit__source_available $s]]
    if {[string equal $src(kind) file] && [file isfile $src(file)]} {
        lappend t size n [file size $src(file)] mtime n [file mtime $src(file)]
    }
    if {[string equal $src(kind) syslog] && [kit_is_openccu_lite]} {
        lappend t link s /system/log?unit=addon-$KIT(id)
    }
    return [kit_json_obj $t]
}

proc kit__stamp {} { return [clock format [clock seconds] -format %Y%m%d-%H%M] }

proc kit__lines_json {lines} {
    set parts [list]
    foreach l $lines { lappend parts [kit_json_string $l] }
    return [kit_json_list $parts]
}

proc kit__bundle {} {
    global KIT
    set out ""
    set sections [list \
        "versions" [list cat $KIT(versionFile)] \
        "/VERSION" [list cat [kit__testpath KIT_VERSION_FILE /VERSION]] \
        "uname -a" [list uname -a] \
        "status" [list $KIT(rc) status] \
        "df -h" [list df -h] \
        "free" [list free]]
    if {[file readable $KIT(addonDir)/lib/kit-debug.tcl]} {
        if {![catch {source $KIT(addonDir)/lib/kit-debug.tcl}] && [info procs kit_debug_sections] != ""} {
            catch {set sections [concat $sections [kit_debug_sections]]}
        }
    }
    foreach {title cmd} $sections {
        append out "===== $title =====\n"
        set r [eval [concat [list kit_run 15] $cmd]]
        append out [lindex $r 1] "\n"
        if {[lindex $r 0] != 0} { append out "(exit [lindex $r 0])\n" }
        append out "\n"
    }
    foreach s $KIT(logs) {
        array set src $s
        append out "===== log $src(id) ($src(kind)) =====\n"
        if {[kit__source_available $s]} {
            append out [join [lindex [kit__read_source $s 200 262144] 0] "\n"] "\n"
        } else {
            append out "(not available)\n"
        }
        append out "\n"
        unset src
    }
    return $out
}

kit_main {
    kit_require log
    kit_query q
    if {[info exists q(bundle)] && [string equal $q(bundle) 1]} {
        kit_header 200 "text/plain; charset=utf-8" \
            [list Content-Disposition "attachment; filename=\"$KIT(id)-debug-[kit__stamp].txt\""]
        kit_body [kit__bundle]
        exit 0
    }
    if {![info exists q(source)]} {
        set parts [list]
        foreach s $KIT(logs) { lappend parts [kit__source_json $s] }
        kit_ok [list sources j [kit_json_list $parts]]
    }
    set s [kit__source $q(source)]
    if {[string equal $s ""]} {
        kit_fail 404 not-found "unknown log source"
    }
    array set src $s
    if {[string equal $src(kind) syslog] && [kit_is_openccu_lite]} {
        kit_ok [list source s $src(id) available b 0 link s /system/log?unit=addon-$KIT(id) lines j {[]}]
    }
    if {[info exists q(download)] && [string equal $q(download) 1]} {
        kit_header 200 "text/plain; charset=utf-8" \
            [list Content-Disposition "attachment; filename=\"$KIT(id)-$src(id)-[kit__stamp].log\""]
        if {[string equal $src(kind) file]} {
            kit_body [kit_read_file $src(file) $KIT_DOWNLOAD_BYTES]
        } else {
            kit_body "[join [lindex [kit__read_source $s 100000 $KIT_DOWNLOAD_BYTES] 0] "\n"]\n"
        }
        exit 0
    }
    set want 200
    if {[info exists q(lines)]} {
        if {![regexp {^[0-9]+$} $q(lines)] || $q(lines) < 1} {
            kit_fail 400 bad-request "lines must be a positive number"
        }
        set want $q(lines)
        if {$want > 2000} { set want 2000 }
    }
    set r [kit__read_source $s $want $KIT_TAIL_BYTES]
    set t [list source s $src(id) lines j [kit__lines_json [lindex $r 0]] truncated b [lindex $r 1]]
    if {[string equal $src(kind) file] && [file isfile $src(file)]} {
        lappend t size n [file size $src(file)] mtime n [file mtime $src(file)]
    }
    kit_ok $t
}
