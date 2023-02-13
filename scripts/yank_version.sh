#!/usr/bin/env bash

set -euo pipefail

ORIGIN=${ORIGIN:-origin}

TAG="v$1"

echo "Yanking version $TAG"

localtag () {
  if [[ $(git tag -l $TAG) ]]; then
    echo 1
  else
    echo 0
  fi
}

remotetag () {
  if [[ $(git ls-remote $ORIGIN --refs refs/tags/$TAG) ]]; then
    echo 1
  else
    echo 0
  fi
}

prompt () {
  read -p "Are you sure? " -n 1 -r
  if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo 1
  else
    echo 0
  fi
}

LOCAL_EXISTS=$(localtag)
REMOTE_EXISTS=$(remotetag)

if [[ $LOCAL_EXISTS != 1 && $REMOTE_EXISTS != 1 ]]; then
  echo "no tag found"
  exit 1
fi

if [[ $(prompt) != 1 ]]; then
  exit 1
fi

if [[ $LOCAL_EXISTS == 1 ]]; then
  echo "deleting local tag"
  git tag -d "$TAG"
  echo "done"
fi

if [[ $REMOTE_EXISTS == 1 ]]; then
  echo "deleting remote tag"
  git push $ORIGIN :refs/tags/$TAG
  echo "done"
fi
