//go:build windows

package ssh

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/require"
	flyssh "github.com/superfly/flyctl/ssh"
	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/sys/windows"
)

// Temporary native-console CI proof; remove before merging.
func TestConsoleConPTY(t *testing.T) {
	for _, status := range []int{0, 1, 2} {
		t.Run(fmt.Sprintf("exit-%d", status), func(t *testing.T) {
			t.Setenv("FLYCTL_CONPTY_PROOF_CHILD", strconv.Itoa(status))
			inputR, inputW, err := os.Pipe()
			require.NoError(t, err)
			defer inputR.Close()
			defer inputW.Close()
			outputR, outputW, err := os.Pipe()
			require.NoError(t, err)
			defer outputR.Close()
			defer outputW.Close()
			var console windows.Handle
			require.NoError(t, windows.CreatePseudoConsole(windows.Coord{X: 120, Y: 40}, windows.Handle(inputR.Fd()), windows.Handle(outputW.Fd()), 0, &console))
			defer func() {
				if console != 0 {
					windows.ClosePseudoConsole(console)
				}
			}()
			require.NoError(t, inputR.Close())
			require.NoError(t, outputW.Close())

			ready := make(chan struct{}, 1)
			output := make(chan string, 1)
			go func() {
				var collected strings.Builder
				buf := make([]byte, 4096)
				signaled := false
				for {
					n, readErr := outputR.Read(buf)
					collected.Write(buf[:n])
					if !signaled && strings.Contains(collected.String(), "PTY_READY") {
						signaled = true
						ready <- struct{}{}
					}
					if readErr != nil {
						output <- collected.String()
						return
					}
				}
			}()

			attrs, err := windows.NewProcThreadAttributeList(1)
			require.NoError(t, err)
			defer attrs.Delete()
			// This attribute takes the pseudoconsole handle value, not a pointer to it.
			update := windows.NewLazySystemDLL("kernel32.dll").NewProc("UpdateProcThreadAttribute")
			ok, _, callErr := update.Call(uintptr(unsafe.Pointer(attrs.List())), 0, windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE, uintptr(console), unsafe.Sizeof(console), 0, 0)
			require.NotZero(t, ok, "UpdateProcThreadAttribute: %v", callErr)
			exe, err := os.Executable()
			require.NoError(t, err)
			command, err := windows.UTF16PtrFromString(windows.ComposeCommandLine([]string{exe, "-test.run=^TestConsoleConPTYChild$", "-test.v", "-test.timeout=25s"}))
			require.NoError(t, err)
			startup := windows.StartupInfoEx{StartupInfo: windows.StartupInfo{Cb: uint32(unsafe.Sizeof(windows.StartupInfoEx{}))}, ProcThreadAttributeList: attrs.List()}
			var process windows.ProcessInformation
			require.NoError(t, windows.CreateProcess(nil, command, nil, nil, false, windows.EXTENDED_STARTUPINFO_PRESENT, nil, nil, &startup.StartupInfo, &process))
			defer windows.CloseHandle(process.Thread)
			defer windows.CloseHandle(process.Process)
			defer windows.TerminateProcess(process.Process, 1)
			exited := make(chan uint32, 1)
			go func() { result, _ := windows.WaitForSingleObject(process.Process, 30000); exited <- result }()
			select {
			case <-ready:
				_, err = io.WriteString(inputW, "input-proof\r")
				require.NoError(t, err)
			case <-exited:
				windows.ClosePseudoConsole(console)
				console = 0
				t.Fatalf("child exited before requesting input: %s", <-output)
			case <-time.After(20 * time.Second):
				t.Fatal("child did not request terminal input")
			}
			require.Equal(t, uint32(windows.WAIT_OBJECT_0), <-exited)
			var exit uint32
			require.NoError(t, windows.GetExitCodeProcess(process.Process, &exit))
			windows.ClosePseudoConsole(console)
			console = 0
			transcript := <-output
			require.Zero(t, exit, "%s", transcript)
			require.Contains(t, transcript, "REMOTE_INPUT_OK")
			require.Contains(t, transcript, "REMOTE_STDERR_OK")
			require.Contains(t, transcript, "CONSOLE_MODES_RESTORED")
		})
	}
}

