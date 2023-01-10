FROM debian:bullseye as builder

ARG NODE_VERSION={{ .nodeVersion }}
{{ if .yarn -}}
ARG YARN_VERSION={{ .yarnVersion }}
{{ end }}
RUN apt-get update; apt install -y curl python-is-python3 pkg-config build-essential
RUN curl https://get.volta.sh | bash
ENV VOLTA_HOME /root/.volta
ENV PATH /root/.volta/bin:$PATH
{{ if .yarn -}}
RUN volta install node@${NODE_VERSION} yarn@${YARN_VERSION}
{{ else -}}
RUN volta install node@${NODE_VERSION}
{{ end }}
#######################################################################

RUN mkdir /app
WORKDIR /app

{{ if .yarn -}}
# Yarn will not install any package listed in "devDependencies" when NODE_ENV is set to "production"
# to install all modules: "yarn install --production=false"
# Ref: https://classic.yarnpkg.com/lang/en/docs/cli/install/#toc-yarn-install-production-true-false
{{ else -}}
# NPM will not install any package listed in "devDependencies" when NODE_ENV is set to "production",
# to install all modules: "npm install --production=false".
# Ref: https://docs.npmjs.com/cli/v9/commands/npm-install#description
{{ end }}
ENV NODE_ENV production

COPY . .

{{ if .build -}}
RUN {{ .packager }} install && {{ .packager }} run build
{{ else -}}
RUN {{ .packager }} install
{{ end -}}
FROM debian:bullseye

LABEL fly_launch_runtime="nodejs"

COPY --from=builder /root/.volta /root/.volta
COPY --from=builder /app /app

WORKDIR /app
ENV NODE_ENV production
ENV PATH /root/.volta/bin:$PATH

CMD [ "{{ .packager }}", "run", "start" ]
