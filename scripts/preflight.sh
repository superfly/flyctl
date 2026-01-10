#!/bin/bash
set -euo pipefail

ref=
group=
# Legacy support for numeric sharding (deprecated)
total=
index=
out=

while getopts r:g:t:i:o: name
do
    case "$name" in
        r)
	    ref="$OPTARG"
	    ;;
        g)
	    group="$OPTARG"
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
	    printf "Usage: %s: [-r REF] [-g GROUP] [-t TOTAL] [-i INDEX] [-o FILE]\n" $0
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

# Define test groups based on logical groupings
if [[ -n "$group" ]]; then
    case "$group" in
        apps)
            test_pattern="^TestAppsV2"
            ;;
        deploy)
            test_pattern="^Test(FlyDeploy|Deploy)"
            ;;
        launch)
            test_pattern="^Test(FlyLaunch|Launch)"
            ;;
        scale)
            test_pattern="^TestFlyScale"
            ;;
        volume)
            test_pattern="^TestVolume"
            ;;
        console)
            test_pattern="^TestFlyConsole"
            ;;
        logs)
            test_pattern="^TestFlyLogs"
            ;;
        machine)
            test_pattern="^TestFlyMachine"
            ;;
        postgres)
            test_pattern="^TestPostgres"
            ;;
        tokens)
            test_pattern="^TestTokens"
            ;;
        wireguard)
            test_pattern="^TestFlyWireguard"
            ;;
        misc)
            test_pattern="^Test(ErrOutput|ImageLabel|NoPublicIP)"
            ;;
        *)
            echo "Unknown test group: $group"
            echo "Available groups: apps, deploy, launch, scale, volume, console, logs, machine, postgres, tokens, wireguard, misc"
            exit 1
            ;;
    esac

    go test -tags=integration -v -timeout=15m $test_opts -run "$test_pattern" github.com/superfly/flyctl/test/preflight/... | tee "$test_log"
    test_status=$?
# Legacy numeric sharding using gotesplit (deprecated)
elif [[ -n "$total" && -n "$index" ]]; then
    gotesplit \
        -total "$total" \
        -index "$index" \
        github.com/superfly/flyctl/test/preflight/... \
        -- --tags=integration -v -timeout=15m $test_opts | tee "$test_log"
    test_status=$?
else
    echo "Error: Must specify either -g GROUP or both -t TOTAL and -i INDEX"
    exit 1
fi

set -e

if [[ -n "$out" ]]; then
    awk '/^--- FAIL:/{ printf("%s ", $3) }' "$test_log" >> "$out"
    echo >> "$out"
fi

exit $test_status
