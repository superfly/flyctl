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
	"strconv"
	"strings"

	"github.com/pkg/sftp"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"

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
	)

	return cmd
}

func newSFTPShell() *cobra.Command {
	const (
		long  = `The SFTP SHELL command brings up an interactive SFTP session to fetch and push files to a VM:.`
		short = long
		usage = "shell"
	)

	cmd := command.New(usage, short, long, runShell, command.RequireSession, command.LoadAppNameIfPresent)

	stdArgsSSH(cmd)

	return cmd
}

func newFind() *cobra.Command {
	const (
		long  = `The SFTP FIND command lists files (from an optional root directory) on a remote VM.`
		short = long
		usage = "find [path]"
	)

	cmd := command.New(usage, short, long, runLs, command.RequireSession, command.LoadAppNameIfPresent)

	stdArgsSSH(cmd)

	return cmd
}

func newGet() *cobra.Command {
	const (
		long  = `The SFTP GET retrieves a file from a remote VM.`
		short = long
		usage = "get <path>"
	)

	cmd := command.New(usage, short, long, runGet, command.RequireSession, command.LoadAppNameIfPresent)

	cmd.Args = cobra.MaximumNArgs(2)

	stdArgsSSH(cmd)

	return cmd

}

func newSFTPConnection(ctx context.Context) (*sftp.Client, error) {
	client := client.FromContext(ctx).API()
	appName := app.NameFromContext(ctx)

	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return nil, fmt.Errorf("get app: %w", err)
	}

	agentclient, dialer, err := bringUp(ctx, client, app)
	if err != nil {
		return nil, err
	}

	addr, err := lookupAddress(ctx, agentclient, dialer, app, false)
	if err != nil {
		return nil, err
	}

	params := &SSHParams{
		Ctx:            ctx,
		Org:            app.Organization,
		Dialer:         dialer,
		App:            appName,
		Stdin:          os.Stdin,
		Stdout:         os.Stdout,
		Stderr:         os.Stderr,
		DisableSpinner: true,
	}

	conn, err := sshConnect(params, addr)
	if err != nil {
		captureError(err, app)
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

		fmt.Printf(walker.Path() + "\n")
	}

	return nil
}

func runGet(ctx context.Context) error {
	args := flag.Args(ctx)

	var remote string
	var local string

	switch len(args) {
	case 0:
		fmt.Printf("get <remote-path> [local-path]\n")
		return nil

	case 1:
		remote = args[0]
		local = remote

	default:
		remote = args[0]
		local = args[1]
	}

	if _, err := os.Stat(local); err == nil {
		return fmt.Errorf("get: local file %s: already exists", remote)
	}

	ftp, err := newSFTPConnection(ctx)
	if err != nil {
		return err
	}

	rf, err := ftp.Open(remote)
	if err != nil {
		return fmt.Errorf("get: remote file %s: %w", remote, err)
	}
	defer rf.Close()

	f, err := os.OpenFile(local, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0644)
	if err != nil {
		return fmt.Errorf("get: local file %s: %w", local, err)
	}
	defer f.Close()

	bytes, err := rf.WriteTo(f)
	if err != nil {
		return fmt.Errorf("get: copy file: %w (%d bytes written)", err, bytes)
	}

	fmt.Printf("%d bytes written to %s\n", bytes, local)
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

	f, err := os.OpenFile(lpath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
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

	rpath := sc.wd + path.Base(lpath)
	if rarg := fgs.Arg(1); rarg != "" {
		if rarg[0] == '/' {
			rpath = rarg
		} else {
			rpath = sc.wd + rarg
		}
	}

	if _, err = sc.ftp.Stat(rpath); err == nil {
		sc.out("put %s -> %s: file exists on VM", lpath, rpath)
		return nil
	}

	f, err := os.Open(lpath)
	if err != nil {
		sc.out("put %s -> %s: open local file: %s", lpath, rpath, err)
		return nil
	}
	defer f.Close()

	rf, err := sc.ftp.OpenFile(rpath, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
	if err != nil {
		sc.out("put %s -> %s: create remote file: %s", lpath, rpath, err)
		return nil
	}
	defer rf.Close()

	bytes, err := rf.ReadFrom(f)
	if err != nil {
		sc.out("put %s -> %s: copy file file: %s (%d bytes written)", lpath, rpath, err, bytes)
		return nil
	}

	sc.out("%d bytes written", bytes)

	if err = sc.ftp.Chmod(rpath, fs.FileMode(permbits)); err != nil {
		sc.out("put %s -> %s: set permissions: %s", lpath, rpath, err)
		return nil
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
		sc.out("get %s -> %s: file exists", rpath, localFile)
		return nil
	}

	func() {
		rf, err := sc.ftp.Open(rpath)
		if err != nil {
			sc.out("get %s -> %s: %s", err)
			return
		}
		defer rf.Close()

		f, err := os.OpenFile(localFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
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

		default:
			out("unrecognized command; try 'cd', 'ls', 'get', 'put', or 'chmod'")
		}
	}

	return nil
}
