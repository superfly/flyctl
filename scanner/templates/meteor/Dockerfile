FROM debian:bullseye-slim AS builder

RUN apt-get update && apt-get install -y --no-install-recommends \
    curl \
    ca-certificates \
    python3 \
    python3-pip \
    make \
    g++ \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /home/meteor

ARG RELEASE={{ .meteorVersion }}

RUN curl -s https://install.meteor.com/?release=$RELEASE | bash

ENV PATH="${PATH}:/home/meteor/.meteor"

USER root

COPY ./package*.json .

RUN meteor npm install

COPY . .

RUN meteor build --allow-superuser --server-only --directory bundle

# Hacky. Remove at some point: https://github.com/meteor/meteor/issues/12932
RUN chmod 777 bundle/bundle/programs/server/npm-shrinkwrap.json

RUN cd bundle/bundle/programs/server && meteor npm install

FROM node:20-bullseye-slim AS final

LABEL fly_launch_runtime="meteor"

RUN apt-get update && apt-get install -y --no-install-recommends

RUN useradd --no-create-home --shell /bin/bash meteor

USER meteor

WORKDIR /app

COPY --from=builder /app/bundle/bundle .

ENV PORT=3000

CMD exec node main.js
