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
    test_skip_pattern=
    case "$group" in
        apps)
            test_pattern="^TestAppsV2"
            ;;
        deploy)
            test_pattern="^Test(FlyDeploy|Deploy)"
            # Slow fixture and bluegreen tests have independent matrix jobs so
            # they do not consume the shared deploy package timeout.
            test_skip_pattern="^Test(Deploy$|FlyDeploy_BlueGreen)"
            ;;
        deploy-node)
            test_pattern="^TestDeployNodeApp$"
            ;;
        deploy-fixtures)
            test_pattern="^TestDeploy$"
            ;;
        bluegreen)
            test_pattern="^TestFlyDeploy_BlueGreen"
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
            # Flex failover can spend several minutes retrying cluster creation.
            # Run it separately so earlier postgres tests do not exhaust its budget.
            test_skip_pattern="^TestPostgres_FlexFailover$"
            ;;
        postgres-flex-failover)
            test_pattern="^TestPostgres_FlexFailover$"
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
            echo "Available groups: apps, deploy, deploy-node, deploy-fixtures, bluegreen, launch, scale, volume, console, logs, machine, postgres, postgres-flex-failover, tokens, wireguard, misc"
            exit 1
            ;;
    esac

    go_test_args=(-tags=integration -v -timeout=15m)
    if [[ -n "$test_opts" ]]; then
        go_test_args+=("$test_opts")
    fi
    if [[ -n "$test_skip_pattern" ]]; then
        go_test_args+=(-skip "$test_skip_pattern")
    fi
    go_test_args+=(-run "$test_pattern" github.com/superfly/flyctl/test/preflight/...)

    go test "${go_test_args[@]}" | tee "$test_log"
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
