#! /bin/bash
set -euo pipefail

ref=
total=
index=
out=

while getopts r:t:i:o: name
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
        o)
	    out="$OPTARG"
	    ;;
        ?)
	    printf "Usage: %s: [-r REF] [-t TOTAL] [-i INDEX] [-o FILE]\n" $0
            exit 2
	    ;;
    esac
done

shift $(($OPTIND - 1))

test_opts=
if [[ "$ref" != "refs/heads/master" ]]; then
    test_opts=-short
fi

test_log="$(mktemp)"
function finish {
  rm "$test_log"
}
trap finish EXIT

set +e

gotesplit \
    -total "$total" \
    -index "$index" \
    github.com/superfly/flyctl/test/preflight/... \
    -- --tags=integration -v -timeout=10m $test_opts | tee "$test_log"
test_status=$?

set -e

if [[ -n "$out" ]]; then
    awk '/^--- FAIL:/{ printf("%s ", $3) }' "$test_log" >> "$out"
    echo >> "$out"
fi

exit $test_status
