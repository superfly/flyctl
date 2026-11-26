package sprites

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	sprites "github.com/superfly/sprites-go"
)

// commonTerms are terminal types every sprite image already describes, so
// there is nothing to send for them.
var commonTerms = map[string]bool{
	"xterm": true, "xterm-256color": true, "xterm-color": true,
	"screen": true, "screen-256color": true,
	"tmux": true, "tmux-256color": true,
	"linux": true, "vt100": true, "vt220": true, "ansi": true, "dumb": true,
}

// terminfoEnv carries the terminal description to the console wrapper when it
// could not be written to the sprite's filesystem.
const terminfoEnv = "SPRITE_TERMINFO_DATA"

// shareTerminfo gives the sprite the local terminal's terminfo entry, so that
// terminals the image does not know, such as Ghostty or kitty, work in a
// console. It returns environment to pass to the console, which is only
// needed when the entry could not be installed. Any failure just means the
// console starts without the entry.
func shareTerminfo(ctx context.Context, sprite *sprites.Sprite) []string {
	term := os.Getenv("TERM")
	if term == "" || commonTerms[term] || strings.ContainsAny(term, `/\`) {
		return nil
	}

	data := localTerminfo(ctx, term)
	if data == nil {
		return nil
	}

	remote := fmt.Sprintf("/usr/share/terminfo/%c/%s", term[0], term)

	if err := sprite.FilesystemAt("/").WriteFileContext(ctx, remote, data, 0o644); err != nil {
		return []string{terminfoEnv + "=" + base64.StdEncoding.EncodeToString(data)}
	}

	return nil
}

// localTerminfo returns the compiled terminfo entry for term, from the places
// ncurses looks, or compiled from infocmp's output when no file is found.
func localTerminfo(ctx context.Context, term string) []byte {
	var dirs []string

	if dir := os.Getenv("TERMINFO"); dir != "" {
		dirs = append(dirs, dir)
	}

	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".terminfo"))
	}

	for _, dir := range filepath.SplitList(os.Getenv("TERMINFO_DIRS")) {
		if dir != "" {
			dirs = append(dirs, dir)
		}
	}

	dirs = append(dirs, "/usr/share/terminfo", "/lib/terminfo", "/etc/terminfo")

	// Entries are filed under the first letter, or its hex code on macOS.
	subdirs := []string{term[:1], fmt.Sprintf("%02x", term[0])}

	for _, dir := range dirs {
		for _, subdir := range subdirs {
			if data, err := os.ReadFile(filepath.Join(dir, subdir, term)); err == nil && len(data) > 0 {
				return data
			}
		}
	}

	return compileTerminfo(ctx, term, subdirs)
}

// compileTerminfo asks infocmp for the entry's source and compiles it with
// tic, for terminals that describe themselves some other way.
func compileTerminfo(ctx context.Context, term string, subdirs []string) []byte {
	source, err := exec.CommandContext(ctx, "infocmp", "-x", term).Output()
	if err != nil || len(source) == 0 {
		return nil
	}

	tmp, err := os.MkdirTemp("", "fly-terminfo-")
	if err != nil {
		return nil
	}
	defer os.RemoveAll(tmp) //nolint:errcheck

	tic := exec.CommandContext(ctx, "tic", "-x", "-o", tmp, "-")
	tic.Stdin = strings.NewReader(string(source))

	if err := tic.Run(); err != nil {
		return nil
	}

	for _, subdir := range subdirs {
		if data, err := os.ReadFile(filepath.Join(tmp, subdir, term)); err == nil && len(data) > 0 {
			return data
		}
	}

	return nil
}
