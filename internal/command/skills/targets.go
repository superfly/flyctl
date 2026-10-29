package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// universalSlug is the catch-all target every install writes to, regardless
// of which coding tools are detected.
const universalSlug = "universal"

// CodingTool is a coding agent that reads skills from a global directory.
type CodingTool struct {
	Slug   string
	Name   string
	Parent string // directory whose presence means the tool is installed
}

// SkillsDir is the directory skills are written into for this tool.
func (t CodingTool) SkillsDir() string {
	return filepath.Join(t.Parent, "skills")
}

// Target is one skills directory an install writes into.
type Target struct {
	Slug      string
	ToolName  string
	SkillsDir string
}

// Key is the manifest key for this target: its skills directory with forward
// slashes, so the manifest is stable across platforms.
func (t Target) Key() string {
	return filepath.ToSlash(t.SkillsDir)
}

// codingTools lists every tool we know how to install into, in display order.
// The universal target comes first and is always selected.
func codingTools(home string) []CodingTool {
	return []CodingTool{
		{Slug: universalSlug, Name: "Universal (.agents)", Parent: filepath.Join(home, ".agents")},
		{Slug: "claude-code", Name: "Claude Code", Parent: filepath.Join(home, ".claude")},
		{Slug: "codex", Name: "OpenAI Codex", Parent: filepath.Join(home, ".codex")},
		{Slug: "cursor", Name: "Cursor", Parent: filepath.Join(home, ".cursor")},
		{Slug: "opencode", Name: "OpenCode", Parent: filepath.Join(home, ".config", "opencode")},
		{Slug: "copilot", Name: "GitHub Copilot", Parent: filepath.Join(home, ".copilot")},
		{Slug: "factory-droid", Name: "Factory Droid", Parent: filepath.Join(home, ".factory")},
		{Slug: "pi", Name: "Pi", Parent: filepath.Join(home, ".pi", "agent")},
		{Slug: "antigravity", Name: "Antigravity", Parent: filepath.Join(home, ".gemini", "config")},
	}
}

// resolveTools picks the tools to act on. With no filter, the universal target
// plus every tool whose parent directory exists. With a filter, exactly the
// named slugs, in the order given.
func resolveTools(home string, slugs []string) ([]CodingTool, error) {
	all := codingTools(home)

	if len(slugs) == 0 {
		var detected []CodingTool
		for _, tool := range all {
			if tool.Slug == universalSlug || isDir(tool.Parent) {
				detected = append(detected, tool)
			}
		}

		return detected, nil
	}

	var selected []CodingTool
	seen := map[string]bool{}
	for _, slug := range slugs {
		slug = strings.TrimSpace(slug)
		if seen[slug] {
			continue
		}
		var found *CodingTool
		for i := range all {
			if all[i].Slug == slug {
				found = &all[i]
				break
			}
		}
		if found == nil {
			return nil, fmt.Errorf("unknown agent %q\n\nValid agents: %s", slug, strings.Join(toolSlugs(all), ", "))
		}
		seen[slug] = true
		selected = append(selected, *found)
	}

	return selected, nil
}

func toolSlugs(tools []CodingTool) []string {
	slugs := make([]string, 0, len(tools))
	for _, tool := range tools {
		slugs = append(slugs, tool.Slug)
	}
	return slugs
}

func buildTargets(tools []CodingTool) []Target {
	targets := make([]Target, 0, len(tools))
	for _, tool := range tools {
		targets = append(targets, Target{
			Slug:      tool.Slug,
			ToolName:  tool.Name,
			SkillsDir: tool.SkillsDir(),
		})
	}

	return targets
}

// toolNameForKey maps a manifest target key back to a tool name, falling back
// to the path itself for targets we no longer recognise.
func toolNameForKey(home, key string) string {
	for _, tool := range codingTools(home) {
		if filepath.ToSlash(tool.SkillsDir()) == key {
			return tool.Name
		}
	}

	return key
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	return keys
}
