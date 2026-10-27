#
#   ccu-addon-kit: JSON output (MIT). Tcl 8.2 compatible.
#
#   kit_json_string $s         -> "..." with \ " and every control character escaped
#   kit_json_obj {k t v ...}    -> {"k":v,...}; t: s string, n number, b bool (0/1),
#                                  j raw JSON, z string or null when empty
#   kit_json_list {j ...}       -> [j,...] of values that already are JSON
#   kit_json_strings {s ...}    -> ["s",...]
#
#   The CCU addon handbook's template json_string maps a newline to a raw newline (a braced
#   list turns "\n" into the character itself on both sides), which makes a log line with a
#   line break invalid JSON; this one escapes it.
#

if {[info exists KIT_JSON_LOADED]} { return }
set KIT_JSON_LOADED 1

set KIT_JSON_MAP [list "\\" "\\\\" "\"" "\\\"" "\n" "\\n" "\r" "\\r" "\t" "\\t" "\b" "\\b" "\f" "\\f"]

proc kit_json_string {s} {
    global KIT_JSON_MAP
    set s [string map $KIT_JSON_MAP $s]
    # the remaining control characters, rare enough for a loop; U+2028/9 for script contexts
    if {[regexp "\[\x01-\x1f\x7f\u2028\u2029\]" $s] || [string first "\x00" $s] >= 0} {
        set out ""
        foreach c [split $s ""] {
            scan $c %c code
            if {$code < 32 || $code == 127 || $code == 8232 || $code == 8233} {
                append out [format "\\u%04x" $code]
            } else {
                append out $c
            }
        }
        set s $out
    }
    return "\"$s\""
}

# a number as JSON: integers and decimals only, anything else is null
proc kit_json_number {v} {
    if {[regexp {^-?[0-9]+(\.[0-9]+)?([eE][-+]?[0-9]+)?$} $v]} {
        # no leading zeros in JSON
        regsub {^(-?)0+([0-9])} $v {\1\2} v
        return $v
    }
    return null
}

proc kit_json_value {type value} {
    switch -- $type {
        s { return [kit_json_string $value] }
        n { return [kit_json_number $value] }
        b {
            if {[string equal $value 1] || [string equal $value true]} { return true }
            return false
        }
        j {
            if {[string equal $value ""]} { return null }
            return $value
        }
        z {
            if {[string equal $value ""]} { return null }
            return [kit_json_string $value]
        }
    }
    error "kit_json_value: unknown type $type"
}

proc kit_json_obj {triples} {
    if {[llength $triples] % 3 != 0} {
        error "kit_json_obj: the list must hold name type value triples"
    }
    set parts [list]
    foreach {name type value} $triples {
        lappend parts "[kit_json_string $name]:[kit_json_value $type $value]"
    }
    return "\{[join $parts ,]\}"
}

proc kit_json_list {values} {
    return "\[[join $values ,]\]"
}

proc kit_json_strings {values} {
    set parts [list]
    foreach v $values {
        lappend parts [kit_json_string $v]
    }
    return "\[[join $parts ,]\]"
}

# {de .. en ..} -> {"de":"..","en":".."}
proc kit_json_i18n {pairs} {
    set t [list]
    foreach {k v} $pairs {
        lappend t $k s $v
    }
    return [kit_json_obj $t]
}
