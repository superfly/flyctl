package setup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/superfly/flyctl/internal/command/skills"
)

func TestSkillsFailureContinuesOtherAgentsAndSavesManifest(t *testing.T) {
	ctx, home, flags, _, out, _ := harness(t)
	mkdirAgent(t, home, "claude-code", "cursor")
	blocked := filepath.Join(home, ".claude", "skills")
	require.NoError(t, os.WriteFile(blocked, []byte("not a directory"), 0o644))
	require.NoError(t, flags.Set("json", "true"))

	err := runAgent(ctx)
	require.EqualError(t, err, "setup incomplete for Claude Code")
	var rep report
	require.NoError(t, json.Unmarshal(out.Bytes(), &rep))
	require.False(t, rep.OK)
	for _, agent := range rep.Agents {
		if agent.Slug == "claude-code" {
			require.Equal(t, statusFailed, agent.Skills)
			require.Contains(t, agent.SkillsError, blocked)
		} else {
			require.Equal(t, skillsInstalled, agent.Skills)
			require.Empty(t, agent.SkillsError)
			require.FileExists(t, filepath.Join(agent.SkillsDir, "fly", "SKILL.md"))
		}
		if agent.Slug != "universal" {
			require.Equal(t, statusConfigured, agent.MCP)
		}
	}
	var manifest skills.Manifest
	require.NoError(t, json.Unmarshal(readFile(t, filepath.Join(home, ".fly", "skills.json")), &manifest))
	require.Len(t, manifest.Targets, 2)
	for _, parent := range []string{".agents", ".cursor"} {
		key := filepath.ToSlash(filepath.Join(home, parent, "skills"))
		require.NotEmpty(t, manifest.Targets[key]["fly"].Files["SKILL.md"])
	}

	// Repairing the directory lets a normal retry finish and track all three.
	require.NoError(t, os.Remove(blocked))
	out.Reset()
	require.NoError(t, runAgent(ctx))
	require.NoError(t, json.Unmarshal(out.Bytes(), &rep))
	require.True(t, rep.OK)
	require.FileExists(t, filepath.Join(blocked, "fly", "SKILL.md"))
	require.NoError(t, json.Unmarshal(readFile(t, filepath.Join(home, ".fly", "skills.json")), &manifest))
	require.Len(t, manifest.Targets, 3)
}

func TestSetupReportsPreservedChangesPerAgent(t *testing.T) {
	for _, change := range []string{"modified", "deleted locally"} {
		t.Run(change, func(t *testing.T) {
			ctx, home, flags, _, out, _ := harness(t)
			mkdirAgent(t, home, "cursor")
			require.NoError(t, flags.Set("json", "true"))
			require.NoError(t, runAgent(ctx))
			path := filepath.Join(home, ".cursor", "skills", "fly", "SKILL.md")
			if change == "modified" {
				require.NoError(t, os.WriteFile(path, []byte("local edit"), 0o644))
			} else {
				require.NoError(t, os.Remove(path))
			}
			// Setup's force flag must still only affect MCP configuration.
			require.NoError(t, flags.Set("force", "true"))
			out.Reset()
			err := runAgent(ctx)
			if change == "modified" {
				require.NoError(t, err)
				require.Equal(t, "local edit", string(readFile(t, path)))
			} else {
				require.EqualError(t, err, "setup incomplete for Cursor")
				require.NoFileExists(t, path)
			}
			var rep report
			require.NoError(t, json.Unmarshal(out.Bytes(), &rep))
			require.Equal(t, change == "modified", rep.OK)
			require.Equal(t, skillsInstalled, rep.Agents[0].Skills)
			require.Empty(t, rep.Agents[0].SkillsPreserved)
			cursor := rep.Agents[1]
			want := skillsPreserved
			if change == "deleted locally" {
				want = skillsIncomplete
			}
			require.Equal(t, want, cursor.Skills)
			require.Equal(t, []skills.PreservedFile{{Path: "fly/SKILL.md", Reason: change}}, cursor.SkillsPreserved)
			require.Equal(t, statusAlready, cursor.MCP)

			// Embedded output remains one line per agent and includes the cause.
			require.NoError(t, flags.Set("json", "false"))
			t.Setenv(embeddedEnv, "1")
			out.Reset()
			_ = runAgent(ctx)
			require.Len(t, nonEmptyLines(out.String()), 2)
			require.Contains(t, out.String(), "fly/SKILL.md ("+change+")")
			require.NotContains(t, out.String(), "Cursor: skills installed")
		})
	}
}

func TestSetupReportsUntrackedLocalSkill(t *testing.T) {
	ctx, home, flags, _, out, _ := harness(t)
	path := filepath.Join(home, ".cursor", "skills", "fly", "SKILL.md")
	writeExisting(t, path, []byte("user-owned skill"), 0o644)
	require.NoError(t, flags.Set("agent", "cursor"))
	require.NoError(t, flags.Set("skip-mcp", "true"))
	require.NoError(t, flags.Set("json", "true"))
	require.NoError(t, runAgent(ctx))
	var rep report
	require.NoError(t, json.Unmarshal(out.Bytes(), &rep))
	require.True(t, rep.OK)
	require.Equal(t, skillsPreserved, rep.Agents[0].Skills)
	require.Equal(t, []skills.PreservedFile{{Path: "fly/SKILL.md", Reason: "unverified local copy"}}, rep.Agents[0].SkillsPreserved)
	require.Equal(t, "user-owned skill", string(readFile(t, path)))
}
