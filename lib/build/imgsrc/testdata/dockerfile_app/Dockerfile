FROM nginx:alpine

RUN sed -i 's/    listen       80;/    listen       8080;/g' /etc/nginx/conf.d/default.conf

ARG SOURCE_DIR=.
ARG APP_DIR=/usr/share/nginx/html
ARG ENTRY_FILE=index.html

COPY ${SOURCE_DIR} ${APP_DIR}

ENV NGINX_PORT=8080
