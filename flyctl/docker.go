package flyctl

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	dockerparser "github.com/novln/docker-parser"
	"github.com/spf13/viper"
	"github.com/superfly/flyctl/terminal"
	"golang.org/x/net/context"
)

func NewDeploymentTag(appName string) string {
	t := time.Now()

	return fmt.Sprintf("registry.fly.io/%s:deployment-%d", appName, t.Unix())
}

type DockerClient struct {
	ctx          context.Context
	docker       *client.Client
	registryAuth string
}

func NewDockerClient() (*DockerClient, error) {
	cli, err := client.NewEnvClient()
	if err != nil {
		return nil, err
	}

	accessToken := viper.GetString(ConfigAPIAccessToken)

	authConfig := types.AuthConfig{
		Username: accessToken,
		Password: "x",
	}
	encodedJSON, err := json.Marshal(authConfig)
	if err != nil {
		return nil, err
	}
	authStr := base64.URLEncoding.EncodeToString(encodedJSON)

	c := &DockerClient{
		ctx:          context.Background(),
		docker:       cli,
		registryAuth: authStr,
	}

	return c, nil
}

type PushResp struct {
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

type PushProgress struct {
	BytesTotal    int
	BytesComplete int
	BytesProgress float64
	LayerTotal    int
	LayerComplete int
	LayerProgress float64
}

type PushOperation struct {
	error  error
	status chan PushProgress
}

func (op *PushOperation) Error() error {
	return op.error
}

func (op *PushOperation) Status() <-chan PushProgress {
	return op.status
}

// type Progress struct {
// 	Total     int
// 	Remaining int
// }

type layerStatus struct {
	bytesTotal    int
	bytesComplete int
	status        string
	complete      bool
}

func (c *DockerClient) PushImage(imageName string) *PushOperation {
	status := make(chan PushProgress, 0)

	op := &PushOperation{
		status: status,
	}

	go func() {
		out, err := c.docker.ImagePush(c.ctx, imageName, types.ImagePushOptions{RegistryAuth: c.registryAuth})
		if err != nil {
			op.error = err
		}
		read, write := io.Pipe()

		defer out.Close()
		// defer
		defer close(status)

		go func() {
			io.Copy(write, out)
			write.Close()
		}()

		readbuf := bufio.NewReader(read)
		layers := make(map[string]layerStatus)

		for {
			var resp PushResp
			// fmt.Println("readline1")
			line, _, err := readbuf.ReadLine()

			if len(line) > 0 {
				if err := json.Unmarshal(line, &resp); err != nil {
					op.error = err
					break
				}

				if resp.ID != "" {
					layers[resp.ID] = layerStatus{
						bytesTotal:    resp.ProgressDetail.Total,
						bytesComplete: resp.ProgressDetail.Current,
						status:        resp.Status,
						complete:      isLaterComplete(resp.Status),
					}
				}

				progress := &PushProgress{
					LayerTotal: len(layers),
				}

				for _, layer := range layers {
					progress.BytesTotal += layer.bytesTotal
					progress.BytesComplete += layer.bytesComplete
					if layer.complete {
						progress.LayerComplete++
					}
				}

				if progress.BytesTotal > 0 {
					progress.BytesProgress = float64(progress.BytesComplete / progress.BytesTotal)
				}

				if progress.LayerTotal > 0 {
					progress.LayerProgress = float64(progress.LayerComplete / progress.LayerTotal)
				}

				status <- *progress
			}

			if err != nil {
				if err == io.EOF {
					break
				}
				op.error = err
				break
			}
		}

		//

		// for {
		// 	n, err := out.Read(p)

		// 	if err == nil {
		// 		terminal.Debug(string(p[:n]))

		// 		r := bytes.NewBuffer(p)

		// 		json.NewDecoder(strings.NewReader(string(p[:n]))).Decode(&resp)

		// 		// terminal.Debugf("%+v\n", resp)

		// 	}

		// 	if err != nil {
		// 		if err != io.EOF {
		// 			op.error = err
		// 		}
		// 		break
		// 	}

		// }

		// close(status)
	}()

	return op
}

func (c *DockerClient) FindImage(imageName string) (*types.ImageSummary, error) {
	ref, err := dockerparser.Parse(imageName)
	if err != nil {
		return nil, err
	}

	imageIDPattern := regexp.MustCompile("[a-f0-9]")

	isID := imageIDPattern.MatchString(imageName)

	images, err := c.docker.ImageList(c.ctx, types.ImageListOptions{})
	if err != nil {
		return nil, err
	}

	if isID {
		for _, img := range images {
			if img.ID[7:7+len(imageName)] == imageName {
				terminal.Debug("Found image by id", imageName)
				return &img, nil
			}
		}
	}

	searchTerms := []string{
		imageName,
		imageName + ":" + ref.Tag(),
		ref.Name(),
		ref.ShortName(),
		ref.Remote(),
		ref.Repository(),
	}

	terminal.Debug("Search terms:", searchTerms)

	for _, img := range images {
		for _, tag := range img.RepoTags {
			// skip <none>:<none>
			if strings.HasPrefix(tag, "<none>") {
				continue
			}

			for _, term := range searchTerms {
				if tag == term {
					return &img, nil
				}
			}
		}
	}

	return nil, nil
}

func (c *DockerClient) TagImage(sourceRef, tag string) error {
	return c.docker.ImageTag(c.ctx, sourceRef, tag)
}

func isLaterComplete(status string) bool {
	switch status {
	case "Layer already exists", "Pushed":
		return true
	}
	return false
}
