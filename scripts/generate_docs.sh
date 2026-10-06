#!/usr/bin/env bash

# Clear out old docs
rm -f out/*.mdx

echo "Running doc/main.go"
go run doc/main.go

if [ "$1" ]
    then
        # Carry sidebarTitle across. On about three dozen pages it is a
        # hand-written nav label that cobra's Short does not match, so the
        # generator cannot produce it and must not clobber it. Anything else in
        # the frontmatter is generated and safe to overwrite.
        echo "Preserving sidebarTitle from $1"
        for f in out/*.mdx; do
          target="$1/$(basename "$f")"
          [ -f "$target" ] || continue
          label=$(sed -n '2,/^---$/{/^sidebarTitle:/p;}' "$target" | head -1)
          [ -n "$label" ] || continue
          awk -v lbl="$label" 'NR==2 && /^title:/ {print; print lbl; next} {print}' "$f" > "$f.tmp" && mv "$f.tmp" "$f"
        done

        # --delete so a command removed from flyctl stops being documented.
        # Without it the pages linger: the litefs-cloud commands went in
        # #5187 and their eleven pages stayed live for four weeks, telling
        # readers a retired CLI worked.
        #
        # This deletes anything in the destination the generator did not
        # produce, so nothing hand-written belongs in flyctl/cmd.
        echo "rsync to $1"
        rsync out/ $1 --delete -r -v
fi
