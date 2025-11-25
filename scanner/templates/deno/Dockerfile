# Based on https://github.com/denoland/deno_docker/blob/main/alpine.dockerfile

ARG DENO_VERSION=2.4.5
ARG BIN_IMAGE=denoland/deno:bin-${DENO_VERSION}
FROM ${BIN_IMAGE} AS bin

FROM gcr.io/distroless/cc as cc

FROM alpine:latest

# Inspired by https://github.com/dojyorin/deno_docker_image/blob/master/src/alpine.dockerfile
COPY --from=cc --chown=root:root --chmod=755 /lib/*-linux-gnu/* /usr/local/lib/
COPY --from=cc --chown=root:root --chmod=755 /lib/ld-linux-* /lib/

RUN addgroup --gid 1000 deno \
  && adduser --uid 1000 --disabled-password deno --ingroup deno \
  && mkdir /deno-dir/ \
  && chown deno:deno /deno-dir/ \
  && mkdir /lib64 \
  && ln -s /usr/local/lib/ld-linux-* /lib64/

ENV LD_LIBRARY_PATH="/usr/local/lib"
ENV DENO_USE_CGROUPS=1
ENV DENO_DIR /deno-dir/
ENV DENO_INSTALL_ROOT /usr/local

ARG DENO_VERSION
ENV DENO_VERSION=${DENO_VERSION}
COPY --from=bin /deno /bin/deno

WORKDIR /deno-dir
COPY . .

ENTRYPOINT ["/bin/deno"]
CMD ["run", "-A", "./index.ts"]
