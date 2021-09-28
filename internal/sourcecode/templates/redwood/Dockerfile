FROM node:14-alpine as base

WORKDIR /app

COPY package.json package.json
COPY web/package.json web/package.json
COPY api/package.json api/package.json
COPY yarn.lock yarn.lock
RUN yarn install --frozen-lockfile

COPY redwood.toml .
COPY graphql.config.js .
COPY babel.config.js .

FROM base as web_build

COPY web web
RUN yarn rw build web

FROM base as api_build

COPY api api
RUN yarn rw build api

FROM node:14-alpine

WORKDIR /app

# Only install API packages to keep image small
COPY api/package.json .

RUN yarn install && yarn add react react-dom @redwoodjs/api-server @redwoodjs/internal

COPY graphql.config.js .
COPY redwood.toml .
COPY api api

COPY --from=web_build /app/web/dist /app/web/dist
COPY --from=api_build /app/api/dist /app/api/dist
COPY --from=api_build /app/node_modules/.prisma /app/node_modules/.prisma

EXPOSE 8911

# Entrypoint to @redwoodjs/api-server binary
CMD [ "yarn", "rw-api-server", "--port", "8911", "--rootPath", "/api" ]
