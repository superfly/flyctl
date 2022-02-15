ARG BASE_IMAGE=node:14.19.0-bullseye-slim
FROM $BASE_IMAGE as base

WORKDIR /app

COPY package.json package.json
COPY web/package.json web/package.json
COPY api/package.json api/package.json
COPY yarn.lock yarn.lock
RUN yarn install --frozen-lockfile

COPY redwood.toml .
COPY graphql.config.js .

FROM base as web_build

COPY web web
RUN yarn rw build web

FROM base as api_build

COPY api api
RUN yarn rw build api

FROM $BASE_IMAGE
RUN apt-get update && apt-get install -y nginx bash

RUN curl -L https://github.com/DarthSim/overmind/releases/download/v2.2.2/overmind-v2.2.2-linux-amd64.gz -o overmind.gz \
    && gunzip overmind.gz \
    && mv overmind /usr/local/bin

RUN chmod +x /usr/local/bin/hivemind

WORKDIR /app

# Only install API packages to keep image small
COPY api/package.json .

RUN yarn install && yarn add react react-dom @redwoodjs/api-server @redwoodjs/internal prisma

COPY graphql.config.js .
COPY redwood.toml .
COPY api api

COPY --from=web_build /app/web/dist /app/web/dist
COPY --from=api_build /app/api/dist /app/api/dist
COPY --from=api_build /app/api/db /app/api/db
COPY --from=api_build /app/node_modules/.prisma /app/node_modules/.prisma

EXPOSE 8911 8080
COPY .fly .fly
CMD ["/usr/local/bin/overmind", ".fly/Procfile"]
