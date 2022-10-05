package ssh

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path"
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

func newSftp() *cobra.Command {
	const (
		long  = `Get or put files from a remote VM.`
		short = long
		usage = "sftp"
	)

	cmd := command.New("sftp", short, long, nil)

	cmd.AddCommand(
		newLs(),
		newShell(),
	)

	return cmd
}

func newShell() *cobra.Command {
	const (
		long  = `tktktktk.`
		short = long
		usage = "shell"
	)

	cmd := command.New(usage, short, long, runShell, command.RequireSession, command.LoadAppNameIfPresent)

	stdArgsSSH(cmd)

	return cmd
}

func newLs() *cobra.Command {
	const (
		long  = `tktktktk.`
		short = long
		usage = "ls [path]"
	)

	cmd := command.New(usage, short, long, runLs, command.RequireSession, command.LoadAppNameIfPresent)

	stdArgsSSH(cmd)

	return cmd
}

func newSFTP(ctx context.Context) (*sftp.Client, error) {
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
	ftp, err := newSFTP(ctx)
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

var completer = readline.NewPrefixCompleter(
	readline.PcItem("ls"),
	readline.PcItem("cd"),
	readline.PcItem("get"),
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

func (sc *sftpContext) ls(args ...string) error {
	rpath := sc.wd

	if len(args) > 2 {
		if args[1][0] == '/' {
			rpath = args[1]
		} else {
			rpath = sc.wd + rpath
		}
	}

	files, err := sc.ftp.ReadDir(rpath)
	if err != nil {
		sc.out("ls: %s", err)
		return nil
	}

	for _, f := range files {
		tl := ""
		if f.IsDir() {
			tl = "/"
		}

		sc.out("%s%s\n", f.Name(), tl)
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

	f, err := os.OpenFile(lpath, os.O_WRONLY|os.O_CREATE, 0644)
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

func (sc *sftpContext) get(args ...string) error {
	if len(args) < 2 {
		sc.out("get <filename>")
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

		f, err := os.OpenFile(localFile, os.O_WRONLY|os.O_CREATE, 0644)
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
	ftp, err := newSFTP(ctx)
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
		}
	}

	return nil
}
