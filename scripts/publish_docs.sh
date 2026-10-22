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
  # Captured before committing, so it does not depend on commit topology.
  CHANGES=$(git diff --cached --name-status -- flyctl/cmd)

  git commit -a -m "[flyctl-bot] Update docs from flyctl"
  git push -f --set-upstream origin HEAD:$BRANCH

  # Name the commands that changed. The title is always the same, so without
  # this a reviewer cannot tell one reworded flag from thirty without reading
  # the diff.
  BODY=$(mktemp)
  {
    echo "Generated from flyctl by \`scripts/publish_docs.sh\`."
    echo
    echo "This branch is rebuilt from main on every release and force pushed, so the"
    echo "diff and the list below always describe the current state rather than"
    echo "accumulating. Do not push fixes onto the branch: the next release discards"
    echo "them. Fix the generator in flyctl, or merge this and let the next release"
    echo "produce a clean one."
    echo
    printf '%s\n' "$CHANGES" | sort -k2 | while read -r status path; do
      [ -n "$path" ] || continue
      cmd=$(basename "$path" .mdx | tr '_' ' ')
      case "$status" in
        A) echo "- added: \`$cmd\`" ;;
        M) echo "- changed: \`$cmd\`" ;;
        D) echo "- removed: \`$cmd\`" ;;
        *) echo "- $status: \`$cmd\`" ;;
      esac
    done
  } > "$BODY"

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
  gh pr create -t "[flybot] Fly CLI docs update" --body-file "$BODY" -B main -H $BRANCH \
    || gh pr edit "$BRANCH" --body-file "$BODY"
fi
