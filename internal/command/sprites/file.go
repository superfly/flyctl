package sprites

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/google/shlex"
	"github.com/spf13/cobra"
	sprites "github.com/superfly/sprites-go"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyerr"
	"github.com/superfly/flyctl/iostreams"
)

// remoteHome is the directory relative remote paths are resolved against.
const remoteHome = "/home/sprite"

func newFile() *cobra.Command {
	const (
		short = "Copy files to and from a sprite, or edit one in place."

		long = `Manage files in a sprite.

Remote paths may be written as <sprite>:<path> to name the sprite inline.
Relative remote paths are resolved against ` + remoteHome + `.
`
	)

	cmd := command.New("file", short, long, nil)
	cmd.Aliases = []string{"files"}

	cmd.AddCommand(
		newFileEdit(),
		newFilePush(),
		newFilePull(),
	)

	return cmd
}

// remotePath is a path in a particular sprite.
type remotePath struct {
	sprite *sprites.Sprite
	path   string
}

func (r remotePath) fs() sprites.FS {
	return r.sprite.FilesystemAt(remoteHome)
}

// parseRemotePath splits an optional "<sprite>:" prefix off a remote path. A
// path without one uses the command's target sprite, which is resolved once
// and only when needed.
func parseRemotePaths(ctx context.Context, s *session, args []string) ([]remotePath, error) {
	paths := make([]remotePath, len(args))

	var fallback *sprites.Sprite

	for i, arg := range args {
		if name, p, ok := strings.Cut(arg, ":"); ok && name != "" && !strings.Contains(name, "/") {
			paths[i] = remotePath{sprite: s.Client.Sprite(name), path: p}
			continue
		}

		if fallback == nil {
			sprite, err := s.sprite(ctx, "")
			if err != nil {
				return nil, err
			}

			fallback = sprite
		}

		paths[i] = remotePath{sprite: fallback, path: arg}
	}

	return paths, nil
}

func newFileEdit() *cobra.Command {
	const (
		short = "Edit a file in a sprite with your local editor."

		long = `Download a file from a sprite, open it in $VISUAL or $EDITOR, and upload it
if it changed.

When stdin is not a terminal, the new contents are read from it instead. Empty
input is refused rather than emptying the file.
`
	)

	cmd := newCommand("edit [flags] [<sprite>:]<path>", short, long, runFileEdit)
	cmd.Args = cobra.ExactArgs(1)

	return cmd
}

func runFileEdit(ctx context.Context, s *session) error {
	ios := iostreams.FromContext(ctx)

	targets, err := parseRemotePaths(ctx, s, flag.Args(ctx))
	if err != nil {
		return err
	}

	target := targets[0]
	fsys := target.fs()

	info, err := fsys.Stat(target.path)
	if err != nil {
		return fmt.Errorf("%s: %w", target.path, err)
	}

	if info.IsDir() {
		return fmt.Errorf("%s is a directory", target.path)
	}

	data, err := fsys.ReadFile(target.path)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", target.path, err)
	}

	// Open an editor whenever there is a terminal to type into. Only take the
	// new contents from stdin when something is really piped in, and never
	// let an empty pipe, such as </dev/null, blank the file.
	var edited []byte
	if ios.IsStdinTTY() {
		edited, err = editLocally(ctx, data, path.Ext(target.path))
	} else {
		edited, err = io.ReadAll(ios.In)

		if err == nil && len(edited) == 0 {
			return flyerr.WithSuggestion(
				errors.New("no new contents on stdin, so the file was left unchanged"),
				"Pipe the new contents in, or run this in a terminal to use an editor. "+
					"To empty a file, use 'fly sprites exec -- truncate -s 0 <path>'.",
			)
		}
	}

	if err != nil {
		return err
	}

	if bytes.Equal(edited, data) {
		fmt.Fprintln(ios.ErrOut, "No changes made.")
		return nil
	}

	if err := fsys.WriteFileContext(ctx, target.path, edited, info.Mode()); err != nil {
		return fmt.Errorf("failed to upload %s: %w", target.path, err)
	}

	fmt.Fprintf(ios.ErrOut, "Updated %s\n", target.path)

	return nil
}

