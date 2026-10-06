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

        # --delete is deliberately absent. With it, the first sync after the
        # MDX switch would also remove the 12 pages the generator no longer
        # produces: the retired litefs-cloud commands and the duplicate root
        # page. 12 docs.json nav entries and 15 redirects still point at those,
        # so they are a reviewable docs change rather than something this bot
        # should do unannounced. Restore --delete once that has landed, or
        # removed commands will linger here forever.
        echo "rsync to $1"
        rsync out/ $1 -r -v
fi
