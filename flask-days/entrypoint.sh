
for f in /app/.profile.d/*.sh; do . $f; done
eval "exec $@"