// editLocally opens data in the user's editor and returns the saved contents.
func editLocally(ctx context.Context, data []byte, ext string) ([]byte, error) {
	editor, err := findEditor()
	if err != nil {
		return nil, err
	}

	tmp, err := os.CreateTemp("", "sprite-edit-*"+ext)
	if err != nil {
		return nil, fmt.Errorf("failed to create a temporary file: %w", err)
	}
	defer os.Remove(tmp.Name()) //nolint:errcheck

	if _, err := tmp.Write(data); err != nil {
		tmp.Close() //nolint:errcheck
		return nil, fmt.Errorf("failed to write the temporary file: %w", err)
	}

	if err := tmp.Close(); err != nil {
		return nil, fmt.Errorf("failed to write the temporary file: %w", err)
	}

	cmd := exec.CommandContext(ctx, editor[0], append(editor[1:], tmp.Name())...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("the editor exited with an error, so nothing was uploaded: %w", err)
	}

	return os.ReadFile(tmp.Name())
}

// findEditor returns the editor command line from $VISUAL or $EDITOR, falling
// back to vi or nano.
func findEditor() ([]string, error) {
	for _, key := range []string{"VISUAL", "EDITOR"} {
		if value := os.Getenv(key); value != "" {
			fields, err := shlex.Split(value)
			if err != nil || len(fields) == 0 {
				return nil, fmt.Errorf("could not parse $%s (%q)", key, value)
			}

			return fields, nil
		}
	}

	for _, fallback := range []string{"vi", "nano"} {
		if _, err := exec.LookPath(fallback); err == nil {
			return []string{fallback}, nil
		}
	}

	return nil, flyerr.WithSuggestion(errors.New("no editor found"), "Set $EDITOR to your preferred editor.")
}

func transferFlags(parentsWhere string) []flag.Flag {
	return []flag.Flag{
		flag.Bool{
			Name:        "parents",
			Shorthand:   "p",
			Description: "Create missing parent directories " + parentsWhere,
		},
		flag.Bool{
			Name:        "recursive",
			Shorthand:   "r",
			Description: "Copy directories recursively",
		},
	}
}

func newFilePush() *cobra.Command {
	const (
		short = "Copy local files into a sprite."

		long = `Copy one or more local files or directories into a sprite.

The last argument is the remote destination. With several sources it is a
directory that each source is copied into.
`
	)

	cmd := newCommand("push [flags] <source>... [<sprite>:]<path>", short, long, runFilePush)
	cmd.Args = cobra.MinimumNArgs(2)
	cmd.Example = `  fly sprites file push -s my-sprite ./config.yml /etc/app/config.yml
  fly sprites file push ./config.yml my-sprite:/etc/app/config.yml
  fly sprites file push -s my-sprite ./a.txt ./b.txt /tmp/uploads/
  fly sprites file push -s my-sprite -r ./dist/ /srv/app/
  fly sprites file push -s my-sprite -p ./app.conf /etc/nested/dir/app.conf`

	flag.Add(cmd, transferFlags("in the sprite")...)

	return cmd
}

func runFilePush(ctx context.Context, s *session) error {
	ios := iostreams.FromContext(ctx)

	var (
		args      = flag.Args(ctx)
		sources   = args[:len(args)-1]
		parents   = flag.GetBool(ctx, "parents")
		recursive = flag.GetBool(ctx, "recursive")
	)

	targets, err := parseRemotePaths(ctx, s, args[len(args)-1:])
	if err != nil {
		return err
	}

	target := targets[0]
	fsys := target.fs()

	// Uploads create missing directories, so without --parents check that the
	// directory the files go into is really there rather than silently
	// creating it. A destination written with a trailing slash is that
	// directory; otherwise it is the destination's parent.
	if !parents {
		parent := strings.TrimSuffix(target.path, "/")
		if len(sources) == 1 && !strings.HasSuffix(target.path, "/") {
			parent = path.Dir(parent)
		}

		if parent == "" {
			parent = "/"
		}

		switch _, err := fsys.Stat(parent); {
		case errors.Is(err, fs.ErrNotExist):
			return flyerr.WithSuggestion(
				fmt.Errorf("remote directory %s does not exist", parent),
				"Pass --parents to create it.",
			)
		case err != nil:
			return fmt.Errorf("failed to check remote directory %s: %w", parent, err)
		}
	}

	// A destination that is an existing directory, or is written with a
	// trailing slash, receives the sources under their own names.
	intoDir := len(sources) > 1 || strings.HasSuffix(target.path, "/")
	if !intoDir {
		if info, err := fsys.Stat(target.path); err == nil && info.IsDir() {
			intoDir = true
		}
	}

	for _, source := range sources {
		info, err := os.Stat(source)
		if err != nil {
			return err
		}

		dest := target.path
		if intoDir && (!info.IsDir() || len(sources) > 1) {
			dest = path.Join(target.path, filepath.Base(filepath.Clean(source)))
		}

		if !info.IsDir() {
			if err := pushFile(ctx, ios.Out, fsys, source, dest); err != nil {
				return err
			}

			continue
		}

		if !recursive {
			return flyerr.WithSuggestion(fmt.Errorf("%s is a directory", source), "Pass --recursive to copy directories.")
		}

		if err := pushDir(ctx, ios.Out, target, source, dest); err != nil {
			return err
		}
	}

	return nil
}

