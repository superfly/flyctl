FROM laravelsail/php81-composer:latest

ADD flyctl /usr/local/bin

WORKDIR /app

RUN composer create-project laravel/laravel /app/testflight

WORKDIR /app/testflight

CMD ["flyctl", "launch", "--build-only"]
# CMD ["php", "artisan", "serve", "--host", "0.0.0.0", "--port", "80"]
