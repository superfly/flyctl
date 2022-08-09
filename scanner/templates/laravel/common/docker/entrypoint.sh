#!/usr/bin/env sh

# Start Octane with the roadrunner binary if we find
# that dependency in the composer.json file
# This is a noop if octane is not used

# TODO: Adjust for our new containers

#if [ -f /var/www/html/composer.json ]; then
#  if grep -Fq "spiral/roadrunner" /var/www/html/composer.json
#  then
#      sed -i 's/;rr command/command/g' /etc/supervisord.conf
#  else
#      sed -i 's/;swoole command/command/g' /etc/supervisord.conf
#  fi
#fi

if [ $# -gt 0 ];then
    # If we passed a command, run it as root
    exec "$@"
else
    # Otherwise start the web server

    ## Do some caching
    /usr/bin/php /var/www/html/artisan config:cache
    /usr/bin/php /var/www/html/artisan route:cache
    /usr/bin/php /var/www/html/artisan view:cache
    chown -R webuser:webgroup /var/www/html

    exec /init
fi
