# The service convention, version 1

How an addon's rc.d script behaves, so that one settings frame (and any other tool) can show
and control any addon's service. It extends the rc.d script described in the
[CCU addon handbook](https://github.com/homematic-community/ccu-addon-howto), chapter 03; it
does not change how the firmware calls the script.

The reference implementation is [`classic/sh/rc.d-reference`](../classic/sh/rc.d-reference);
`ccu-addon-kit check-rc` tests a script against this document (below).

## Commands

The script `/usr/local/etc/config/rc.d/<id>` stays a POSIX sh script with one argument.
`init`, `info`, `info.de`, `info.en` and `uninstall` stay as the handbook describes them.

| Command | Must | Exit code | Time limit |
| --- | --- | --- | --- |
| `status` | print **exactly one line of JSON** on stdout (below), nothing else on stdout; stderr is ignored. Cheap: no network, no writes, no start of anything; runnable as a non-root user | `0` running · `3` not running (stopped cleanly or never started) · `1` not running although it should (stale pid file, crashed) · `4` status unknown | ≤ 2 s (callers kill it at 5 s) |
| `start` | start the daemon detached and return; `cd /` first; never leave the daemon's stdout/stderr attached to the caller (a CGI would wait for the pipe to close) | `0` started **or already running** · `1` failed (message on stdout) | ≤ 60 s |
| `stop` | stop the daemon (TERM, wait, KILL as a last resort), remove the pid file | `0` stopped **or was not running** · `1` failed to stop | ≤ 60 s |
| `restart` | `stop`, then `start`; ends with a new process | `0` running afterwards · `1` failed | ≤ 120 s |
| `reload` | re-read the configuration without a new pid (SIGHUP or the daemon's own way) | `0` done · `1` failed · `3` **not supported**; then leave `reload` out of `kit.json`'s `service.commands` | ≤ 15 s |
| anything else (`info.de`, `info.en`, unknown) | exit quietly, no stdout | `0` | — |

The exit codes follow the LSB init-script conventions: `status` answers 0/1/3/4, and a `start`
of a running service or a `stop` of a stopped one is a no-op that succeeds. This differs from
the handbook's older template, which returns 1 for "already running" and "not running".

**Transition rule** for scripts written before v1: a `status` exit code `1` with
`"running":false` and no `state` counts as stopped, and `check-rc --legacy` reports the old exit
codes as warnings, not failures.

## The status line

One JSON object on one line, UTF-8, nothing after it. Numbers are JSON numbers.
[`schema/service-status.schema.json`](../schema/service-status.schema.json) describes it.

| Field | Type | Required | Meaning |
| --- | --- | --- | --- |
| `schema` | integer | **yes** | `1` for this convention |
| `running` | boolean | **yes** | the truth; the exit code is the cross-check, and the JSON wins on a mismatch |
| `state` | string | no | `running`, `stopped`, `failed`, `starting`, `stopping`; when absent: `running` ⇒ running, else stopped |
| `pid` | integer | **when running** | the main process |
| `rss_kb` | integer | **when running** | resident memory in KiB (`VmRSS`), summed over the daemon's process tree when it has children |
| `pids` | integer[] | no | every pid counted in `rss_kb` |
| `since` | integer | no (recommended) | start time of the main process, Unix seconds |
| `version` | string | no (recommended) | the addon's version (`VERSION_ADDON`); equals `info`'s `Version:` |
| `cpu_pct` | number | no | CPU share since the last sample, when the script computes one |
| `lastError` | string | no | when not running: the last error line of the daemon |
| `url` | string | no | where the addon's own UI lives, e.g. `/addons/<id>/app/` |
| `components` | object | no | versions of bundled parts, `{"node":"22.20.0"}` |
| `x_*` | any | no | addon-specific extras; generic tools ignore them |

Unknown fields without the `x_` prefix are ignored, so a later minor version can add fields.

```json
{"schema":1,"running":true,"state":"running","pid":2817,"rss_kb":48320,"since":1761562800,"version":"0.4.0"}
{"schema":1,"running":false,"state":"stopped","version":"0.4.0"}
{"schema":1,"running":false,"state":"failed","version":"0.4.0","lastError":"listen tcp 127.0.0.1:8091: bind: address already in use"}
```

Strings that can hold arbitrary text (`lastError`, `version`) go through the reference script's
`json_str`, which drops control characters and escapes `\` and `"`. Never print raw log text into
the line.

## The reference script

[`classic/sh/rc.d-reference`](../classic/sh/rc.d-reference) is a complete script for a daemon
that does not fork itself. Copy it, change the variables at the top (`ADDON`, `NAME`, `DAEMON`,
`DAEMON_ARGS`, `LOGFILE`), and keep the rest. What it does:

- the pid file lives in the addon's tree (`$ADDON_DIR/var/`): the CCU3's install chroot has no
  `/var/run`, and on openccu-lite a confined addon cannot write there;
- `Pid` accepts the pid only when `/proc/<pid>/cmdline` names the daemon, so a stale pid file
  after a reboot never matches an unrelated process;
- `status` derives `since` from `/proc/<pid>/stat` and `btime`, `rss_kb` from `VmRSS`, and
  `lastError` from the daemon's log or the syslog;
- `start` touches `.nobackup` in `bin`, `lib`, `www` (the CCU backup skips them), rotates the log
  once it passes `LOG_MAX_KB`, starts the daemon with its output in the log, waits one second and
  reports a failed start; inside openccu-lite's service unit the output goes to the journal;
- `stop` sends TERM, waits up to 20 s, then KILL;
- `info` names the kit's `settings.cgi` as `Config-Url` and `update_check.cgi` as `Update`.

A daemon that forks itself, or one managed by `start-stop-daemon -b -m`, replaces the two
background starts; the convention fixes the commands' behaviour, not how the script gets there.

[`classic/sh/rc-helpers.sh`](../classic/sh/rc-helpers.sh) has optional helpers for a process
tree: `Descendants <pid>`, `RssTreeKb <pid>`, `PidsJson <pid>`, and `JsonStr`.

## The conformance test

```sh
npx ccu-addon-kit check-rc --exec "docker exec <container>" --rc /usr/local/etc/config/rc.d/<id> --log /var/log/<id>.log
npx ccu-addon-kit check-rc --exec "ssh root@<ccu>" --rc /usr/local/etc/config/rc.d/<id> [--legacy] [--no-reload] [--json]
```

It needs only `sh` on the system and runs every step through the command prefix. It stops and
starts the service, so run it on a test system. At the end it takes C12's line out of the log
again and starts the service when it found it running.

| # | Step | Passes when |
| --- | --- | --- |
| C1 | `stop` (setup), then `status` | exit 3 (legacy: 1 = warning), one line, valid JSON, `schema:1`, `running:false` |
| C2 | `start` | exit 0 within 60 s; the command returns |
| C3 | `status` (retried up to 30 s) | exit 0, `running:true`, the `pid` exists, `rss_kb` > 0, `since` within the last 120 s when present |
| C4 | `start` again | exit 0 (legacy: 1 = warning), the same `pid` |
| C5 | `reload` | exit 0 and the same pid, or exit 3 with `--no-reload` |
| C6 | `restart` | exit 0 within 120 s, `running:true`, a different pid |
| C7 | `stop` | exit 0 within 60 s; `status` exit 3; the old pid and its children are gone |
| C8 | `stop` again | exit 0 (legacy: 1 = warning) |
| C9 | `info.de`, `info.en`, `nonsense` | exit 0, no stdout |
| C10 | `info` | `Name:` present; `Version:` equals the `version` of `status` |
| C11 | every `status` call | ≤ 2 s (a warning above, a failure at 5 s), exactly one line |
| C12 | `status` after a line with `"`, `\` and a tab was appended to `--log` | still valid JSON |

The timings subtract the cost of the prefix itself (measured with `true`).

## Moving an existing script to v1

- `status`: print the line above; exit 3 when stopped, 1 only for a stale pid file or a crash.
- `start`/`stop` on a running/stopped service: exit 0.
- `reload`: implement it, or exit 3 and leave it out of `kit.json`.
- Run `check-rc --legacy` first, then without `--legacy` once the warnings are gone.

The convention's version is the `schema` field. A kit minor release never changes it; a new
convention version comes with a kit major release.
