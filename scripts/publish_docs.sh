#!/usr/bin/env bash

BRANCH=flyctl-docs_$1
scripts/generate_docs.sh docs/flyctl/cmd

cd docs
git config --global user.email "joshua@fly.io"
git config --global user.name "Fly.io CI"
git checkout -b $BRANCH
git add flyctl/cmd
git diff --cached --quiet

if [ $? -gt 0 ]; then
  git commit -a -m "[flyctl-bot] Update docs from flyctl"
  git push -f --set-upstream origin HEAD:$BRANCH
  gh pr create -t "[flybot] Fly CLI docs update" -b "Fly CLI docs update" -B main -H $BRANCH -r jsierles
  gh pr merge --delete-branch
fi
