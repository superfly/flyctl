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
  # $BRANCH comes from the ref, so every release reuses one branch, and the
  # checkout above is a fresh clone of the docs default branch. So this branch
  # is rebuilt from main each run and force pushed, replacing whatever was
  # there with a single commit holding all current drift. An open PR always
  # shows the whole picture rather than a stack, and never needs rebasing when
  # other docs changes land.
  #
  # The consequence worth knowing: do not push fixes onto this branch, because
  # the next release discards them. Fix the generator in flyctl, or merge the
  # PR first and let the following release produce a clean one.
  #
  # gh pr create failing because a PR is already open is the expected path.
  gh pr create -t "[flybot] Fly CLI docs update" -b "Fly CLI docs update" -B main -H $BRANCH \
    || echo "a pull request for $BRANCH is already open; it now has the latest docs"
fi
