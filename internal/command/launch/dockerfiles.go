package launch

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/filemu"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/scanner"
	"github.com/superfly/flyctl/terminal"
)

func appendDockerfileAppendix(appendix []string) (err error) {
	const dockerfilePath = "Dockerfile"

	var b bytes.Buffer
	b.WriteString("\n# Appended by flyctl\n")

	for _, value := range appendix {
		_, _ = b.WriteString(value)
		_ = b.WriteByte('\n')
	}

	var unlock filemu.UnlockFunc

	if unlock, err = filemu.Lock(context.Background(), dockerfilePath); err != nil {
		return
	}
	defer func() {
		if e := unlock(); err == nil {
			err = e
		}
	}()

	var f *os.File
	// TODO: we don't flush
	if f, err = os.OpenFile(dockerfilePath, os.O_APPEND|os.O_WRONLY, 0o600); err != nil {
		return
	}
	defer func() {
		if e := f.Close(); err == nil {
			err = e
		}
	}()

	_, err = b.WriteTo(f)

	return
}

func createDockerignoreFromGitignores(root string, gitIgnores []string) (string, error) {
	dockerIgnore := filepath.Join(root, ".dockerignore")
	f, err := os.Create(dockerIgnore)
	if err != nil {
		return "", err
	}
	defer func() {
		if err := f.Close(); err != nil {
			terminal.Debugf("error closing %s file after writing: %v\n", dockerIgnore, err)
		}
	}()

	firstHeaderWritten := false
	foundFlyDotToml := false
	linebreak := []byte("\n")
	for _, gitIgnore := range gitIgnores {
		gitF, err := os.Open(gitIgnore)
		defer func() {
			if err := gitF.Close(); err != nil {
				terminal.Debugf("error closing %s file after reading: %v\n", gitIgnore, err)
			}
		}()
		if err != nil {
			terminal.Debugf("error opening %s file: %v\n", gitIgnore, err)
			continue
		}
		relDir, err := filepath.Rel(root, filepath.Dir(gitIgnore))
		if err != nil {
			terminal.Debugf("error finding relative directory of %s relative to root %s: %v\n", gitIgnore, root, err)
			continue
		}
		relFile, err := filepath.Rel(root, gitIgnore)
		if err != nil {
			terminal.Debugf("error finding relative file of %s relative to root %s: %v\n", gitIgnore, root, err)
			continue
		}

		headerWritten := false
		scanner := bufio.NewScanner(gitF)
		for scanner.Scan() {
			line := scanner.Text()
			if !headerWritten {
				if !firstHeaderWritten {
					firstHeaderWritten = true
				} else {
					f.Write(linebreak)
				}
				_, err := f.WriteString(fmt.Sprintf("# flyctl launch added from %s\n", relFile))
				if err != nil {
					return "", err
				}
				headerWritten = true
			}
			var dockerIgnoreLine string
			if strings.TrimSpace(line) == "" {
				dockerIgnoreLine = ""
			} else if strings.HasPrefix(line, "#") {
				dockerIgnoreLine = line
			} else if strings.HasPrefix(line, "!/") {
				dockerIgnoreLine = fmt.Sprintf("!%s", filepath.Join(relDir, line[2:]))
			} else if strings.HasPrefix(line, "!") {
				dockerIgnoreLine = fmt.Sprintf("!%s", filepath.Join(relDir, "**", line[1:]))
			} else if strings.HasPrefix(line, "/") {
				dockerIgnoreLine = filepath.Join(relDir, line[1:])
			} else {
				dockerIgnoreLine = filepath.Join(relDir, "**", line)
			}
			if strings.Contains(dockerIgnoreLine, "fly.toml") {
				foundFlyDotToml = true
			}
			if _, err := f.WriteString(dockerIgnoreLine); err != nil {
				return "", err
			}
			if _, err := f.Write(linebreak); err != nil {
				return "", err
			}
		}
	}

	if !foundFlyDotToml {
		if _, err := f.WriteString("fly.toml"); err != nil {
			return "", err
		}
		if _, err := f.Write(linebreak); err != nil {
			return "", err
		}
	}

	return dockerIgnore, nil
}

// determineDockerIgnore attempts to create a .dockerignore from .gitignore
func (state *launchState) createDockerIgnore(ctx context.Context) (err error) {
	io := iostreams.FromContext(ctx)
	dockerIgnore := ".dockerignore"
	gitIgnore := ".gitignore"
	allGitIgnores := scanner.FindGitignores(state.workingDir)
	createDockerignoreFromGitignore := false

	// An existing .dockerignore should always be used instead of .gitignore
	if helpers.FileExists(dockerIgnore) {
		terminal.Debugf("Found %s file. Will use when deploying to Fly.\n", dockerIgnore)
		return
	}

	// If we find .gitignore files, determine whether they should be converted to .dockerignore
	if len(allGitIgnores) > 0 {

		if flag.GetBool(ctx, "dockerignore-from-gitignore") {
			createDockerignoreFromGitignore = true
		} else {
			confirm, err := prompt.Confirm(ctx, fmt.Sprintf("Create %s from %d %s files?", dockerIgnore, len(allGitIgnores), gitIgnore))
			if confirm && err == nil {
				createDockerignoreFromGitignore = true
			}
		}

		if createDockerignoreFromGitignore {
			createdDockerIgnore, err := createDockerignoreFromGitignores(state.workingDir, allGitIgnores)
			if err != nil {
				terminal.Warnf("Error creating %s from %d %s files: %v\n", dockerIgnore, len(allGitIgnores), gitIgnore, err)
			} else {
				fmt.Fprintf(io.Out, "Created %s from %d %s files.\n", createdDockerIgnore, len(allGitIgnores), gitIgnore)
			}
			return nil
		}
	}
	return
}
