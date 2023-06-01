#!/usr/bin/env bash
set -eo pipefail
if [ "${FLY_PREFLIGHT_TEST_FLY_ORG}" = "" ] ; then
    echo "error: ensure FLY_PREFLIGHT_TEST_FLY_ORG env var is set"
    exit 1
fi
for app in $(flyctl apps list --json | jq -r '.[] | select(.Organization.Slug == "'${FLY_PREFLIGHT_TEST_FLY_ORG}'") | .Name') ; do
    flyctl apps destroy --yes "${app}"
done
