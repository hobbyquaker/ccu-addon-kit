#!/bin/sh
#
#   rc.d script following the ccu-addon-kit service convention v1 (MIT).
#   Linked as /usr/local/etc/config/rc.d/$ADDON. init/start/stop come from the
#   firmware (S55InitAddons, S98StartAddons), info/restart/uninstall from the
#   Zusatzsoftware page, start/stop/restart/reload/status from the kit's
#   service.cgi. status prints one JSON line; exit 0 running, 3 stopped,
#   1 failed, 4 unknown.
#

ADDON=kitdemo
NAME="Kit Demo"
CONF_DIR=/usr/local/etc/config
ADDON_DIR=/usr/local/addons/$ADDON
WWW_DIR=$CONF_DIR/addons/www/$ADDON
# in the addon's tree: the CCU3's install chroot has no /var/run, and on
# openccu-lite a confined addon cannot write there
PIDFILE=$ADDON_DIR/var/$ADDON.pid
DAEMON=$ADDON_DIR/bin/$ADDON
DAEMON_ARGS="-c $ADDON_DIR/etc/$ADDON.conf"
# classic only: the daemon's output; on openccu-lite it goes to the journal
LOGFILE=/var/log/$ADDON.log
LOG_MAX_KB=1024

VERSION_ADDON=unknown
[ -r $ADDON_DIR/versions ] && . $ADDON_DIR/versions

log() { logger -t $ADDON -p daemon.$1 "$2"; }

# JSON string content: control characters dropped, \ and " escaped
json_str() { printf '%s' "$1" | tr -d '\000-\037' | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g'; }

# inside openccu-lite's addon-$ADDON.service unit?
InUnit() { grep -q "addon-$ADDON.service" /proc/self/cgroup 2>/dev/null; }

# pid of the running daemon, empty when not running (the pid file is left alone)
Pid() {
    local pid
    [ -f $PIDFILE ] || return 1
    pid=`cat $PIDFILE 2>/dev/null`
    case "$pid" in ''|*[!0-9]*) return 1 ;; esac
    [ -d /proc/$pid ] || return 1
    case "`tr '\0' ' ' < /proc/$pid/cmdline 2>/dev/null`" in
        *$DAEMON*) echo $pid; return 0 ;;   # also "/bin/sh $DAEMON ..." for a script daemon
    esac
    return 1
}

# start time of a pid in Unix seconds: btime + starttime/USER_HZ (100 on every CCU kernel);
# the comm field may contain spaces, so it is cut off before counting
StartEpoch() {
    local ticks btime
    ticks=`sed -e 's/^.*) //' /proc/$1/stat 2>/dev/null | awk '{print $20}'`
    btime=`awk '/^btime/ {print $2}' /proc/stat`
    [ -n "$ticks" ] && [ -n "$btime" ] && echo $((btime + ticks / 100))
}

RssKb() { awk '/^VmRSS:/ {print $2}' /proc/$1/status 2>/dev/null; }

LastError() {
    local src
    if [ -f $LOGFILE ]; then src=$LOGFILE; else src=/var/log/messages; fi
    grep -i "error\|fatal\|failed\|unable" $src 2>/dev/null | grep "$ADDON" | tail -1 | cut -c1-300
}

Status() {
    local pid rss since state code
    pid=`Pid`
    if [ -z "$pid" ]; then
        state=stopped; code=3
        if [ -f $PIDFILE ]; then state=failed; code=1; fi
        printf '{"schema":1,"running":false,"state":"%s","version":"%s","lastError":"%s"}\n' \
            $state "`json_str "$VERSION_ADDON"`" "`json_str "$(LastError)"`"
        return $code
    fi
    rss=`RssKb $pid`
    since=`StartEpoch $pid`
    printf '{"schema":1,"running":true,"state":"running","pid":%s,"rss_kb":%s,"since":%s,"version":"%s"}\n' \
        $pid ${rss:-0} ${since:-null} "`json_str "$VERSION_ADDON"`"
    return 0
}

Start() {
    local dir
    for dir in bin lib www; do [ -d $ADDON_DIR/$dir ] && touch $ADDON_DIR/$dir/.nobackup; done
    if [ -n "`Pid`" ]; then echo "$ADDON already running"; return 0; fi
    rm -f $PIDFILE
    mkdir -p $ADDON_DIR/var
    cd /
    log info "Starting $ADDON $VERSION_ADDON"
    if InUnit; then
        # openccu-lite: let the output reach the journal
        $DAEMON $DAEMON_ARGS &
    else
        if [ -f $LOGFILE ] && [ "`du -k $LOGFILE | cut -f1`" -gt $LOG_MAX_KB ]; then mv -f $LOGFILE $LOGFILE.0; fi
        $DAEMON $DAEMON_ARGS >>$LOGFILE 2>&1 </dev/null &
    fi
    echo $! > $PIDFILE
    sleep 1
    if [ -z "`Pid`" ]; then echo "Starting $ADDON: FAILED"; log err "start failed"; return 1; fi
    echo "Starting $ADDON: OK"
}

Stop() {
    local pid i
    pid=`Pid`
    if [ -z "$pid" ]; then rm -f $PIDFILE; echo "$ADDON not running"; return 0; fi
    log info "Stopping $ADDON (pid $pid)"
    kill $pid 2>/dev/null
    i=0
    while [ $i -lt 40 ] && [ -d /proc/$pid ]; do sleep 0.5; i=$((i + 1)); done
    if [ -d /proc/$pid ]; then log warn "Killing $ADDON (pid $pid)"; kill -9 $pid 2>/dev/null; sleep 1; fi
    [ -d /proc/$pid ] && { echo "Stopping $ADDON: FAILED"; return 1; }
    rm -f $PIDFILE
    echo "Stopping $ADDON: OK"
}

case "$1" in
    init) ;;
    start) Start; exit $? ;;
    stop) Stop; exit $? ;;
    restart) Stop; sleep 1; Start; exit $? ;;
    reload)
        pid=`Pid`
        [ -n "$pid" ] || { echo "$ADDON not running"; exit 1; }
        kill -HUP $pid && echo "Reloading $ADDON: OK" ;;
    status) Status; exit $? ;;
    info)
        echo "Info: <div>$NAME</div>"
        echo "Name: $NAME"
        echo "Version: $VERSION_ADDON"
        echo "Update: /addons/$ADDON/update_check.cgi"
        echo "Config-Url: /addons/$ADDON/settings.cgi"
        echo "Operations: restart uninstall"
        ;;
    uninstall)
        Stop
        $ADDON_DIR/bin/update_addon $ADDON
        rm -f $CONF_DIR/rc.d/$ADDON $WWW_DIR
        rm -f $CONF_DIR/lighttpd/$ADDON.conf
        rm -f $LOGFILE $LOGFILE.0 /tmp/kit-$ADDON-*
        rm -rf $ADDON_DIR
        log info "Uninstalled $ADDON"
        ;;
    *) exit 0 ;;
esac
exit 0
