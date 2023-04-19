# syntax = docker/dockerfile:1

# Adjust NODE_VERSION as desired
ARG NODE_VERSION={{ .nodeVersion }}
FROM node:${NODE_VERSION}-slim as base

LABEL fly_launch_runtime="{{ .runtime }}"

# {{ .runtime }} app lives here
WORKDIR /app

# Set production environment
ENV NODE_ENV=production
{{ if .yarn -}}

ARG YARN_VERSION={{ .yarnVersion }}
RUN npm install -g yarn@$YARN_VERSION
{{ end }}

# Throw-away build stage to reduce size of final image
FROM base as build

# Install packages needed to build node modules
RUN apt-get update -qq && \
    apt-get install -y python-is-python3 pkg-config build-essential {{
    if .prisma }}openssl {{ end }}

# Install node modules
COPY --link {{ .package_files }} .
RUN {{ .packager }} install{{ if .devDependencies }} --production=false{{ end }}

{{ if .prisma -}}
# Generate Prisma Client
COPY --link prisma .
RUN npx prisma generate

{{ end -}}
# Copy application code
COPY --link . .

{{ if .build -}}
# Build application
RUN {{ .packager }} run build

{{ end -}}
{{ if .devDependencies -}}
# Remove development dependencies
{{ if .yarn -}}
RUN yarn install --production=true
{{ else -}}
RUN npm prune --production
{{ end -}}

{{ end }}

# Final stage for app image
FROM base

# Copy built application
COPY --from=build /app /app

{{ if .prisma -}}
# Entrypoint prepares the database.
ENTRYPOINT ["/app/docker-entrypoint"]

{{ end -}}
# Start the server by default, this can be overwritten at runtime
CMD [ "{{ .packager }}", "run", "start" ]