func TestConsoleConPTYChild(t *testing.T) {
	statusText, child := os.LookupEnv("FLYCTL_CONPTY_PROOF_CHILD")
	if !child {
		return
	}
	status, err := strconv.Atoi(statusText)
	require.NoError(t, err)
	// CI's inherited standard handles are pipes. Open this child's attached
	// pseudoconsole explicitly before exercising the console path.
	for i, device := range []string{"CONIN$", "CONOUT$", "CONOUT$"} {
		name, err := windows.UTF16PtrFromString(device)
		require.NoError(t, err)
		h, err := windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE,
			windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
		require.NoError(t, err)
		f := os.NewFile(uintptr(h), device)
		defer f.Close()
		switch i {
		case 0:
			os.Stdin = f
			windows.Stdin = h
			require.NoError(t, windows.SetStdHandle(windows.STD_INPUT_HANDLE, h))
		case 1:
			os.Stdout = f
			windows.Stdout = h
			require.NoError(t, windows.SetStdHandle(windows.STD_OUTPUT_HANDLE, h))
		case 2:
			os.Stderr = f
			windows.Stderr = h
			require.NoError(t, windows.SetStdHandle(windows.STD_ERROR_HANDLE, h))
		}
	}
	handles := []windows.Handle{windows.Handle(os.Stdin.Fd()), windows.Handle(os.Stdout.Fd()), windows.Handle(os.Stderr.Fd())}
	// Force setup to change modes, rather than merely restore already-enabled flags.
	for i, h := range handles {
		var mode uint32
		require.NoError(t, windows.GetConsoleMode(h, &mode))
		flag := uint32(windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING)
		if i == 0 {
			flag = windows.ENABLE_VIRTUAL_TERMINAL_INPUT
		}
		require.NoError(t, windows.SetConsoleMode(h, mode&^flag))
	}
	before := make([]uint32, len(handles))
	for i, h := range handles {
		require.NoError(t, windows.GetConsoleMode(h, &before[i]))
	}
	client := conptySSHServer(t, uint32(status))
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	err = Console(ctx, &flyssh.Client{Client: client}, "smoke", true, SessionTarget{})
	if status == 0 {
		require.NoError(t, err)
	} else {
		var exit *gossh.ExitError
		require.ErrorAs(t, err, &exit)
		require.Equal(t, status, exit.ExitStatus())
	}
	for i, h := range handles {
		var after uint32
		require.NoError(t, windows.GetConsoleMode(h, &after))
		require.Equal(t, before[i], after, "console handle %d was not restored", i)
	}
	fmt.Println("CONSOLE_MODES_RESTORED")
}

// The loopback fixture accepts only a fixed command; it never invokes a shell.
func conptySSHServer(t *testing.T, status uint32) *gossh.Client {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	signer, err := gossh.NewSignerFromKey(key)
	require.NoError(t, err)
	config := &gossh.ServerConfig{NoClientAuth: true}
	config.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	done := make(chan error, 1)
	go func() {
		done <- func() error {
			conn, err := listener.Accept()
			if err != nil {
				return err
			}
			defer conn.Close()
			_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
			server, channels, requests, err := gossh.NewServerConn(conn, config)
			if err != nil {
				return err
			}
			defer server.Close()
			go gossh.DiscardRequests(requests)
			channel, ok := <-channels
			if !ok {
				return fmt.Errorf("session not opened")
			}
			ch, reqs, err := channel.Accept()
			if err != nil {
				return err
			}
			defer ch.Close()
			pty := false
			for req := range reqs {
				if req.Type == "pty-req" {
					pty = true
					_ = req.Reply(true, nil)
					continue
				}
				if req.Type != "exec" {
					_ = req.Reply(false, nil)
					continue
				}
				var command struct{ Command string }
				if err := gossh.Unmarshal(req.Payload, &command); err != nil {
					return err
				}
				if command.Command != "smoke" {
					return fmt.Errorf("unexpected command")
				}
				if err := req.Reply(true, nil); err != nil {
					return err
				}
				if !pty {
					return fmt.Errorf("PTY was not requested")
				}
				if _, err := io.WriteString(ch, "PTY_READY\r\n"); err != nil {
					return err
				}
				input := make([]byte, len("input-proof\r"))
				if _, err := io.ReadFull(ch, input); err != nil {
					return err
				}
				if string(input) != "input-proof\r" {
					return fmt.Errorf("unexpected terminal input: %q", input)
				}
				if _, err := io.WriteString(ch, "REMOTE_INPUT_OK\r\n"); err != nil {
					return err
				}
				if _, err := io.WriteString(ch.Stderr(), "REMOTE_STDERR_OK\r\n"); err != nil {
					return err
				}

				_, err = ch.SendRequest("exit-status", false, gossh.Marshal(struct{ Status uint32 }{status}))

				return err
			}

			return fmt.Errorf("exec not received")
		}()
	}()
	client, err := gossh.Dial("tcp", listener.Addr().String(), &gossh.ClientConfig{
		User: "test", HostKeyCallback: gossh.FixedHostKey(signer.PublicKey()), Timeout: 10 * time.Second,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = client.Close()
		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(12 * time.Second):
			t.Error("SSH fixture did not stop")
		}
	})

	return client
}