func pushDir(ctx context.Context, out io.Writer, target remotePath, localDir, remoteDir string) error {
	fsys := target.fs()

	// Uploading a file creates its parent directories, so only directories
	// that end up with no files in them need creating.
	empty := map[string]bool{}

	err := filepath.WalkDir(localDir, func(local string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(localDir, local)
		if err != nil {
			return err
		}

		remote := path.Join(remoteDir, filepath.ToSlash(rel))

		switch {
		case d.IsDir():
			empty[remote] = true
			delete(empty, path.Dir(remote))

			return nil
		case !d.Type().IsRegular():
			// Symlinks, devices and sockets have no portable representation
			// over the file API.
			fmt.Fprintf(out, "skipping %s: not a regular file\n", local)
			return nil
		default:
			delete(empty, path.Dir(remote))

			return pushFile(ctx, out, fsys, local, remote)
		}
	})
	if err != nil || len(empty) == 0 {
		return err
	}

	dirs := make([]string, 0, len(empty))
	for dir := range empty {
		dirs = append(dirs, dir)
	}

	// The file API has no way to make a directory without putting a file in
	// it, so run mkdir in the sprite.
	mkdir := target.sprite.CommandContext(ctx, "mkdir", append([]string{"-p", "--"}, dirs...)...)
	mkdir.Dir = remoteHome

	if output, err := mkdir.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create empty directories in the sprite: %w: %s", err, strings.TrimSpace(string(output)))
	}

	return nil
}

func pushFile(ctx context.Context, out io.Writer, fsys sprites.FS, local, remote string) error {
	info, err := os.Stat(local)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(local)
	if err != nil {
		return err
	}

	if err := fsys.WriteFileContext(ctx, remote, data, info.Mode().Perm()); err != nil {
		return fmt.Errorf("failed to upload %s to %s: %w", local, remote, err)
	}

	fmt.Fprintln(out, remote)

	return nil
}

func newFilePull() *cobra.Command {
	const (
		short = "Copy files out of a sprite."

		long = `Copy one or more files or directories from a sprite to the local machine.

The last argument is the local destination. With several sources it is a
directory that each source is copied into. Sources may come from different
sprites.
`
	)

	cmd := newCommand("pull [flags] [<sprite>:]<path>... <target>", short, long, runFilePull)
	cmd.Args = cobra.MinimumNArgs(2)
	cmd.Example = `  fly sprites file pull -s my-sprite /var/log/app.log .
  fly sprites file pull -s my-sprite /etc/app/config.yml ./config.yml
  fly sprites file pull my-sprite:/tmp/a.txt other-sprite:/tmp/b.txt ./downloads/
  fly sprites file pull -s my-sprite -r /srv/app/dist/ ./dist/
  fly sprites file pull -s my-sprite -p /etc/nested/file.conf ./local/nested/file.conf`

	flag.Add(cmd, transferFlags("locally")...)

	return cmd
}

