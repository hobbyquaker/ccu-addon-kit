#
#   settings.cgi - the addon's Config-Url (MIT, docs/cgi-api.md). Answers the frame's static
#   www/frame/index.html without a session check: the page holds no data, its first request
#   (kit/session.cgi) decides, and every data request is checked.
#
source $KIT_HOME/lib/kit/cgi.tcl

kit_main {
    set file $KIT_HOME/www/frame/index.html
    if {![file readable $file]} {
        kit_fail 503 internal "the frame is not installed (www/frame/index.html)"
    }
    kit_header 200 "text/html; charset=utf-8" {Referrer-Policy no-referrer}
    set fd [open $file r]
    fconfigure $fd -translation binary
    fconfigure stdout -translation binary
    puts -nonewline stdout [read $fd]
    close $fd
    flush stdout
}
