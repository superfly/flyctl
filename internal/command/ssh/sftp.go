package ssh

import (
	"archive/zip"
	"context"
	goflag "flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pkg/sftp"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"

	"github.com/chzyer/readline"
	"github.com/google/shlex"
)

func NewSFTP() *cobra.Command {
	const (
		long  = `Get or put files from a remote VM.`
		short = long
		usage = "sftp"
	)

	cmd := command.New("sftp", short, long, nil)

	cmd.AddCommand(
		newFind(),
		newSFTPShell(),
		newGet(),
		newPut(),
	)

	return cmd
}

func newSFTPShell() *cobra.Command {
	const (
		long  = `The SFTP SHELL command brings up an interactive SFTP session to fetch and push files from/to a VM.`
		short = long
		usage = "shell"
	)

	cmd := command.New(usage, short, long, runShell, command.RequireSession, command.RequireAppName)

	stdArgsSSH(cmd)

	return cmd
}

func newFind() *cobra.Command {
	const (
		long  = `The SFTP FIND command lists files (from an optional root directory) on a remote VM.`
		short = long
		usage = "find [path]"
	)

	cmd := command.New(usage, short, long, runLs, command.RequireSession, command.RequireAppName)

	stdArgsSSH(cmd)

	return cmd
}

func newGet() *cobra.Command {
	const (
		long  = `The SFTP GET retrieves a file from a remote VM.`
		short = long
		usage = "get <remote-path> [local-path]"
	)

	cmd := command.New(usage, short, long, runGet, command.RequireSession, command.RequireAppName)

	cmd.Args = cobra.MaximumNArgs(2)

	flag.Add(cmd,
		flag.Bool{
			Name:        "recursive",
			Shorthand:   "R",
			Description: "Download directories recursively",
			Default:     false,
		},
	)

	stdArgsSSH(cmd)

	return cmd
}

func newPut() *cobra.Command {
	const (
		long  = `The SFTP PUT uploads a file to a remote VM.`
		short = long
		usage = "put <local-path> [remote-path]"
	)

	cmd := command.New(usage, short, long, runPut, command.RequireSession, command.RequireAppName)

	cmd.Args = cobra.RangeArgs(1, 2)

	flag.Add(cmd,
		flag.String{
			Name:        "mode",
			Shorthand:   "m",
			Description: "File mode/permissions for the uploaded file (default: 0644)",
			Default:     "0644",
		},
		flag.Bool{
			Name:        "recursive",
			Shorthand:   "R",
			Description: "Upload directories recursively",
			Default:     false,
		},
	)

	stdArgsSSH(cmd)

	return cmd
}

func newSFTPConnection(ctx context.Context) (*sftp.Client, error) {
	client := flyutil.ClientFromContext(ctx)
	appName := appconfig.NameFromContext(ctx)

	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return nil, fmt.Errorf("get app: %w", err)
	}

	network, err := client.GetAppNetwork(ctx, appName)
	if err != nil {
		return nil, fmt.Errorf("get app network: %w", err)
	}

	agentclient, dialer, err := agent.BringUpAgent(ctx, client, app, *network, quiet(ctx))
	if err != nil {
		return nil, err
	}

	addr, container, err := lookupAddressAndContainer(ctx, agentclient, dialer, app, false)
	if err != nil {
		return nil, err
	}

	params := &ConnectParams{
		Ctx:            ctx,
		Org:            app.Organization,
		Dialer:         dialer,
		Username:       DefaultSshUsername,
		DisableSpinner: true,
		Container:      container,
		AppNames:       []string{app.Name},
	}

	conn, err := Connect(params, addr)
	if err != nil {
		captureError(ctx, err, app)
		return nil, err
	}

	return sftp.NewClient(conn.Client,
		sftp.UseConcurrentReads(true),
		sftp.UseConcurrentWrites(true),
	)
}

func runLs(ctx context.Context) error {
	ftp, err := newSFTPConnection(ctx)
	if err != nil {
		return err
	}

	root := "/"
	args := flag.Args(ctx)
	if len(args) != 0 {
		root = args[0]
	}

	walker := ftp.Walk(root)
	for walker.Step() {
		if err = walker.Err(); err != nil {
			return err
		}

		fmt.Println(walker.Path())
	}

	return nil
}

