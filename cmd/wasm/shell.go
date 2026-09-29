//go:build js && wasm

package main

import (
	"context"
	"io"
	"sync"
	"syscall/js"

	"github.com/superfly/flyctl/ssh"
)

// termInput is the shell's stdin: JavaScript appends, the session's copy
// goroutine reads. Appending never blocks, which a callback from the event
// loop must not do, and bytes stay in order.
type termInput struct {
	mu     sync.Mutex
	buf    []byte
	notify chan struct{}
	closed bool
}

func newTermInput() *termInput {
	return &termInput{notify: make(chan struct{}, 1)}
}

func (in *termInput) Write(p []byte) {
	in.mu.Lock()
	in.buf = append(in.buf, p...)
	in.mu.Unlock()

	select {
	case in.notify <- struct{}{}:
	default:
	}
}

func (in *termInput) Close() {
	in.mu.Lock()
	in.closed = true
	in.mu.Unlock()

	select {
	case in.notify <- struct{}{}:
	default:
	}
}

func (in *termInput) Read(p []byte) (int, error) {
	for {
		in.mu.Lock()
		if len(in.buf) > 0 {
			n := copy(p, in.buf)
			in.buf = in.buf[n:]
			in.mu.Unlock()

			return n, nil
		}
		closed := in.closed
		in.mu.Unlock()

		if closed {
			return 0, io.EOF
		}

		<-in.notify
	}
}

// termOutput is the shell's stdout and stderr: each write becomes one
// onData(Uint8Array) call on the page.
type termOutput struct {
	onData js.Value
}

func (out termOutput) Write(p []byte) (int, error) {
	out.onData.Invoke(bytesToJS(p))

	return len(p), nil
}

func (termOutput) Close() error { return nil }

// shell implements Connection.shell(options: ShellOptions): Session.
//
// The session runs on its own goroutine; the returned object's write,
// resize and close methods return immediately.
func (c *connection) shell(_ js.Value, args []js.Value) any {
	if len(args) < 1 || args[0].Type() != js.TypeObject {
		panic("shell: options object required")
	}
	opts := args[0]

	onData := optFunc(opts, "onData")
	if onData.IsUndefined() {
		panic("shell: onData callback required")
	}
	onExit := optFunc(opts, "onExit")

	ctx, cancel := context.WithCancel(c.ctx)

	var sizeMu sync.Mutex
	size := ssh.WindowSize{Width: optInt(opts, "cols", ssh.DefaultWidth), Height: optInt(opts, "rows", ssh.DefaultHeight)}
	resizes := make(chan ssh.WindowSize, 1)

	stdin := newTermInput()
	out := termOutput{onData: onData}

	sessIO := &ssh.SessionIO{
		Stdin:    stdin,
		Stdout:   out,
		Stderr:   out,
		AllocPTY: true,
		TermEnv:  optString(opts, "term", "xterm-256color"),
		Window: func() ssh.WindowSize {
			sizeMu.Lock()
			defer sizeMu.Unlock()

			return size
		},
		Resizes: resizes,
	}

	go func() {
		err := c.client.Shell(ctx, sessIO, optString(opts, "command", ""), c.target)
		cancel()

		if onExit.IsUndefined() {
			return
		}

		if err != nil {
			onExit.Invoke(js.ValueOf(err.Error()))
		} else {
			onExit.Invoke(js.Null())
		}
	}()

	return map[string]any{
		"write": js.FuncOf(func(_ js.Value, args []js.Value) any {
			if len(args) < 1 {
				return nil
			}
			b, err := bytesFromJS(args[0])
			if err != nil {
				panic("write: " + err.Error())
			}
			stdin.Write(b)

			return nil
		}),
		"resize": js.FuncOf(func(_ js.Value, args []js.Value) any {
			if len(args) < 2 {
				return nil
			}
			s := ssh.WindowSize{Width: args[0].Int(), Height: args[1].Int()}

			sizeMu.Lock()
			size = s
			sizeMu.Unlock()

			// latest size wins; an unread older one is superseded
			select {
			case <-resizes:
			default:
			}
			resizes <- s

			return nil
		}),
		"close": js.FuncOf(func(_ js.Value, _ []js.Value) any {
			stdin.Close()
			cancel()

			return nil
		}),
	}
}
