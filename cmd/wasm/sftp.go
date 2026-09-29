//go:build js && wasm

package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"sync"
	"syscall/js"

	"github.com/pkg/sftp"
)

const sftpChunk = 32 * 1024

func fileEntry(info fs.FileInfo, name string) map[string]any {
	mode := info.Mode()

	return map[string]any{
		"name":      name,
		"size":      info.Size(),
		"mode":      int(mode.Perm()),
		"modeText":  mode.String(),
		"mtime":     info.ModTime().UnixMilli(),
		"isDir":     info.IsDir(),
		"isSymlink": mode&fs.ModeSymlink != 0,
	}
}

func fileMode(opts js.Value, key string, def fs.FileMode) fs.FileMode {
	v := opts.Get(key)
	if v.Type() != js.TypeNumber {
		return def
	}

	return fs.FileMode(v.Int()) & fs.ModePerm
}

// sftp implements Connection.sftp(): Promise<SftpClient>.
func (c *connection) sftp(_ js.Value, _ []js.Value) any {
	return promise(func() (any, error) {
		client, err := c.client.SFTP(c.ctx, c.target, sftp.UseConcurrentReads(true), sftp.UseConcurrentWrites(true))
		if err != nil {
			return nil, err
		}

		s := &sftpClient{client: client}

		return map[string]any{
			"list":      js.FuncOf(s.list),
			"stat":      js.FuncOf(s.stat),
			"realpath":  js.FuncOf(s.realpath),
			"mkdir":     js.FuncOf(s.mkdir),
			"rename":    js.FuncOf(s.rename),
			"remove":    js.FuncOf(s.remove),
			"chmod":     js.FuncOf(s.chmod),
			"readFile":  js.FuncOf(s.readFile),
			"writeFile": js.FuncOf(s.writeFile),
			"close": js.FuncOf(func(_ js.Value, _ []js.Value) any {
				return promise(func() (any, error) { return nil, client.Close() })
			}),
		}, nil
	})
}

type sftpClient struct {
	client *sftp.Client
}

func pathArg(args []js.Value, i int) (string, error) {
	if len(args) <= i || args[i].Type() != js.TypeString {
		return "", fmt.Errorf("argument %d must be a path string", i+1)
	}

	return args[i].String(), nil
}

// list implements SftpClient.list(path): Promise<FileEntry[]>.
func (s *sftpClient) list(_ js.Value, args []js.Value) any {
	return promise(func() (any, error) {
		p, err := pathArg(args, 0)
		if err != nil {
			return nil, err
		}

		infos, err := s.client.ReadDir(p)
		if err != nil {
			return nil, err
		}

		entries := make([]any, 0, len(infos))
		for _, info := range infos {
			entries = append(entries, fileEntry(info, info.Name()))
		}

		return entries, nil
	})
}

// stat implements SftpClient.stat(path): Promise<FileEntry>. It doesn't
// follow a final symlink, so a link shows as one.
func (s *sftpClient) stat(_ js.Value, args []js.Value) any {
	return promise(func() (any, error) {
		p, err := pathArg(args, 0)
		if err != nil {
			return nil, err
		}

		info, err := s.client.Lstat(p)
		if err != nil {
			return nil, err
		}

		return fileEntry(info, p), nil
	})
}

// realpath implements SftpClient.realpath(path): Promise<string>.
func (s *sftpClient) realpath(_ js.Value, args []js.Value) any {
	return promise(func() (any, error) {
		p, err := pathArg(args, 0)
		if err != nil {
			return nil, err
		}

		return s.client.RealPath(p)
	})
}

// mkdir implements SftpClient.mkdir(path): Promise<void>, creating parents
// as needed.
func (s *sftpClient) mkdir(_ js.Value, args []js.Value) any {
	return promise(func() (any, error) {
		p, err := pathArg(args, 0)
		if err != nil {
			return nil, err
		}

		return nil, s.client.MkdirAll(p)
	})
}

// rename implements SftpClient.rename(from, to): Promise<void>, replacing
// an existing destination.
func (s *sftpClient) rename(_ js.Value, args []js.Value) any {
	return promise(func() (any, error) {
		from, err := pathArg(args, 0)
		if err != nil {
			return nil, err
		}
		to, err := pathArg(args, 1)
		if err != nil {
			return nil, err
		}

		return nil, s.client.PosixRename(from, to)
	})
}

