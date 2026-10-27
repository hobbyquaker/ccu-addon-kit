# The kit's test image with the distribution's Tcl 8.6 (what OpenCCU and openccu-lite run),
# Node for the runner, shellcheck and busybox for the shell checks, and the stub tclrega.so.
#
#   docker build -f test/docker/tcl86.Dockerfile -t ccu-addon-kit-tcl86 test/
FROM node:22-bookworm-slim
RUN apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
        busybox gcc libc6-dev procps shellcheck tcl tcl-dev \
    && rm -rf /var/lib/apt/lists/*
COPY tcl/tclrega-stub.c /tmp/tclrega-stub.c
RUN mkdir -p /usr/local/lib/kit \
    && gcc -shared -fPIC -I/usr/include/tcl -o /usr/local/lib/kit/tclrega.so /tmp/tclrega-stub.c \
    && rm /tmp/tclrega-stub.c
ENV TCLSH=tclsh KIT_TCLREGA_DIR=/usr/local/lib/kit
