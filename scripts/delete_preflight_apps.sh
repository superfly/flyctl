#!/usr/bin/env bash
set -eo pipefail
if [ "${FLY_PREFLIGHT_TEST_FLY_ORG}x" = "x" ] ; then
    echo "error: ensure FLY_PREFLIGHT_TEST_FLY_ORG env var is set"
    exit 1
fi
for app in $(fly apps list --json | jq -r '.[] | select(.Organization.Slug == "'${FLY_PREFLIGHT_TEST_FLY_ORG}'") | .Name') ; do
    fly apps destroy --yes "${app}"
done
