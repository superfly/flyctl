#! /bin/bash
set -euo pipefail

ref=
total=
index=

while getopts r:t:i: name
do
    case "$name" in
        r)
	    ref="$OPTARG"
	    ;;
        t)
	    total="$OPTARG"
	    ;;
        i)
	    index="$OPTARG"
	    ;;
        ?)
	    printf "Usage: %s: [-r REF] [-t TOTAL] [-i INDEX]\n" $0
            exit 2
	    ;;
    esac
done

shift $(($OPTIND - 1))


if [[ "$ref" != "refs/heads/master" ]]; then
    test_opts=-short
fi

gotesplit \
    -total "$total" \
    -index "$index" \
    github.com/superfly/flyctl/test/preflight/... \
    -- --tags=integration -v -timeout=60m $test_opts