// remove implements SftpClient.remove(path, options?: RemoveOptions):
// Promise<void>.
func (s *sftpClient) remove(_ js.Value, args []js.Value) any {
	return promise(func() (any, error) {
		p, err := pathArg(args, 0)
		if err != nil {
			return nil, err
		}

		recursive := len(args) > 1 && args[1].Type() == js.TypeObject && optBool(args[1], "recursive")

		info, err := s.client.Lstat(p)
		if err != nil {
			return nil, err
		}

		switch {
		case !info.IsDir():
			return nil, s.client.Remove(p)
		case recursive:
			return nil, s.client.RemoveAll(p)
		default:
			return nil, s.client.RemoveDirectory(p)
		}
	})
}

// chmod implements SftpClient.chmod(path, mode): Promise<void>.
func (s *sftpClient) chmod(_ js.Value, args []js.Value) any {
	return promise(func() (any, error) {
		p, err := pathArg(args, 0)
		if err != nil {
			return nil, err
		}
		if len(args) < 2 || args[1].Type() != js.TypeNumber {
			return nil, errors.New("mode must be a number")
		}

		return nil, s.client.Chmod(p, fs.FileMode(args[1].Int())&fs.ModePerm)
	})
}

// readFile implements both SftpClient.readFile overloads: with onChunk,
// each 32 KiB chunk is handed to it (awaiting any Promise it returns) and
// the result is the byte count; without, the whole file comes back as one
// Uint8Array, so keep that to small files.
func (s *sftpClient) readFile(_ js.Value, args []js.Value) any {
	return promise(func() (any, error) {
		p, err := pathArg(args, 0)
		if err != nil {
			return nil, err
		}

		onChunk := js.Undefined()
		if len(args) > 1 && args[1].Type() == js.TypeFunction {
			onChunk = args[1]
		}

		f, err := s.client.Open(p)
		if err != nil {
			return nil, err
		}
		defer f.Close()

		if onChunk.IsUndefined() {
			data, err := io.ReadAll(f)
			if err != nil {
				return nil, err
			}

			return bytesToJS(data), nil
		}

		buf := make([]byte, sftpChunk)
		var total int64
		for {
			n, err := f.Read(buf)
			if n > 0 {
				total += int64(n)
				if _, err := await(onChunk.Invoke(bytesToJS(buf[:n]))); err != nil {
					return nil, fmt.Errorf("chunk callback: %w", err)
				}
			}
			if err == io.EOF {
				return total, nil
			}
			if err != nil {
				return nil, err
			}
		}
	})
}

// writeFile implements SftpClient.writeFile(path, options?: WriteOptions):
// Promise<FileWriter>. The file is created exclusively unless overwrite is
// set, matching what `fly ssh sftp put` does, and chmod'ed to mode
// (default 0644) when the writer is closed.
func (s *sftpClient) writeFile(_ js.Value, args []js.Value) any {
	return promise(func() (any, error) {
		p, err := pathArg(args, 0)
		if err != nil {
			return nil, err
		}

		opts := js.Undefined()
		if len(args) > 1 && args[1].Type() == js.TypeObject {
			opts = args[1]
		}

		flags := os.O_WRONLY | os.O_CREATE | os.O_EXCL
		mode := fs.FileMode(0o644)
		if !opts.IsUndefined() {
			if optBool(opts, "overwrite") {
				flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
			}
			mode = fileMode(opts, "mode", mode)
		}

		f, err := s.client.OpenFile(p, flags)
		if err != nil {
			return nil, err
		}

		var mu sync.Mutex // writes are sequential; the mutex keeps them so

		return map[string]any{
			"write": js.FuncOf(func(_ js.Value, args []js.Value) any {
				return promise(func() (any, error) {
					if len(args) < 1 {
						return nil, errors.New("write: data required")
					}
					b, err := bytesFromJS(args[0])
					if err != nil {
						return nil, err
					}

					mu.Lock()
					defer mu.Unlock()

					n, err := f.Write(b)

					return n, err
				})
			}),
			"close": js.FuncOf(func(_ js.Value, _ []js.Value) any {
				return promise(func() (any, error) {
					mu.Lock()
					defer mu.Unlock()

					if err := f.Close(); err != nil {
						return nil, err
					}

					return nil, s.client.Chmod(p, mode)
				})
			}),
		}, nil
	})
}
