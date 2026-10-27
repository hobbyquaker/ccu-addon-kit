# Tcl 8.2.3 - the interpreter of eQ-3's CCU3 firmware - built from source, with Node for the test
# runner and the stub tclrega.so built against the 8.2 headers. The kit's Tcl tests run under it
# in CI (job tcl82), so a construct newer than 8.2 fails a test, not only the lint.
#
#   docker build -f test/docker/tcl82.Dockerfile -t ccu-addon-kit-tcl82 test/
#
# Tcl 8.2.3 is from 1999: it builds with a current gcc only as gnu89 with -fcommon and the
# implicit-declaration errors turned back into warnings; its configure script needs bash (dash
# stops at the dynamic-loading check) and the build triplet spelled out. The release tarball is
# used because the git tag has no generated configure script.

FROM debian:bookworm-slim AS build
RUN apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
        ca-certificates curl gcc libc6-dev make \
    && rm -rf /var/lib/apt/lists/*
ARG TCL_URL=https://downloads.sourceforge.net/project/tcl/Tcl/8.2.3/tcl8.2.3.tar.gz
ARG TCL_SHA256=e0d354e784a4b129244a13c79d678b4817d5104e5cb4f9c13d87470accc674fb
RUN mkdir /src && curl -fsSL -o /tmp/tcl.tgz "$TCL_URL" \
    && echo "$TCL_SHA256  /tmp/tcl.tgz" | sha256sum -c - \
    && tar -xzf /tmp/tcl.tgz -C /src --strip-components=2
WORKDIR /src/unix
RUN CFLAGS="-O2 -std=gnu89 -fcommon -Wno-implicit-function-declaration -Wno-implicit-int -Wno-int-conversion -Wno-incompatible-pointer-types" \
        CONFIG_SHELL=/bin/bash bash ./configure --prefix=/opt/tcl82 --enable-shared --build=x86_64-unknown-linux-gnu \
    && make -j"$(nproc)" CFLAGS_OPTIMIZE="-O2 -std=gnu89 -fcommon -Wno-implicit-function-declaration -Wno-implicit-int -Wno-int-conversion -Wno-incompatible-pointer-types" \
    && make install \
    && ln -s tclsh8.2 /opt/tcl82/bin/tclsh
COPY tcl/tclrega-stub.c /tmp/tclrega-stub.c
RUN gcc -std=gnu89 -shared -fPIC -I/opt/tcl82/include -o /opt/tcl82/lib/tclrega.so /tmp/tclrega-stub.c

FROM node:22-bookworm-slim
COPY --from=build /opt/tcl82 /opt/tcl82
RUN apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends busybox procps \
    && rm -rf /var/lib/apt/lists/* \
    && echo 'puts [info patchlevel]' | /opt/tcl82/bin/tclsh | grep -qx 8.2.3
ENV TCLSH=/opt/tcl82/bin/tclsh LD_LIBRARY_PATH=/opt/tcl82/lib KIT_TCLREGA_DIR=/opt/tcl82/lib
