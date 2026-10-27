#
#   ccu-addon-kit: the release check shared by kit/version.cgi and update_check.cgi (MIT).
#   Tcl 8.2 compatible.
#
#   GET https://api.github.com/repos/<owner>/<repo>/releases/latest (drafts and prereleases
#   excluded) with curl through kit-run. Cache /tmp/kit-<id>-release.json (tmpfs, never the
#   userfs): a success for 6 h, an error for 30 min, so a CCU stays far below GitHub's 60
#   unauthenticated requests per hour. No network is not an error, it is "offline".
#

source $KIT_HOME/lib/kit/cgi.tcl

set KIT_RELEASE_TTL_OK 21600
set KIT_RELEASE_TTL_ERR 1800
set KIT_RELEASE_FORCE_MIN 60

proc kit_installed_version {} {
    foreach {k v} [kit_versions] {
        if {[string equal $k VERSION_ADDON]} { return $v }
    }
    return ""
}

proc kit__release_cache {} {
    global KIT
    return [kit_tmp]/kit-$KIT(id)-release.json
}

# the cache as a list {latest url checkedAt error}, "" when there is none
proc kit__release_read_cache {} {
    set data [kit_read_file [kit__release_cache] 8192]
    if {![regexp {"checkedAt":([0-9]+)} $data all at]} { return "" }
    set latest ""
    set url ""
    set error ""
    regexp {"latest":"([^"\\]*)"} $data all latest
    regexp {"url":"([^"\\]*)"} $data all url
    regexp {"error":"([^"\\]*)"} $data all error
    return [list $latest $url $at $error]
}

proc kit__release_write_cache {latest url at error} {
    set file [kit__release_cache]
    set tmp $file.[pid]
    if {[catch {
        set fd [open $tmp w]
        puts $fd [kit_json_obj [list latest z $latest url z $url checkedAt n $at error z $error]]
        close $fd
        file rename -force $tmp $file
    }]} {
        catch {file delete -force $tmp}
    }
}

# what GitHub says now: {latest url error}
proc kit__release_fetch {} {
    global KIT
    set api https://api.github.com/repos/$KIT(github)/releases/latest
    set r [kit_run_stdout 15 curl -sS --connect-timeout 5 --max-time 10 \
        -H "Accept: application/vnd.github+json" -H "User-Agent: ccu-addon-kit" \
        -w "\n%{http_code}" $api]
    set out [lindex $r 1]
    set nl [string last "\n" $out]
    set status [string trim [string range $out [expr {$nl + 1}] end]]
    set body [string range $out 0 [expr {$nl - 1}]]
    if {$nl < 0} {
        set status ""
        set body $out
    }
    if {[lindex $r 0] != 0 || ![regexp {^[0-9][0-9][0-9]$} $status] || [string equal $status 000]} {
        return [list "" "" offline]
    }
    if {[string equal $status 200]} {
        if {![regexp {"tag_name"[ ]*:[ ]*"([^"\\]+)"} $body all tag]} {
            return [list "" "" no-release]
        }
        set url ""
        regexp {"html_url"[ ]*:[ ]*"(https://github\.com/[^"\\]+/releases/tag/[^"\\]+)"} $body all url
        regsub {^[vV]} $tag "" latest
        return [list $latest $url ""]
    }
    if {([string equal $status 403] || [string equal $status 429])
            && [regexp -nocase {rate limit} $body]} {
        return [list "" "" rate-limited]
    }
    if {[string equal $status 404]} {
        return [list "" "" no-release]
    }
    return [list "" "" http-$status]
}

# the release check as a name/value list: latest url checkedAt source error newer
proc kit_release {force} {
    global KIT KIT_RELEASE_TTL_OK KIT_RELEASE_TTL_ERR KIT_RELEASE_FORCE_MIN
    set now [clock seconds]
    if {[string equal $KIT(github) ""]} {
        return [list latest "" url "" checkedAt "" source none error not-configured newer 0]
    }
    set c [kit__release_read_cache]
    set source cache
    set fresh 0
    if {![string equal $c ""]} {
        set age [expr {$now - [lindex $c 2]}]
        if {[string equal [lindex $c 3] ""]} { set ttl $KIT_RELEASE_TTL_OK } else { set ttl $KIT_RELEASE_TTL_ERR }
        if {$force} { set ttl $KIT_RELEASE_FORCE_MIN }
        if {$age >= 0 && $age < $ttl} { set fresh 1 }
    }
    if {!$fresh} {
        set lock [kit_lock release]
        if {[string equal $lock ""] && ![string equal $c ""]} {
            # another request is asking GitHub right now: the cache is good enough
        } else {
            set f [kit__release_fetch]
            kit_unlock $lock
            set c [list [lindex $f 0] [lindex $f 1] $now [lindex $f 2]]
            kit__release_write_cache [lindex $c 0] [lindex $c 1] $now [lindex $c 3]
            set source network
        }
    }
    set latest [lindex $c 0]
    set url [lindex $c 1]
    if {[string equal $url ""] && ![string equal $latest ""]} {
        set url https://github.com/$KIT(github)/releases/latest
    }
    set newer 0
    set installed [kit_installed_version]
    if {![string equal $latest ""] && [kit_version_compare $latest $installed] > 0} { set newer 1 }
    return [list latest $latest url $url checkedAt [lindex $c 2] source $source error [lindex $c 3] newer $newer]
}

proc kit_release_fields {force} {
    array set r [kit_release $force]
    set t [list installed z [kit_installed_version] latest z $r(latest) newer b $r(newer)]
    if {![string equal $r(url) ""]} { lappend t url s $r(url) }
    if {![string equal $r(checkedAt) ""]} { lappend t checkedAt n $r(checkedAt) }
    lappend t source s $r(source)
    if {![string equal $r(error) ""]} { lappend t error s $r(error) }
    return $t
}
