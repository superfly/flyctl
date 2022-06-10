# syntax = docker/dockerfile:experimental
FROM alpine:edge as base

LABEL fly_launch_runtime="laravel"

RUN apk update \
    && apk add curl zip unzip tzdata supervisor nginx htop vim ca-certificates \
           php8           php8-cli        php8-pecl-mcrypt \
           php8-soap      php8-openssl    php8-gmp \
           php8-pdo_odbc  php8-json       php8-dom \
           php8-pdo       php8-zip        php8-pdo_mysql \
           php8-sqlite3   php8-pdo_pgsql  php8-bcmath \
           php8-gd        php8-odbc       php8-pdo_sqlite \
           php8-gettext   php8-xmlreader  php8-bz2 \
           php8-iconv     php8-pdo_dblib  php8-curl \
           php8-ctype     php8-phar       php8-xml \
           php8-common    php8-mbstring   php8-tokenizer \
           php8-xmlwriter php8-fileinfo   php8-opcache \
           php8-simplexml php8-pecl-redis php8-sockets \
           php8-pcntl     php8-posix      php8-pecl-swoole \
           php8-fpm \
    && cp /etc/nginx/nginx.conf /etc/nginx/nginx.old.conf \
    && rm -rf /etc/nginx/http.d/default.conf \
    && curl -sS https://getcomposer.org/installer | php -- --install-dir=/usr/local/bin --filename=composer \
    && adduser -D -u 1000 -g 'app' app \
    && addgroup nginx app \
    && mkdir -p /var/run/php \
    && chown -R app:app /var/run/php \
    && mkdir -p /var/www/html

WORKDIR /var/www/html
# copy application code, skipping files based on .dockerignore
COPY . /var/www/html

# Install dependencies, configure server
RUN composer update && composer install --optimize-autoloader --no-dev \
    && mkdir -p storage/logs \
    && chown -R app:app /var/www/html \
    && /usr/bin/crontab docker/crontab \
    && mv docker/supervisor.conf /etc/supervisord.conf \
    && mv docker/nginx.conf /etc/nginx/nginx.conf \
    && mv docker/server.conf /etc/nginx/server.conf \
    && mv docker/php.ini /etc/php8/conf.d/php.ini

# If we're not using Octane, configure php-fpm
RUN if ! grep -Fq "laravel/octane" /var/www/html/composer.json; then \
        rm -rf /etc/php8/php-fpm.conf; \
        rm -rf /etc/php8/php-fpm.d/www.conf; \
        mv docker/php-fpm.conf /etc/php8/php-fpm.conf; \
        mv docker/app.conf /etc/php8/php-fpm.d/app.conf; \
    elif grep -Fq "spiral/roadrunner" /var/www/html/composer.json; then \
        if [ -f ./vendor/bin/rr ]; then ./vendor/bin/rr get-binary; fi; \
        rm -f .rr.yaml; \
    fi

# clear Laravel cache that may be left over
RUN composer dump-autoload \
    && php artisan optimize:clear \
    && chmod -R ug+w /var/www/html/storage \
    && chmod -R 755 /var/www/html

# Multi-stage build: Build static assets
FROM node:14 as node_modules_go_brrrr

RUN mkdir /app

RUN mkdir -p  /app
WORKDIR /app
COPY . .
RUN if [ -f "yarn.lock" ]; then \
        yarn install; \
    elif [ -f "package-lock.json" ]; then \
        npm ci --no-audit; \
    else \
        npm install; \
    fi

# Create final container, adding in static assets
FROM base

COPY --from=node_modules_go_brrrr /app/public /var/www/html/public

# The same port nginx.conf is set to listen on and fly.toml references (standard is 8080)
EXPOSE 8080

ENTRYPOINT ["/var/www/html/docker/run.sh"]
