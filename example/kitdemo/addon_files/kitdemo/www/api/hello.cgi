#!/bin/tclsh
# www/api/hello.cgi - the example tab's CGI (ccu-addon-kit, MIT): GET reads the greeting,
# POST greeting=... writes it
set KIT_HOME /usr/local/addons/kitdemo
if {[info exists env(KIT_HOME)] && [info exists env(KIT_TEST)]} { set KIT_HOME $env(KIT_HOME) }
source $KIT_HOME/lib/kit/cgi.tcl

kit_main {
    if {[kit_request_is_post]} {
        kit_require write
        kit_form f
        if {![info exists f(greeting)] || [string length $f(greeting)] > 80} {
            kit_fail 400 bad-request "greeting missing or too long"
        }
        set fd [open $KIT_HOME/etc/greeting.txt w]
        fconfigure $fd -encoding utf-8
        puts -nonewline $fd $f(greeting)
        close $fd
        kit_ok [list saved b 1]
    }
    kit_require read
    kit_ok [list greeting s [kit_read_file $KIT_HOME/etc/greeting.txt 1024]]
}
