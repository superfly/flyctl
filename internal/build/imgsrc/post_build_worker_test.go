package imgsrc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/containerd/containerd/api/services/content/v1"
	depotmachine "github.com/depot/depot-go/machine"
	control "github.com/moby/buildkit/api/services/control"
	types "github.com/moby/buildkit/api/types"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/worker/label"
	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/internal/uiex"
	"google.golang.org/grpc"
)

type namedDepotWorker struct {
	control.UnimplementedControlServer
}

func (*namedDepotWorker) ListWorkers(context.Context, *control.ListWorkersRequest) (*control.ListWorkersResponse, error) {
	return &control.ListWorkersResponse{Record: []*types.WorkerRecord{{Labels: map[string]string{label.Hostname: "test-builder"}}}}, nil
}

type manifestContent struct {
	content.UnimplementedContentServer
}

func (*manifestContent) Read(_ *content.ReadContentRequest, stream content.Content_ReadServer) error {
	return stream.Send(&content.ReadContentResponse{Data: []byte(`{"config":{"size":41}}`)})
}

func TestDeploymentImageAfterWorkerDisconnect(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	server := grpc.NewServer()
	control.RegisterControlServer(server, &namedDepotWorker{})
	content.RegisterContentServer(server, &manifestContent{})
	go server.Serve(listener)
	t.Cleanup(server.Stop)
	meta := &uiex.BuilderMeta{}
	c, readyHostname, err := connectDepotSDK(ctx, &depotmachine.Machine{Addr: "tcp://" + listener.Addr().String()}, meta)
	require.NoError(t, err)
	defer c.Close()
	require.Empty(t, meta.RemoteMachineId)
	digest := "sha256:" + strings.Repeat("a", 64)
	descriptor := Descriptor{Digest: digest, Size: 35, Annotations: Annotations{RawManifest: "{}"}}
	raw, err := json.Marshal(descriptor)
	require.NoError(t, err)
	receipt := &client.SolveResponse{ExporterResponse: map[string]string{"containerimage.digest": digest, "containerimage.descriptor": base64.StdEncoding.EncodeToString(raw)}}
	plainDescriptor, err := json.Marshal(Descriptor{Digest: digest, Size: 35})
	require.NoError(t, err)
	plain := &client.SolveResponse{ExporterResponse: map[string]string{"containerimage.digest": digest, "containerimage.descriptor": base64.StdEncoding.EncodeToString(plainDescriptor)}}
	plainImage, err := newDeploymentImage(ctx, c, plain, "plain-tag", "")
	require.NoError(t, err)
	require.EqualValues(t, 76, plainImage.Size)
	hostname, err := workerHostname(ctx, c)
	require.NoError(t, err)
	require.Equal(t, "test-builder", hostname)
	server.Stop()
	image, err := newDeploymentImage(ctx, c, receipt, "registry.example/app:deploy", readyHostname)
	require.NoError(t, err)
	require.Equal(t, digest, image.ID)
	require.Equal(t, "registry.example/app:deploy", image.Tag)
	require.Equal(t, "test-builder", image.BuilderID)
	_, err = newDeploymentImage(ctx, c, plain, "plain-tag", readyHostname)
	require.ErrorContains(t, err, "read")
}
