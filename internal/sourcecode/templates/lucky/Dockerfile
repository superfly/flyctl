FROM crystallang/crystal:1.3.2-alpine as crystal_dependencies
ENV LUCKY_ENV=production
ENV SKIP_LUCKY_TASK_PRECOMPILATION=1
WORKDIR /shards
COPY shard.* ./
RUN  shards install --production

FROM node:alpine as asset_build
WORKDIR /assets
COPY . .
RUN yarn install
RUN yarn prod

FROM crystallang/crystal:1.3.2-alpine as lucky_tasks_build
ENV LUCKY_ENV=production
RUN apk --no-cache add yaml-static
COPY . .
COPY --from=crystal_dependencies /shards/lib lib
COPY --from=asset_build /assets/public public
RUN crystal build --static --release tasks.cr -o /usr/local/bin/lucky

FROM crystallang/crystal:1.3.2-alpine as lucky_webserver_build
WORKDIR /webserver_build
RUN apk --no-cache add yaml-static coreutils
ENV LUCKY_ENV=production
COPY . .
COPY --from=crystal_dependencies /shards/lib lib
COPY --from=asset_build /assets/public public
RUN shards build --production --static --release
RUN mv ./bin/`ls ./bin/ | head -1` /usr/local/bin/webserver

FROM alpine as webserver
WORKDIR /app
RUN apk --no-cache add postgresql-client tzdata
COPY --from=lucky_tasks_build /usr/local/bin/lucky /usr/local/bin/lucky
COPY --from=lucky_webserver_build /usr/local/bin/webserver webserver
COPY --from=asset_build /assets/public public

ENV PORT 8080
CMD ["./webserver"]