func runFilePull(ctx context.Context, s *session) error {
	ios := iostreams.FromContext(ctx)

	var (
		args      = flag.Args(ctx)
		target    = args[len(args)-1]
		parents   = flag.GetBool(ctx, "parents")
		recursive = flag.GetBool(ctx, "recursive")
	)

	sources, err := parseRemotePaths(ctx, s, args[:len(args)-1])
	if err != nil {
		return err
	}

	// As with push, a destination that is a directory, or is written with a
	// trailing separator, receives the sources under their own names.
	intoDir := len(sources) > 1 || strings.HasSuffix(target, "/") || strings.HasSuffix(target, string(filepath.Separator))

	// Like cp, a directory pulled to a directory that already exists goes
	// inside it under its own name, rather than being merged into it and
	// overwriting what is there.
	existingDir := false

	switch info, err := os.Stat(target); {
	case err == nil && info.IsDir():
		intoDir, existingDir = true, true
	case err == nil && len(sources) > 1:
		return errors.New("the target must be a directory when pulling multiple files")
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		return err
	case err != nil:
		// The target is new. Its parent must exist, or --parents be given.
		parent := filepath.Dir(filepath.Clean(target))

		if _, statErr := os.Stat(parent); statErr != nil {
			if !parents {
				return flyerr.WithSuggestion(
					fmt.Errorf("local directory %s does not exist", parent),
					"Pass --parents to create it.",
				)
			}

			if err := os.MkdirAll(parent, 0o755); err != nil {
				return err
			}
		}

		// A new target that is to hold the sources is a directory to make.
		if intoDir {
			if err := os.Mkdir(target, 0o755); err != nil {
				return err
			}
		}
	}

	skipped := 0

	for _, source := range sources {
		fsys := source.fs()

		info, err := fsys.Stat(source.path)
		if err != nil {
			return fmt.Errorf("%s: %w", source.path, err)
		}

		dest := target
		if intoDir && (!info.IsDir() || len(sources) > 1 || existingDir) {
			dest = filepath.Join(target, path.Base(path.Clean(source.path)))
		}

		if !info.IsDir() {
			if err := pullFile(ctx, ios.Out, fsys, source.path, dest, info.Mode()); err != nil {
				return err
			}

			continue
		}

		if !recursive {
			return flyerr.WithSuggestion(
				fmt.Errorf("%s is a directory in the sprite", source.path),
				"Pass --recursive to copy directories.",
			)
		}

		n, err := pullDir(ctx, ios.Out, fsys, source.path, dest)
		skipped += n

		if err != nil {
			return err
		}
	}

	if skipped > 0 {
		return fmt.Errorf("%d entries could not be copied; they are listed above, and are usually symlinks", skipped)
	}

	return nil
}

// pullDir copies a remote directory tree. An entry that cannot be read is
// reported and skipped rather than ending the copy: the API lists symlinks as
// ordinary files, so a dangling link, or a link to a directory, only shows
// itself when reading it fails. It returns how many entries were skipped.
func pullDir(ctx context.Context, out io.Writer, fsys sprites.FS, remoteDir, localDir string) (skipped int, err error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	if err := os.MkdirAll(localDir, 0o755); err != nil {
		return 0, err
	}

	entries, err := fsys.ReadDir(remoteDir)
	if err != nil {
		return 0, fmt.Errorf("failed to list %s: %w", remoteDir, err)
	}

	for _, entry := range entries {
		// A remote name must not be able to place a file outside localDir.
		name := entry.Name()
		if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
			return skipped, fmt.Errorf("refusing to copy remote entry with unsafe name %q", name)
		}

		remote := path.Join(remoteDir, name)
		local := filepath.Join(localDir, name)

		if entry.IsDir() {
			n, err := pullDir(ctx, out, fsys, remote, local)
			skipped += n

			if err != nil {
				return skipped, err
			}

			continue
		}

		info, err := entry.Info()
		if err != nil {
			return skipped, err
		}

		if err := pullFile(ctx, out, fsys, remote, local, info.Mode()); err != nil {
			// An interrupt or a local write failure ends the copy. Anything
			// else is this one entry's problem.
			var pathErr *fs.PathError
			if ctx.Err() != nil || !errors.As(err, &pathErr) || pathErr.Op != "read" {
				return skipped, err
			}

			fmt.Fprintf(out, "skipping %s: %v\n", remote, pathErr.Err)
			skipped++
		}
	}

	return skipped, nil
}

// contextReader is the context-aware read the SDK's filesystem offers beyond
// the FS interface.
type contextReader interface {
	ReadFileContext(ctx context.Context, name string) ([]byte, error)
}

func pullFile(ctx context.Context, out io.Writer, fsys sprites.FS, remote, local string, mode fs.FileMode) error {
	// Stop between files when interrupted, and mid-download where the SDK
	// allows it.
	if err := ctx.Err(); err != nil {
		return err
	}

	var (
		data []byte
		err  error
	)

	if cr, ok := fsys.(contextReader); ok {
		data, err = cr.ReadFileContext(ctx, remote)
	} else {
		data, err = fsys.ReadFile(remote)
	}

	if err != nil {
		return fmt.Errorf("failed to download %s: %w", remote, err)
	}

	perm := mode.Perm()
	if perm == 0 {
		perm = 0o644
	}

	if err := os.WriteFile(local, data, perm); err != nil {
		return err
	}

	fmt.Fprintln(out, local)

	return nil
}
