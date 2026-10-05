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

  # The PR is left for a person to merge. It used to merge itself, which kept
  # the reference current but meant reworded flag descriptions published with
  # nobody having read them.
  #
  # $BRANCH is derived from the ref, so successive releases reuse one branch.
  # The force push above updates it, and any open PR picks the new commits up,
  # so gh pr create failing because one already exists is the expected path
  # rather than an error.
  gh pr create -t "[flybot] Fly CLI docs update" -b "Fly CLI docs update" -B main -H $BRANCH \
    || echo "a pull request for $BRANCH is already open; it now has the latest docs"
fi
