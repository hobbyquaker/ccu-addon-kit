# A container close enough to OpenCCU for the kit's end-to-end test (the CCU addon handbook,
# chapter 08 sections 2 and 2b): busybox as /bin/sh and for the tools the rc.d script calls,
# busybox syslogd writing /var/log/messages, tclsh as /bin/tclsh, lighttpd with the firmware's
# /addons and .cgi rules, and the kit's stub tclrega.so.
#
#   docker build -f test/e2e/Dockerfile -t ccu-addon-kit-e2e test/
#
# What it cannot test: Tcl 8.2 and busybox 1.29 of the CCU3 firmware (job tcl82 and the lab),
# the CCU3's chroot install, the real tclrega.so.
FROM debian:bookworm-slim
RUN apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
        busybox ca-certificates curl gcc libc6-dev lighttpd tcl tcl-dev \
    && rm -rf /var/lib/apt/lists/*
COPY tcl/tclrega-stub.c /tmp/tclrega-stub.c
RUN gcc -shared -fPIC -I/usr/include/tcl -o /usr/lib/tclrega.so /tmp/tclrega-stub.c \
    && rm /tmp/tclrega-stub.c && ldconfig \
    && apt-get purge -y gcc libc6-dev tcl-dev && apt-get autoremove -y >/dev/null
RUN mkdir -p /usr/local/addons /usr/local/tmp /usr/local/etc/config/rc.d /usr/local/etc/config/addons/www \
        /usr/local/etc/config/lighttpd /www \
    && ln -sfn /usr/local/etc/config /etc/config \
    && ln -sfn /etc/config/addons/www /www/addons \
    && touch /usr/local/etc/config/hm_addons.cfg \
    && { [ -L /bin ] || ln -sf /usr/bin/tclsh /bin/tclsh; } \
    && printf 'PRODUCT=raspmatic_ova\nVERSION=3.89.8.20260719\nPLATFORM=x86_64\n' > /VERSION
COPY e2e/lighttpd-ccu.conf /etc/lighttpd/lighttpd-ccu.conf
# busybox last: the shell and the tools of the rc.d script with busybox semantics
RUN for a in sh ash awk sed tr grep cut du tail head wc logger date sleep kill ps; do ln -sf /usr/bin/busybox /usr/bin/$a; [ -L /bin ] || ln -sf /usr/bin/busybox /bin/$a; done
CMD ["sh", "-c", "busybox syslogd -n -O /var/log/messages & lighttpd -D -f /etc/lighttpd/lighttpd-ccu.conf & wait"]
