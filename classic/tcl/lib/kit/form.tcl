#
#   ccu-addon-kit: query string and form body (MIT). Tcl 8.2 compatible.
#
#   kit_query arr          the decoded query parameters into the array arr
#   kit_form arr           a POSTed application/x-www-form-urlencoded body (<= 1 MiB) into arr;
#                          empty for anything else, so a change field in the query is never taken
#   kit_request_is_post    1 for a POST
#   kit_request_method     GET, POST, ... (upper case; GET when unset)
#
#   Nothing becomes a global variable, and nothing is decoded with subst: a query string
#   holding [exec ...] stays text.
#

if {[info exists KIT_FORM_LOADED]} { return }
set KIT_FORM_LOADED 1

set KIT_FORM_MAX 1048576

proc kit_urldecode {value} {
    set value [string map {+ " "} $value]
    if {[string first % $value] < 0} {
        return $value
    }
    set out ""
    set len [string length $value]
    set i 0
    while {$i < $len} {
        set c [string index $value $i]
        if {[string equal $c %]} {
            set hex [string range $value [expr {$i + 1}] [expr {$i + 2}]]
            if {[regexp {^[0-9A-Fa-f][0-9A-Fa-f]$} $hex]} {
                scan $hex %x code
                append out [format %c $code]
                incr i 3
                continue
            }
        }
        append out $c
        incr i
    }
    # the decoded bytes are utf-8; without this an umlaut arrives as two characters
    return [encoding convertfrom utf-8 $out]
}

proc kit_parse_pairs {input arrayName} {
    upvar $arrayName arr
    foreach pair [split $input &] {
        if {[string equal $pair ""]} { continue }
        set eq [string first = $pair]
        if {$eq < 0} {
            set arr([kit_urldecode $pair]) ""
        } else {
            set arr([kit_urldecode [string range $pair 0 [expr {$eq - 1}]]]) \
                [kit_urldecode [string range $pair [expr {$eq + 1}] end]]
        }
    }
}

proc kit_request_method {} {
    global env
    if {![info exists env(REQUEST_METHOD)] || [string equal $env(REQUEST_METHOD) ""]} {
        return GET
    }
    return [string toupper $env(REQUEST_METHOD)]
}

proc kit_request_is_post {} {
    return [string equal [kit_request_method] POST]
}

proc kit_query {arrayName} {
    upvar $arrayName arr
    global env
    array set arr {}
    if {[info exists env(QUERY_STRING)]} {
        kit_parse_pairs $env(QUERY_STRING) arr
    }
}

set KIT_FORM_BODY ""
set KIT_FORM_READ 0

# the raw body, read once (a second kit_form gets the same fields)
proc kit_form_body {} {
    global env KIT_FORM_BODY KIT_FORM_READ KIT_FORM_MAX
    if {$KIT_FORM_READ} { return $KIT_FORM_BODY }
    set KIT_FORM_READ 1
    if {![kit_request_is_post]} { return "" }
    set len 0
    if {[info exists env(CONTENT_LENGTH)]} { set len $env(CONTENT_LENGTH) }
    if {![regexp {^[0-9]+$} $len] || $len == 0 || $len > $KIT_FORM_MAX} { return "" }
    if {[info exists env(CONTENT_TYPE)] && ![string equal $env(CONTENT_TYPE) ""]
            && ![string match "application/x-www-form-urlencoded*" [string tolower $env(CONTENT_TYPE)]]} {
        return ""
    }
    fconfigure stdin -translation binary
    set KIT_FORM_BODY [read stdin $len]
    return $KIT_FORM_BODY
}

proc kit_form {arrayName} {
    upvar $arrayName arr
    array set arr {}
    kit_parse_pairs [kit_form_body] arr
}
