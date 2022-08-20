#!/usr/bin/env bash

# Clear out old docs
rm out/*.md

echo "Running doc/main.go"
go run doc/main.go

echo "Cleaning up output"

# test for GNU sed.  Hat tip: https://stackoverflow.com/a/65497543
if sed --version >/dev/null 2>&1; then
  sed -i 's/```/~~~/g' out/*.md
else
  sed -i "" -e 's/```/~~~/g' out/*.md
fi

if [ "$1" ]
    then
        echo "rsync to $1"
        rsync out/ $1 --delete -r -v
fi
