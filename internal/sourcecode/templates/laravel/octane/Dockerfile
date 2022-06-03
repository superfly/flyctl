FROM alpine:edge

LABEL fly_launch_runtime="laravel"

RUN apk update

# add useful utilities
RUN apk add curl \
    zip \
    unzip \
    ssmtp \
    tzdata

# php, with assorted extensions we likely need
RUN apk add php8 \
    php8-cli \
    php8-pecl-mcrypt \
    php8-soap \
    php8-openssl \
    php8-gmp \
    php8-pdo_odbc \
    php8-json \
    php8-dom \
    php8-pdo \
    php8-zip \
    php8-pdo_mysql \
    php8-sqlite3 \
    php8-pdo_pgsql \
    php8-bcmath \
    php8-gd \
    php8-odbc \
    php8-pdo_sqlite \
    php8-gettext \
    php8-xmlreader \
    php8-bz2 \
    php8-iconv \
    php8-pdo_dblib \
    php8-curl \
    php8-ctype \
    php8-phar \
    php8-xml \
    php8-common \
    php8-mbstring \
    php8-tokenizer \
    php8-xmlwriter \
    php8-fileinfo \
    php8-opcache \
    php8-simplexml \
    php8-pecl-redis\
    php8-sockets \
    php8-pcntl \
    php8-posix

# node, for Laravel mix
RUN apk add nodejs npm

# supervisor, to support running multiple processes in a single app
RUN apk add supervisor

# nginx (https://wiki.alpinelinux.org/wiki/Nginx)
RUN apk add nginx
# ... with custom conf
RUN cp /etc/nginx/nginx.conf /etc/nginx/nginx.old.conf && rm -rf /etc/nginx/http.d/default.conf

# htop, which is useful if need to SSH in to the vm
RUN apk add htop

# composer, to install Laravel's dependencies
RUN curl -sS https://getcomposer.org/installer | php -- --install-dir=/usr/local/bin --filename=composer

# add users (see https://www.getpagespeed.com/server-setup/nginx-and-php-fpm-what-my-permissions-should-be)
# 1. a user for the app and octane to interact with it (execute)
RUN adduser -D -u 1000 -g 'app' app
# 2. a user for nginx is not needed because already have one
# ... and add the nginx user TO the app group else it won't have permission to access web files (as can see in /var/log/nginx/error.log)
RUN addgroup nginx app

# working directory
RUN mkdir /var/www/html
WORKDIR /var/www/html

# copy app code across, skipping files based on .dockerignore
COPY . /var/www/html
# ... install Laravel dependencies
RUN composer update && composer install --optimize-autoloader --no-dev

# Download roadrunner binary, prepare needed directory
RUN rm -f rr \
    && ./vendor/bin/rr get-binary \
    && mkdir -p storage/logs \
    && rm -f .rr.yaml


# ... and make all files owned by app, including the just added /vendor
RUN chown -R app:app /var/www/html

# Configure crontab for Supervisord to use
RUN /usr/bin/crontab docker/crontab

# move the docker-related conf files out of the app folder to where on the vm they need to be
RUN mv docker/supervisor.conf /etc/supervisord.conf
RUN mv docker/nginx.conf /etc/nginx/nginx.conf
RUN mv docker/php.ini /etc/php8/conf.d/php.ini

# Laravel's demo app does not need mix however real apps likely will (optimize js/css) so install and then run that:
RUN npm install
RUN npx mix --production
# ... now don't need /node_modules so might as well delete that
RUN rm -rf node_modules
# ... and don't need node and npm so might as well delete them
RUN apk del nodejs npm

# Octane needs to write to the storage directory
RUN chmod -R ug+w /var/www/html/storage

# clear Laravel cache that may be left over
RUN composer dump-autoload
RUN php artisan optimize:clear

# make sure can execute php files (since octane runs as app, it needs permission e.g for /storage/framework/views for caching views)
RUN chmod -R 755 /var/www/html

# the same port nginx.conf is set to listen on and fly.toml references (standard is 8080)
EXPOSE 8080

# off we go (since no docker-compose, keep both nginx and octane running in the same container by using supervisor) ...
ENTRYPOINT ["supervisord", "-c", "/etc/supervisord.conf"]
