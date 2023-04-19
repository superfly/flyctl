# syntax = docker/dockerfile:1

ARG NODE_VERSION={{ .nodeVersion }}
FROM node:${NODE_VERSION}-slim as base

LABEL fly_launch_runtime="{{ .runtime }}"

ENV NODE_ENV production

{{ if .yarn -}}
ARG YARN_VERSION={{ .yarnVersion }}
RUN npm install -g yarn@$YARN_VERSION

{{ end -}}
RUN mkdir /app
WORKDIR /app


FROM base as build

RUN apt-get update; && \
    apt install -y python-is-python3 pkg-config build-essential {{ 
    if .prisma }}openssl {{ end }}

COPY {{ .package_files }} .
RUN {{ .packager }} install{{ if .devDependencies }} --production=false{{ end }}

{{ if .prisma -}}
ADD prisma .
RUN npx prisma generate

{{ end -}}
COPY . .
{{ if .build -}}
    
RUN {{ .packager }} run build
{{ end -}}
{{ if .devDependencies -}}

{{ if .yarn -}}
RUN yarn install --production=true
{{ else -}}
RUN npm prune --production
{{ end -}}
{{ end }}

FROM base

COPY --from=build /app /app

{{ if .prisma -}}
CMD [ "sh", "start_with_migrations.sh" ]
{{ else -}}
CMD [ "{{ .packager }}", "run", "start" ]
{{ end -}}
