#!/usr/bin/env bash
set -eo pipefail

prefix="${1:-}"

if [ "${FLY_PREFLIGHT_TEST_FLY_ORG}" = "" ] ; then
    echo "error: ensure FLY_PREFLIGHT_TEST_FLY_ORG env var is set"
    exit 1
fi
for app in $(flyctl apps list --json | jq -r '.[] | select(.Organization.Slug == "'${FLY_PREFLIGHT_TEST_FLY_ORG}'") | .Name')
do
    if [[ -n "$prefix" && ! "$app" =~ ^$prefix ]]; then
	continue
    fi
    echo "Destroy $app"
    flyctl apps destroy --yes "${app}"
done
