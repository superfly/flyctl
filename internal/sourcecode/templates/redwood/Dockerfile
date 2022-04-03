ARG BASE_IMAGE=node:16.13.0-alpine
FROM ${BASE_IMAGE} as base

RUN corepack enable
RUN yarn set version 3.2.0 && rm /package.json

RUN mkdir /app
WORKDIR /app

# Required for building the api and web distributions
ENV NODE_ENV development

FROM base as dependencies

COPY .yarnrc.yml .yarnrc.yml
COPY package.json package.json
COPY web/package.json web/package.json
COPY api/package.json api/package.json
COPY yarn.lock yarn.lock

RUN yarn install --immutable

COPY redwood.toml .
COPY graphql.config.js .

FROM dependencies as web_build

COPY web web
RUN yarn rw build web

FROM dependencies as api_build

COPY api api
RUN yarn rw build api

FROM dependencies

ENV NODE_ENV production
# Only install API packages to keep image small

COPY --from=web_build /app/web/dist /app/web/dist
COPY --from=api_build /app/api /app/api
COPY --from=api_build /app/node_modules/.prisma /app/node_modules/.prisma

COPY .fly .fly

ENTRYPOINT ["sh"]
CMD [".fly/start.sh"]


