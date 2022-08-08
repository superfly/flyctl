# syntax = docker/dockerfile:experimental

# The "base" container installs PHP, etc
FROM alpine:3.16 as base

LABEL fly_launch_runtime="laravel"

RUN apk update \
    && apk add curl zip unzip tzdata supervisor nginx htop vim ca-certificates rsync \
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
    && ln -sf /usr/bin/php8 /usr/bin/php \
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
# For the time being, we run "composer update" as best effort to get php 8.0 working
RUN composer update \
    && composer install --optimize-autoloader --no-dev \
    && mkdir -p storage/logs \
    && chown -R app:app /var/www/html \
    && echo "* * * * * /usr/bin/php /var/www/html/artisan schedule:run" > /etc/crontabs/app \
    && mv docker/supervisor.conf /etc/supervisord.conf \
    && mv docker/nginx.conf /etc/nginx/nginx.conf \
    && mv docker/server.conf /etc/nginx/server.conf \
    && mv docker/php.ini /etc/php8/conf.d/php.ini \
    && sed -i 's/protected \$proxies/protected \$proxies = "*"/g' app/Http/Middleware/TrustProxies.php

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
# This allows us to not include Node within the final container
FROM node:14 as node_modules_go_brrr

RUN mkdir /app

RUN mkdir -p  /app
WORKDIR /app
COPY . .

# Use yarn or npm depending on what type of
# lock file we might find. Defaults to
# NPM if no lock file is found.
RUN if [ -f "yarn.lock" ]; then \
        yarn install --frozen-lockfile; \
        yarn production; \
    elif [ -f "package-lock.json" ]; then \
        npm ci --no-audit; \
        npm run production; \
    else \
        npm install; \
        npm run production; \
    fi

# From our base container created above, we
# create our image, adding in static assets
# generated above
FROM base

# Packages like Laravel Nova may have added assets to the public directory
# or maybe some custom assets were added manually! Either way, we merge
# in the assets we generated above rather than overwrite them
COPY --from=node_modules_go_brrr /app/public /var/www/html/public-npm
RUN rsync -ar /var/www/html/public-npm/ /var/www/html/public/ \
    && rm -rf /var/www/html/public-npm

# The same port nginx.conf is set to listen on and fly.toml references (standard is 8080)
EXPOSE 8080

ENTRYPOINT ["/var/www/html/docker/run.sh"]
