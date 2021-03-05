package docker

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/docker/docker/pkg/jsonmessage"
	controlapi "github.com/moby/buildkit/api/services/control"
	buildkitClient "github.com/moby/buildkit/client"
	"github.com/moby/buildkit/session"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"github.com/superfly/flyctl/flyctl"
	"golang.org/x/net/context"
)

func createBuildSession(contextDir string) (*session.Session, error) {
	sharedKey := getBuildSharedKey(contextDir)
	s, err := session.NewSession(context.Background(), filepath.Base(contextDir), sharedKey)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create session")
	}
	return s, nil
}

func getBuildSharedKey(dir string) string {
	// build session is hash of build dir with node based randomness
	s := sha256.Sum256([]byte(fmt.Sprintf("%s:%s", getBuildNodeID(), dir)))
	return hex.EncodeToString(s[:])
}

func getBuildNodeID() string {
	buildNodeID := viper.GetString(flyctl.BuildKitNodeID)
	if buildNodeID == "" {
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			return flyctl.ConfigDir()
		}

		buildNodeID = hex.EncodeToString(b)
		viper.Set(flyctl.BuildKitNodeID, buildNodeID)
		if err := flyctl.SaveConfig(); err != nil {
			fmt.Println("error writing config", err)
			return flyctl.ConfigDir()
		}
	}
	return buildNodeID
}

type tracer struct {
	displayCh chan *buildkitClient.SolveStatus
}

func newTracer() *tracer {
	return &tracer{
		displayCh: make(chan *buildkitClient.SolveStatus),
	}
}

func (t *tracer) write(msg jsonmessage.JSONMessage) {
	var resp controlapi.StatusResponse

	if msg.ID != "moby.buildkit.trace" {
		return
	}

	var dt []byte
	// ignoring all messages that are not understood
	if err := json.Unmarshal(*msg.Aux, &dt); err != nil {
		return
	}
	if err := (&resp).Unmarshal(dt); err != nil {
		return
	}

	s := buildkitClient.SolveStatus{}
	for _, v := range resp.Vertexes {
		s.Vertexes = append(s.Vertexes, &buildkitClient.Vertex{
			Digest:    v.Digest,
			Inputs:    v.Inputs,
			Name:      v.Name,
			Started:   v.Started,
			Completed: v.Completed,
			Error:     v.Error,
			Cached:    v.Cached,
		})
	}
	for _, v := range resp.Statuses {
		s.Statuses = append(s.Statuses, &buildkitClient.VertexStatus{
			ID:        v.ID,
			Vertex:    v.Vertex,
			Name:      v.Name,
			Total:     v.Total,
			Current:   v.Current,
			Timestamp: v.Timestamp,
			Started:   v.Started,
			Completed: v.Completed,
		})
	}
	for _, v := range resp.Logs {
		s.Logs = append(s.Logs, &buildkitClient.VertexLog{
			Vertex:    v.Vertex,
			Stream:    int(v.Stream),
			Data:      v.Msg,
			Timestamp: v.Timestamp,
		})
	}

	t.displayCh <- &s
}
