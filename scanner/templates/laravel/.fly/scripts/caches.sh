#!/usr/bin/env bash

/usr/bin/php /var/www/html/artisan config:cache --no-ansi -q
/usr/bin/php /var/www/html/artisan route:cache --no-ansi -q
/usr/bin/php /var/www/html/artisan view:cache --no-ansi -q

# Filament v3.2 Performance Improvement with commands 
if grep "filament/filament.*:.*3\.2" "/var/www/html/composer.json"; then 
    /usr/bin/php /var/www/html/artisan icons:cache --no-ansi -q
    /usr/bin/php /var/www/html/artisan filament:cache-components --no-ansi -q
fi