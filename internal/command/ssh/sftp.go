package ssh

import (
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

	_ = ftp

	wd := "/"

	out := func(format string, args ...interface{}) {
		fmt.Printf(format+"\n", args...)
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

		line = strings.TrimSpace(line)

		switch {
		case strings.HasPrefix(line, "cd"):
			args, _ := shlex.Split(line)
			if len(args) < 2 {
				wd = "/"
				continue
			}

			dir := args[1]
			if dir[0] != '/' {
				dir = wd + dir
			}

			if !strings.HasSuffix(dir, "/") {
				dir = dir + "/"
			}

			inf, err := ftp.Stat(dir)
			if err != nil {
				out("cd %s: %s", dir, err)
				continue
			}

			if !inf.IsDir() {
				out("cd %s: not a directory", dir)
				continue
			}

			dir = path.Clean(dir) + "/"

			out("[%s]", dir)
			wd = dir

		case strings.HasPrefix(line, "ls"):
			files, err := ftp.ReadDir(wd)
			if err != nil {
				fmt.Printf("ls: %s", err)
				continue
			}

			for _, f := range files {
				tl := ""
				if f.IsDir() {
					tl = "/"
				}

				fmt.Printf("%s%s\n", f.Name(), tl)
			}

		case strings.HasPrefix(line, "get"):
			args, _ := shlex.Split(line)
			if len(args) < 2 {
				out("get <file>")
				continue
			}

			rpath := wd + args[1]

			inf, err := ftp.Stat(rpath)
			if err != nil {
				out("get %s: %s", rpath, err)
				continue
			}

			if inf.IsDir() {
				out("get %s: is a directory", rpath)
				continue
			}

			localFile := path.Base(rpath)
			if len(args) > 2 {
				localFile = args[2]
			}

			_, err = os.Stat(localFile)
			if err == nil {
				out("get %s -> %s: file exists", rpath, localFile)
				continue
			}

			func() {
				rf, err := ftp.Open(rpath)
				if err != nil {
					out("get %s -> %s: %s", err)
					return
				}
				defer rf.Close()

				f, err := os.OpenFile(localFile, os.O_WRONLY|os.O_CREATE, 0644)
				if err != nil {
					out("get %s -> %s: %s", rpath, localFile, err)
					return
				}
				defer f.Close()

				out("get %s -> %s", rpath, localFile)

				bytes, err := rf.WriteTo(f)
				if err != nil {
					out("get %s -> %s: %s (wrote %d bytes)", rpath, localFile, err, bytes)
				} else {
					out("wrote %d bytes", bytes)
				}
			}()
		}
	}

	return nil
}