func runGet(ctx context.Context) error {
	args := flag.Args(ctx)

	var remote, local string

	switch len(args) {
	case 0:
		fmt.Printf("get <remote-path> [local-path]\n")
		return nil

	case 1:
		remote = args[0]
		local = filepath.Base(remote)

	default:
		remote = args[0]
		local = args[1]
	}

	if _, err := os.Stat(local); err == nil {
		return fmt.Errorf("file %s is already there. `fly ssh` doesn't override existing files for safety.", local)
	}

	ftp, err := newSFTPConnection(ctx)
	if err != nil {
		return err
	}

	// Check if remote is a directory
	remoteInfo, err := ftp.Stat(remote)
	if err != nil {
		return fmt.Errorf("get: remote path %s: %w", remote, err)
	}

	if remoteInfo.IsDir() {
		recursive := flag.GetBool(ctx, "recursive")
		if !recursive {
			return fmt.Errorf("remote path %s is a directory. Use -R/--recursive flag to download directories", remote)
		}
		return runGetDir(ctx, ftp, remote, local)
	}

	rf, err := ftp.Open(remote)
	if err != nil {
		return fmt.Errorf("get: remote file %s: %w", remote, err)
	}
	defer rf.Close()

	f, err := os.OpenFile(local, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("get: local file %s: %w", local, err)
	}
	defer f.Close()

	bytes, err := rf.WriteTo(f)
	if err != nil {
		return fmt.Errorf("get: copy file: %w (%d bytes written)", err, bytes)
	}

	fmt.Printf("%d bytes written to %s\n", bytes, local)
	return f.Sync()
}

func runGetDir(ctx context.Context, ftp *sftp.Client, remote, local string) error {
	// Check if target directory already exists
	if _, err := os.Stat(local); err == nil {
		return fmt.Errorf("directory %s already exists. flyctl sftp doesn't override existing directories for safety", local)
	}

	// Create temporary ZIP file
	tempZip, err := os.CreateTemp("", "flyctl-sftp-*.zip")
	if err != nil {
		return fmt.Errorf("create temporary zip file: %w", err)
	}
	defer os.Remove(tempZip.Name()) // Clean up temp file
	defer tempZip.Close()

	z := zip.NewWriter(tempZip)
	walker := ftp.Walk(remote)
	totalBytes := int64(0)

	// Download all files into ZIP
	for walker.Step() {
		if err = walker.Err(); err != nil {
			return fmt.Errorf("walk remote directory: %w", err)
		}

		rfpath := walker.Path()

		inf, err := ftp.Stat(rfpath)
		if err != nil {
			fmt.Printf("warning: stat %s: %s\n", rfpath, err)
			continue
		}

		if inf.IsDir() {
			continue
		}

		rf, err := ftp.Open(rfpath)
		if err != nil {
			fmt.Printf("warning: open %s: %s\n", rfpath, err)
			continue
		}

		// Create relative path for ZIP entry
		relPath := strings.TrimPrefix(rfpath, remote)
		relPath = strings.TrimPrefix(relPath, "/")
		if relPath == "" {
			relPath = filepath.Base(rfpath)
		}

		zf, err := z.Create(relPath)
		if err != nil {
			rf.Close()
			fmt.Printf("warning: create zip entry %s: %s\n", relPath, err)
			continue
		}

		bytes, err := rf.WriteTo(zf)
		if err != nil {
			fmt.Printf("warning: write %s: %s (%d bytes written)\n", relPath, err, bytes)
		} else {
			fmt.Printf("downloaded %s (%d bytes)\n", relPath, bytes)
		}
		totalBytes += bytes

		rf.Close()
	}

	// Close ZIP writer and temp file
	z.Close()
	tempZip.Close()

	// Extract ZIP to target directory
	err = extractZip(tempZip.Name(), local)
	if err != nil {
		return fmt.Errorf("extract directory: %w", err)
	}

	fmt.Printf("extracted %d bytes to %s/\n", totalBytes, local)
	return nil
}

func extractZip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	// Create destination directory
	err = os.MkdirAll(dest, 0755)
	if err != nil {
		return err
	}

	// Extract files
	for _, f := range r.File {
		path := filepath.Join(dest, f.Name)

		// Security check: ensure path is within destination
		if !strings.HasPrefix(path, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid file path: %s", f.Name)
		}

		if f.FileInfo().IsDir() {
			err = os.MkdirAll(path, f.FileInfo().Mode())
			if err != nil {
				return err
			}
			continue
		}

		// Create parent directories
		err = os.MkdirAll(filepath.Dir(path), 0755)
		if err != nil {
			return err
		}

		// Extract file
		rc, err := f.Open()
		if err != nil {
			return err
		}

		outFile, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.FileInfo().Mode())
		if err != nil {
			rc.Close()
			return err
		}

		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()

		if err != nil {
			return err
		}
	}

	return nil
}

