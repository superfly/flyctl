FROM debian:bullseye as builder

ENV PATH=/usr/local/node/bin:$PATH
ARG NODE_VERSION={{ .nodeVersion }}
{{ if .yarn -}}
ARG YARN_VERSION={{ .yarnVersion }}
{{ end }}
RUN apt-get update; apt install -y curl python-is-python3 pkg-config build-essential && \
    curl -sL https://github.com/nodenv/node-build/archive/master.tar.gz | tar xz -C /tmp/ && \
    /tmp/node-build-master/bin/node-build "${NODE_VERSION}" /usr/local/node && \
{{ if .yarn -}}
    npm install -g yarn@$YARN_VERSION && \
{{ end -}}
    rm -rf /tmp/node-build-master

RUN mkdir /app
WORKDIR /app

COPY . .

{{ if .build -}}
RUN {{ .packager }} install && {{ .packager }} run build
{{ else -}}
RUN {{ .packager }} install
{{ end }}

FROM debian:bullseye-slim

LABEL fly_launch_runtime="nodejs"

COPY --from=builder /usr/local/node /usr/local/node
COPY --from=builder /app /app

WORKDIR /app
ENV NODE_ENV production
ENV PATH /usr/local/node/bin:$PATH

CMD [ "{{ .packager }}", "run", "start" ]
