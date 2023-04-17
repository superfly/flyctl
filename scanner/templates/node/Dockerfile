ARG NODE_VERSION={{ .nodeVersion }}

FROM node:${NODE_VERSION}-slim as builder

{{ if .yarn -}}
ARG YARN_VERSION={{ .yarnVersion }}
{{ end -}}
RUN apt-get update; apt install -y curl python-is-python3 pkg-config build-essential {{ if .prisma }}openssl {{ end }}{{ if .yarn -}} && \
    npm install -g yarn@$YARN_VERSION{{ end }}

RUN mkdir /app
WORKDIR /app

{{ if .prisma -}}
ADD prisma .
RUN npx prisma generate

{{ end -}}
COPY . .

{{ if .build -}}
RUN {{ .packager }} install && {{ .packager }} run build
{{ else -}}
RUN {{ .packager }} install
{{ end }}

FROM node:${NODE_VERSION}-slim

LABEL fly_launch_runtime="{{ .runtime }}"

COPY --from=builder /app /app

WORKDIR /app
ENV NODE_ENV production

{{ if .yarn -}}
RUN yarn install --production
{{ else -}}
RUN npm prune --production
{{ end -}}

{{ if .prisma }}
CMD [ "sh", "start_with_migrations.sh" ]
{{ else }}
CMD [ "{{ .packager }}", "run", "start" ]
{{ end -}}
