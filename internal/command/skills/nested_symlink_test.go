package skills

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemovalRejectsNestedDirectorySymlinks(t *testing.T) {
	for _, nested := range []string{"references", "references/shared"} {
		for _, operation := range []string{"remove", "update", "force update", "retire"} {
			t.Run(nested+"/"+operation, func(t *testing.T) {
				base := t.TempDir()
				dir := filepath.Join(base, "fly")
				shared := filepath.Join(base, "shared")
				writeFiles(t, shared, map[string]string{"deploy.md": "deploy", "user.md": "keep"})
				linkSkill(t, shared, joinRel(dir, nested))
				files := map[string]string{"SKILL.md": "fly", nested + "/deploy.md": "deploy"}
				// Reproduce adoption of an existing matching file through a directory link.
				rec, res, err := applySkill(dir, skillFiles(files), SkillRecord{}, false, "old", false)
				require.NoError(t, err)
				require.Contains(t, res.Adopted, nested+"/deploy.md")

				switch operation {
				case "remove":
					_, err = removeSkillFiles(dir, rec)
				case "update", "force update":
					_, _, err = applySkill(dir, skillFiles(map[string]string{"SKILL.md": "fly"}), rec, true, "new", operation == "force update")
				case "retire":
					_, _, err = applySkill(dir, nil, rec, true, "new", false)
				}
				require.Error(t, err, "refuse to remove a recorded file through a link outside the skill directory")
				body, err := os.ReadFile(joinRel(shared, "deploy.md"))
				require.NoError(t, err)
				assert.Equal(t, "deploy", string(body))
				assert.FileExists(t, joinRel(shared, "user.md"))
				info, err := os.Lstat(joinRel(dir, nested))
				require.NoError(t, err)
				assert.NotZero(t, info.Mode()&os.ModeSymlink)
			})
		}
	}
}

func TestRemoveFileSymlinkPreservesDestination(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "fly")
	shared := filepath.Join(base, "shared")
	writeFiles(t, shared, map[string]string{"deploy.md": "deploy"})
	linkSkill(t, joinRel(shared, "deploy.md"), joinRel(dir, "references/deploy.md"))
	rec := recordFor(map[string]string{"references/deploy.md": "deploy"}, "old")
	_, err := removeSkillFiles(dir, rec)
	require.NoError(t, err)
	assert.FileExists(t, joinRel(shared, "deploy.md"))
	assert.NoDirExists(t, dir)
}
