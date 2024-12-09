#!/bin/bash

if [ ! -f /data/prod.db ]; then
    echo "Creating database file"
    sqlite3 /data/prod.db
fi

/app/entry eval HelloElixir.Release.migrate && \
    /app/entry start