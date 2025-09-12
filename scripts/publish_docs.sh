#!/usr/bin/env bash

BRANCH=flyctl-docs_$1
scripts/generate_docs.sh docs/flyctl/cmd

cd docs
git config --global user.name 'docs-syncer[bot]'
git config --global user.email '134718678+docs-syncer[bot]@users.noreply.github.com'
git checkout -b $BRANCH
git add flyctl/cmd
git diff --cached --quiet

if [ $? -gt 0 ]; then
  git commit -a -m "[flyctl-bot] Update docs from flyctl"
  git push -f --set-upstream origin HEAD:$BRANCH
  gh pr create -t "[flybot] Fly CLI docs update" -b "Fly CLI docs update" -B main -H $BRANCH
  gh pr merge --delete-branch --squash
fi
