#
#   update_check.cgi - the Zusatzsoftware page's update check (MIT, docs/cgi-api.md; the CCU
#   addon handbook, chapter 04). Plain text: ?cmd=check_version -> the newest version or n/a;
#   ?cmd=download -> a redirect to the package for this system (kit.json's update.assetHint, by
#   uname -m) or else to the release page. Shares kit/version.cgi's cache. No
#   session: the WebUI's XMLHttpRequest and OpenCCU's checkAddonUpdates.sh send none.
#
source $KIT_HOME/lib/kit/release.tcl

kit_main {
    kit_query q
    set cmd ""
    if {[info exists q(cmd)]} { set cmd $q(cmd) }
    set r [kit_release 0]
    array set rel $r
    if {[string equal $cmd download]} {
        set url $rel(url)
        # the asset for this system, when kit.json names its pattern and the release's tag is known
        set arch [kit_package_arch]
        if {![string equal $KIT(assetHint) ""] && ![string equal $arch ""] && ![string equal $rel(latest) ""]
                && [regexp {^(https://github\.com/[^/]+/[^/]+)/releases/tag/([^/?#]+)$} $url all base tag]} {
            regsub -all {\{arch\}} $KIT(assetHint) $arch asset
            regsub -all {\{version\}} $asset $rel(latest) asset
            set url $base/releases/download/$tag/$asset
        }
        if {[string equal $url ""] && ![string equal $KIT(github) ""]} {
            set url https://github.com/$KIT(github)/releases/latest
        }
        if {[string equal $url ""]} {
            kit_header 404 "text/plain; charset=utf-8"
            kit_body "n/a"
            exit 0
        }
        kit_header 302 "text/html; charset=utf-8" [list Location $url]
        kit_body "<html><head><meta http-equiv=\"refresh\" content=\"0; url=$url\"></head></html>\n"
        exit 0
    }
    kit_header 200 "text/plain; charset=utf-8"
    if {[string equal $rel(latest) ""]} {
        kit_body "n/a"
    } else {
        kit_body $rel(latest)
    }
}
