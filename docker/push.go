package docker

import (
	"bufio"
	"encoding/json"
	"io"

	"github.com/docker/docker/api/types"
)

type PushOperation struct {
	error  error
	stream chan PushMessage
}

func (op *PushOperation) Error() error {
	return op.error
}

func (op *PushOperation) Stream() <-chan PushMessage {
	return op.stream
}

func (c *DockerClient) PushImage(imageName string) *PushOperation {
	stream := make(chan PushMessage, 0)

	op := &PushOperation{
		stream: stream,
	}

	go func() {
		defer close(stream)

		out, err := c.docker.ImagePush(c.ctx, imageName, types.ImagePushOptions{RegistryAuth: c.registryAuth})
		if err != nil {
			op.error = err
			return
		}
		defer out.Close()

		if err := processPushMessages(out, stream); err != nil {
			op.error = err
			return
		}
	}()

	return op
}

type pushResp struct {
	Status         string
	ID             string
	Progress       string
	ProgressDetail struct {
		Current int
		Total   int
	}
	Aux struct {
		Tag    string
		Digest string
	}
}

type layerState struct {
	bytesTotal    int
	bytesComplete int
	complete      bool
}

type PushMessage struct {
	LayersTotal    int
	LayersComplete int
	Progress       int
}

func processPushMessages(reader io.Reader, stream chan<- PushMessage) error {
	respBuf := bufio.NewReader(reader)

	layers := make(map[string]layerState)
	finishedLayers := make(map[string]bool)

	var resp pushResp
	for {
		line, _, err := respBuf.ReadLine()

		if len(line) > 0 {
			if err := json.Unmarshal(line, &resp); err != nil {
				return err
			}

			if resp.ID != "" {
				layers[resp.ID] = layerState{
					bytesTotal:    resp.ProgressDetail.Total,
					bytesComplete: resp.ProgressDetail.Current,
				}

				if isLaterComplete(resp.Status) {
					finishedLayers[resp.ID] = true
				}
			}

			msg := &PushMessage{
				LayersTotal:    len(layers),
				LayersComplete: len(finishedLayers),
			}

			total := len(layers)
			complete := len(finishedLayers)

			for _, layer := range layers {
				total += layer.bytesTotal
				complete += layer.bytesComplete
			}

			if total > 0 {
				msg.Progress = int(float64(complete) / float64(total) * 100)
				if msg.Progress < 0 {
					msg.Progress = 0
				}
				if msg.Progress > 100 {
					msg.Progress = 100
				}
			}

			stream <- *msg
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

func isLaterComplete(status string) bool {
	switch status {
	case "Layer already exists", "Pushed":
		return true
	}
	return false
}
