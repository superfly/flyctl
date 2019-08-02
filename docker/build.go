package docker

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"

	"github.com/docker/docker/api/types"
	"github.com/mholt/archiver"
	"github.com/superfly/flyctl/terminal"
)

func (c *DockerClient) BuildImage(path, tag string) *BuildOperation {
	stream := make(chan BuildMessage, 0)

	op := &BuildOperation{
		status: stream,
	}

	tarReader, tarWriter := io.Pipe()

	go func() {
		defer tarWriter.Close()

		if err := tarBuildContext(tarWriter, path); err != nil {
			// if error occures here log and bail, the build will fail when the stream is closed
			terminal.Error(err)
			return
		}
	}()

	go func() {
		defer close(op.status)

		resp, err := c.docker.ImageBuild(c.ctx, tarReader, types.ImageBuildOptions{
			Tags: []string{tag},
		})
		if err != nil {
			op.error = err
			return
		}
		defer resp.Body.Close()

		if err := processBuildMessages(resp.Body, stream); err != nil {
			op.error = err
			return
		}
	}()

	return op
}

type BuildMessage struct {
	Stream      string
	Error       string
	ErrorDetail struct {
		Code    int
		Message string
	}
}

type BuildOperation struct {
	error  error
	status chan BuildMessage
}

func (op *BuildOperation) Error() error {
	return op.error
}

func (op *BuildOperation) Status() <-chan BuildMessage {
	return op.status
}

func tarBuildContext(writer io.Writer, path string) error {
	tar := archiver.Tar{
		MkdirAll: true,
	}

	if err := tar.Create(writer); err != nil {
		return err
	}

	err := filepath.Walk(path, func(fpath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(path, fpath)
		if err != nil {
			return err
		}

		file, err := os.Open(fpath)
		if err != nil {
			return err
		}
		defer file.Close()

		err = tar.Write(archiver.File{
			FileInfo: archiver.FileInfo{
				FileInfo:   info,
				CustomName: relPath,
			},
			ReadCloser: file,
		})

		return err
	})

	return err
}

func processBuildMessages(reader io.Reader, stream chan<- BuildMessage) error {
	respBuf := bufio.NewReader(reader)

	var msg BuildMessage
	for {
		line, _, err := respBuf.ReadLine()

		if len(line) > 0 {
			if err := json.Unmarshal(line, &msg); err != nil {
				return err
			}

			stream <- msg
		}

		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
	}

	return nil
}
