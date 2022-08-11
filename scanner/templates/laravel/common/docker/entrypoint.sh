#!/usr/bin/env sh

if [ $# -gt 0 ];then
    # If we passed a command, run it as root
    exec "$@"
else
    # Otherwise start the web server

    ## Prepare Laravel caches
    /usr/bin/php /var/www/html/artisan config:cache --no-ansi -q
    /usr/bin/php /var/www/html/artisan route:cache --no-ansi -q
    /usr/bin/php /var/www/html/artisan view:cache --no-ansi -q
    chown -R webuser:webgroup /var/www/html

    exec /init
fi