func runPut(ctx context.Context) error {
	args := flag.Args(ctx)

	var local, remote string

	switch len(args) {
	case 0:
		fmt.Printf("put <local-path> [remote-path]\n")
		return nil

	case 1:
		local = args[0]
		remote = filepath.Base(local)

	default:
		local = args[0]
		remote = args[1]
	}

	// Parse file mode
	modeStr := flag.GetString(ctx, "mode")
	mode, err := strconv.ParseInt(modeStr, 8, 16)
	if err != nil {
		return fmt.Errorf("invalid file mode '%s': %w", modeStr, err)
	}

	// Check if local file exists and is readable
	localInfo, err := os.Stat(local)
	if err != nil {
		return fmt.Errorf("local file %s: %w", local, err)
	}

	ftp, err := newSFTPConnection(ctx)
	if err != nil {
		return err
	}

	if localInfo.IsDir() {
		recursive := flag.GetBool(ctx, "recursive")
		if !recursive {
			return fmt.Errorf("local path %s is a directory. Use -R/--recursive flag to upload directories", local)
		}
		return runPutDir(ctx, ftp, local, remote, fs.FileMode(mode))
	}

	// Check if remote file already exists
	if _, err := ftp.Stat(remote); err == nil {
		return fmt.Errorf("remote file %s already exists. flyctl sftp doesn't overwrite existing files for safety", remote)
	}

	// Open local file
	localFile, err := os.Open(local)
	if err != nil {
		return fmt.Errorf("open local file %s: %w", local, err)
	}
	defer localFile.Close()

	// Create remote file
	remoteFile, err := ftp.OpenFile(remote, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
	if err != nil {
		return fmt.Errorf("create remote file %s: %w", remote, err)
	}
	defer remoteFile.Close()

	// Copy file contents
	bytes, err := remoteFile.ReadFrom(localFile)
	if err != nil {
		return fmt.Errorf("copy file: %w (%d bytes written)", err, bytes)
	}

	// Set file permissions
	if err = ftp.Chmod(remote, fs.FileMode(mode)); err != nil {
		return fmt.Errorf("set file permissions: %w", err)
	}

	fmt.Printf("%d bytes uploaded to %s\n", bytes, remote)
	return nil
}

func runPutDir(ctx context.Context, ftp *sftp.Client, localDir, remoteDir string, mode fs.FileMode) error {
	// Check if remote directory already exists
	if _, err := ftp.Stat(remoteDir); err == nil {
		return fmt.Errorf("remote directory %s already exists. flyctl sftp doesn't overwrite existing directories for safety", remoteDir)
	}

	totalBytes := int64(0)
	totalFiles := 0

	err := filepath.Walk(localDir, func(localPath string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("walk local directory: %w", err)
		}

		// Create relative path for remote
		relPath, err := filepath.Rel(localDir, localPath)
		if err != nil {
			return fmt.Errorf("get relative path: %w", err)
		}

		remotePath := filepath.Join(remoteDir, relPath)
		// Convert to forward slashes for remote paths
		remotePath = strings.ReplaceAll(remotePath, "\\", "/")

		if info.IsDir() {
			// Create remote directory
			err := ftp.MkdirAll(remotePath)
			if err != nil {
				return fmt.Errorf("create remote directory %s: %w", remotePath, err)
			}
			fmt.Printf("created directory %s\n", remotePath)
		} else {
			// Create parent directories if they don't exist
			remoteDir := filepath.Dir(remotePath)
			remoteDir = strings.ReplaceAll(remoteDir, "\\", "/")
			if remoteDir != "." {
				err := ftp.MkdirAll(remoteDir)
				if err != nil {
					return fmt.Errorf("create parent directory %s: %w", remoteDir, err)
				}
			}

			// Upload file
			localFile, err := os.Open(localPath)
			if err != nil {
				return fmt.Errorf("open local file %s: %w", localPath, err)
			}
			defer localFile.Close()

			remoteFile, err := ftp.OpenFile(remotePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
			if err != nil {
				return fmt.Errorf("create remote file %s: %w", remotePath, err)
			}
			defer remoteFile.Close()

			bytes, err := remoteFile.ReadFrom(localFile)
			if err != nil {
				return fmt.Errorf("copy file %s: %w (%d bytes written)", localPath, err, bytes)
			}

			// Set file permissions
			if err = ftp.Chmod(remotePath, mode); err != nil {
				fmt.Printf("warning: set permissions for %s: %s\n", remotePath, err)
			}

			fmt.Printf("uploaded %s (%d bytes)\n", remotePath, bytes)
			totalBytes += bytes
			totalFiles++
		}

		return nil
	})

	if err != nil {
		return err
	}

	fmt.Printf("%d files uploaded (%d bytes total) to %s\n", totalFiles, totalBytes, remoteDir)
	return nil
}

var completer = readline.NewPrefixCompleter(
	readline.PcItem("ls"),
	readline.PcItem("cd"),
	readline.PcItem("get"),
	readline.PcItem("put"),
	readline.PcItem("chmod"),
)

type sftpContext struct {
	ftp *sftp.Client
	wd  string
	out func(string, ...interface{})
}

func (sc *sftpContext) cd(args ...string) error {
	if len(args) < 2 {
		sc.wd = "/"
		return nil
	}

	dir := args[1]
	if dir[0] != '/' {
		dir = sc.wd + dir
	}

	if !strings.HasSuffix(dir, "/") {
		dir = dir + "/"
	}

	inf, err := sc.ftp.Stat(dir)
	if err != nil {
		sc.out("cd %s: %s", dir, err)
		return nil
	}

	if !inf.IsDir() {
		sc.out("cd %s: not a directory", dir)
		return nil
	}

	dir = path.Clean(dir) + "/"

	sc.out("[%s]", dir)
	sc.wd = dir

	return nil
}

// BUG(tqbf): these return `error` because in theory an error might be bad enough
// that we want to kill the session, but nothing does that right now.
func (sc *sftpContext) ls(args ...string) error {
	fgs := goflag.NewFlagSet("ls", goflag.ContinueOnError)

	long := fgs.Bool("l", false, "detailed file output")

	if err := fgs.Parse(args[1:]); err != nil {
		sc.out("ls: invalid arguments: %s", err)
		return nil
	}

	rpath := sc.wd

	if rarg := fgs.Arg(0); rarg != "" {
		if rarg[0] == '/' {
			rpath = rarg
		} else {
			rpath = sc.wd + rarg
		}
	}

	files, err := sc.ftp.ReadDir(rpath)
	if err != nil {
		sc.out("ls: %s", err)
		return nil
	}

	for _, f := range files {
		if !*long {
			tl := ""
			if f.IsDir() {
				tl = "/"
			}

			sc.out("%s%s", f.Name(), tl)
		} else {
			if f.IsDir() {
				sc.out("%s      -\t%s\t%s/", f.Mode().String(), f.ModTime(), f.Name())
			} else {
				sc.out("%s  %d\t%s\t%s", f.Mode().String(), f.Size(), f.ModTime(), f.Name())
			}
		}
	}

	return nil
}

func (sc *sftpContext) getDir(rpath string, args []string) {
	lpath := path.Base(rpath) + ".zip"

	if len(args) > 2 {
		lpath = args[2]

		if !strings.HasSuffix(lpath, ".zip") {
			lpath += ".zip"
		}
	}

	if _, err := os.Stat(lpath); err == nil {
		sc.out("get %s -> %s: file exists", rpath, lpath)
		return
	}

	f, err := os.OpenFile(lpath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		sc.out("get %s -> %s: %s", rpath, lpath, err)
		return
	}
	defer f.Close()
	z := zip.NewWriter(f)

	defer z.Close()

	walker := sc.ftp.Walk(rpath)

	for walker.Step() {
		if err = walker.Err(); err != nil {
			sc.out("get %s -> %s: walk: %s", rpath, lpath, err)
			break
		}

		rfpath := walker.Path()

		inf, err := sc.ftp.Stat(rfpath)
		if err != nil {
			sc.out("get %s -> %s: stat %s: %s", rpath, lpath, rfpath, err)
			continue
		}

		if inf.IsDir() {
			continue
		}

		rf, err := sc.ftp.Open(rfpath)
		if err != nil {
			sc.out("get %s -> %s: open %s: %s", rpath, lpath, rfpath, err)
			continue
		}

		zf, err := z.Create(rfpath)
		if err != nil {
			rf.Close()
			sc.out("get %s -> %s: write %s: %s", rpath, lpath, rfpath, err)
			continue
		}

		bytes, err := rf.WriteTo(zf)
		if err != nil {
			sc.out("get %s -> %s: write %s: %s (wrote %d bytes)", rpath, lpath, rfpath, err, bytes)
		} else {
			sc.out("%s (%d bytes)", rfpath, bytes)
		}

		rf.Close()
	}

	z.Close()

	err = f.Sync()
	if err != nil {
		sc.out("failed to sync %s: %s", lpath, err)
	}
}

func (sc *sftpContext) chmod(args ...string) error {
	if len(args) < 3 {
		sc.out("chmod <numeric-mode> <file>")
		return nil
	}

	mode, err := strconv.ParseInt(args[1], 8, 16)
	if err != nil {
		sc.out("chmod: invalid permissions (only numeric allowed) '%s': %s", args[1], err)
		return nil
	}

	rpath := args[2]
	if rpath[0] != '/' {
		rpath = sc.wd + rpath
	}

	if err = sc.ftp.Chmod(rpath, fs.FileMode(mode)); err != nil {
		sc.out("chmod %s: %s", rpath, err)
		return nil
	}

	return nil
}

func (sc *sftpContext) rm(args ...string) error {
	if len(args) < 2 {
		sc.out("rm <file>")
		return nil
	}

	rpath := sc.wd

	rarg := args[1]
	if rarg[0] == '/' {
		rpath = rarg
	} else {
		rpath = sc.wd + rarg
	}

	if rarg := args[1]; rarg != "" {
		if rarg[0] == '/' {
			rpath = rarg
		} else {
			rpath = sc.wd + rarg
		}
	}

	if _, err := sc.ftp.Stat(rpath); err != nil {
		sc.out("rm %s: %s does not exist on VM", rpath, rpath)
		return nil
	}

	if err := sc.ftp.Remove(rpath); err != nil {
		sc.out("rm %s: could not delete file", rpath)
		return nil
	}

	return nil
}

func (sc *sftpContext) uploadFile(lpath string, rpath string, permbits int64) {
	if _, err := sc.ftp.Stat(rpath); err == nil {
		sc.out("put %s -> %s: file exists on VM", lpath, rpath)
		return
	}

	f, err := os.Open(lpath)
	if err != nil {
		sc.out("put %s -> %s: open local file: %s", lpath, rpath, err)
		return
	}
	// Safe to ignore the error because this file is for reading.
	defer f.Close() // skipcq: GO-S2307

	rf, err := sc.ftp.OpenFile(rpath, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
	if err != nil {
		sc.out("put %s -> %s: create remote file: %s", lpath, rpath, err)
		return
	}
	defer rf.Close()

	bytes, err := rf.ReadFrom(f)
	if err != nil {
		sc.out("put %s -> %s: copy file file: %s (%d bytes written)", lpath, rpath, err, bytes)
		return
	}

	sc.out("put %s -> %s: %d bytes written", lpath, rpath, bytes)

	if err = sc.ftp.Chmod(rpath, fs.FileMode(permbits)); err != nil {
		sc.out("put %s -> %s: set permissions: %s", lpath, rpath, err)
		return
	}
}

func (sc *sftpContext) put(args ...string) error {
	fgs := goflag.NewFlagSet("put", goflag.ContinueOnError)

	perm := fgs.String("m", "0644", "file mode")

	permbits, err := strconv.ParseInt(*perm, 8, 16)
	if err != nil {
		sc.out("put: invalid permissions (only numeric allowed) '%s': %s", *perm, err)
		return nil
	}

	if err := fgs.Parse(args[1:]); err != nil {
		sc.out("put [-m] <local-filename> [filename]")
		return nil
	}

	lpath := fgs.Arg(0)
	if lpath == "" {
		sc.out("put [-m] <local-filename> [filename]")
		return nil
	}

	matches, err := filepath.Glob(lpath)
	if err != nil {
		sc.out("put: match glob %s: %s", lpath, err)
		return nil
	}

	if len(matches) == 0 {
		sc.out("put: no local files matched the pattern %s", lpath)
		return nil
	}

	rarg := fgs.Arg(1)

	if len(matches) == 1 {
		lpath = filepath.ToSlash(matches[0])

		rpath := sc.wd + path.Base(lpath)
		if rarg != "" {
			if rarg[0] == '/' {
				rpath = rarg
			} else {
				rpath = sc.wd + rarg
			}
		}

		sc.out("put %s -> %s: matched 1 file, uploading...", lpath, rpath)
		sc.uploadFile(lpath, rpath, permbits)
	} else {
		rdir := sc.wd
		if rarg != "" {
			if rarg[0] == '/' {
				rdir = rarg
			} else {
				rdir = sc.wd + rarg
			}
		}
		rdir = filepath.ToSlash(rdir)

		inf, err := sc.ftp.Stat(rdir)
		if err != nil {
			sc.out("put %s -> %s: check remote path: %s", lpath, rdir, err)
			return nil
		}

		if !inf.IsDir() {
			sc.out("put %s -> %s: remote path %s is not a directory", lpath, rdir, rdir)
			return nil
		}

		n := len(matches)
		sc.out("put %s -> %s: matched %d files, uploading...", lpath, rdir, n)

		for i, file := range matches {
			file = filepath.ToSlash(file)
			filename := path.Base(file)
			rpath := path.Join(rdir, filename)
			sc.out("put %s -> %s: uploading file %d/%d", file, rpath, i+1, n)
			sc.uploadFile(file, rpath, permbits)
		}
	}

	return nil
}

func (sc *sftpContext) get(args ...string) error {
	if len(args) < 2 {
		sc.out("get <filename> [local-filename]")
		return nil
	}

	rpath := sc.wd + args[1]

	inf, err := sc.ftp.Stat(rpath)
	if err != nil {
		sc.out("get %s: %s", rpath, err)
		return nil
	}

	if inf.IsDir() {
		sc.getDir(rpath, args)
		return nil
	}

	localFile := path.Base(rpath)
	if len(args) > 2 {
		localFile = args[2]
	}

	_, err = os.Stat(localFile)
	if err == nil {
		sc.out("file %s is already there. `fly ssh` doesn't overwrite existing files for safety.", localFile)
		return nil
	}

	func() {
		rf, err := sc.ftp.Open(rpath)
		if err != nil {
			sc.out("get %s -> %s: %s", err)
			return
		}
		defer rf.Close()

		f, err := os.OpenFile(localFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			sc.out("get %s -> %s: %s", rpath, localFile, err)
			return
		}
		defer f.Close()

		sc.out("get %s -> %s", rpath, localFile)

		bytes, err := rf.WriteTo(f)
		if err != nil {
			sc.out("get %s -> %s: %s (wrote %d bytes)", rpath, localFile, err, bytes)
		} else {
			sc.out("wrote %d bytes", bytes)
		}
		err = f.Sync()
		if err != nil {
			sc.out("failed to sync %s: %s", localFile, err)
		}
	}()

	return nil
}

func runShell(ctx context.Context) error {
	ftp, err := newSFTPConnection(ctx)
	if err != nil {
		return err
	}

	l, err := readline.NewEx(&readline.Config{
		Prompt:          "\033[31m»\033[0m ",
		AutoComplete:    completer,
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",

		HistorySearchFold: true,
		// FuncFilterInputRune: filterInput,
	})
	if err != nil {
		return err
	}

	defer l.Close()
	l.CaptureExitSignal()

	out := func(format string, args ...interface{}) {
		fmt.Printf(format+"\n", args...)
	}

	sc := &sftpContext{
		wd:  "/",
		out: out,
		ftp: ftp,
	}

	for {
		line, err := l.Readline()
		if err == readline.ErrInterrupt {
			if len(line) == 0 {
				break
			} else {
				continue
			}
		} else if err == io.EOF {
			break
		}

		args, err := shlex.Split(strings.TrimSpace(line))
		if err != nil {
			out("read command: %s", err)
			continue
		}

		if len(args) == 0 {
			continue
		}

		switch args[0] {
		case "cd":
			if err = sc.cd(args...); err != nil {
				return err
			}

		case "ls":
			if err = sc.ls(args...); err != nil {
				return err
			}

		case "get":
			if err = sc.get(args...); err != nil {
				return err
			}

		case "put":
			if err = sc.put(args...); err != nil {
				return err
			}

		case "chmod":
			if err = sc.chmod(args...); err != nil {
				return err
			}

		case "rm":
			if err = sc.rm(args...); err != nil {
				return err
			}

		default:
			out("unrecognized command; try 'cd', 'ls', 'get', 'put', 'chmod' or 'rm'")
		}
	}

	return nil
}
