package imgsrc

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	depotmachine "github.com/depot/depot-go/machine"
	control "github.com/moby/buildkit/api/services/control"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	cliv1 "github.com/depot/depot-go/proto/depot/cli/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go/tokens"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"google.golang.org/protobuf/proto"
)

func TestDepotAcquisitionPrivacy(t *testing.T) {
	for _, name := range []string{"82d1104c0034d8", "", "82D1104C0034D8", "82d1104c0034d8.example", "82d1104c0034d8\n", "user:secret@host", "abcdef0123456", "abcdef012345678", "zzd1104c0034d8"} {
		t.Run(name, func(t *testing.T) {
			recorder := tracetest.NewSpanRecorder()
			provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
			defer provider.Shutdown(context.Background())
			_, span := provider.Tracer("test").Start(context.Background(), "depot_build")
			meta := &uiex.BuilderMeta{RemoteMachineId: "unchanged-mirror"}
			recordDepotAcquisition(meta, span, name)
			span.End()
			require.Equal(t, "unchanged-mirror", meta.RemoteMachineId)
			attrs := map[string]string{}
			for _, a := range recorder.Ended()[0].Attributes() {
				attrs[string(a.Key)] = a.Value.AsString()
			}
			if name == "82d1104c0034d8" {
				require.Equal(t, name, meta.DepotAcquisitionTarget)
				require.Equal(t, name, attrs["depot_acquisition_target"])
			} else {
				require.Empty(t, meta.DepotAcquisitionTarget)
				require.NotContains(t, attrs, "depot_acquisition_target")
			}
		})
	}
}

// Drive the real SDK and serialize the resulting metadata through the real web
// FinishBuild client. Only provisioning and the two loopback servers are faked.
func TestDepotFailureReporting(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	certPEM, keyPEM := depotTestCert(t)
	for _, scenario := range []string{"ensure_error", "acquire_timeout", "tls_eof_timeout", "stalled_timeout", "stalled_handshake_30s", "cancel_handshake", "cancel_after_tls_eof"} {
		t.Run(scenario, func(t *testing.T) {
			budget := 800 * time.Millisecond
			if scenario == "stalled_handshake_30s" {
				if testing.Short() {
					t.Skip("requires the real gRPC handshake timeout")
				}
				// Exceed gRPC's 20-second attempt timeout so the SDK exposes
				// the TLS failure instead of only the outer context deadline.
				budget = 30 * time.Second
			}
			ctx := config.NewContext(context.Background(), &config.Config{Tokens: &tokens.Tokens{}})
			ctx, cancel := context.WithTimeout(ctx, budget)
			defer cancel()
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			defer listener.Close()
			done := make(chan struct{})
			canceled := ctx.Done()
			go func() {
				defer close(done)
				for {
					conn, err := listener.Accept()
					if err != nil {
						return
					}
					_ = conn.SetReadDeadline(time.Now().Add(time.Second))
					var hello [4096]byte
					_, _ = conn.Read(hello[:])
					if scenario == "tls_eof_timeout" || scenario == "cancel_after_tls_eof" {
						conn.Close()
						if scenario == "cancel_after_tls_eof" {
							time.AfterFunc(100*time.Millisecond, cancel)
						}
					} else {
						if scenario == "cancel_handshake" {
							cancel()
						}
						<-canceled
						conn.Close()
					}
				}
			}()
			defer func() { listener.Close(); <-done }()
			active, err := proto.Marshal(&cliv1.GetBuildKitConnectionResponse{Connection: &cliv1.GetBuildKitConnectionResponse_Active{Active: &cliv1.GetBuildKitConnectionResponse_ActiveConnection{Endpoint: "tcp://" + listener.Addr().String(), ServerName: "82d1104c0034d8", Cert: &cliv1.Cert{Cert: certPEM, Key: keyPEM}, CaCert: &cliv1.Cert{Cert: certPEM}}}})
			require.NoError(t, err)
			if scenario == "acquire_timeout" {
				active, err = proto.Marshal(&cliv1.GetBuildKitConnectionResponse{Connection: &cliv1.GetBuildKitConnectionResponse_Pending{Pending: &cliv1.GetBuildKitConnectionResponse_PendingConnection{}}})
				require.NoError(t, err)
			}
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/proto")
				switch {
				case strings.HasSuffix(r.URL.Path, "/GetBuildKitConnection"):
					w.Write(active)
				case strings.HasSuffix(r.URL.Path, "/ReportBuildHealth"), strings.HasSuffix(r.URL.Path, "/FinishBuild"):
					w.WriteHeader(http.StatusOK)
				default:
					t.Errorf("unexpected SDK path %s", r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			defer api.Close()
			t.Setenv("DEPOT_API_URL", api.URL)
			ctx = uiexutil.NewContextWithClient(ctx, &mock.UiexClient{EnsureDepotBuilderFunc: func(context.Context, uiex.EnsureDepotBuilderRequest) (*uiex.EnsureDepotBuilderResponse, error) {
				if scenario == "ensure_error" {
					return nil, errors.New("ensure sentinel")
				}
				id, token := "local-build", "local-token"
				return &uiex.EnsureDepotBuilderResponse{BuildId: &id, BuildToken: &token}, nil
			}})
			ios, _, _, _ := iostreams.Test()
			b := newBuild(42, false)
			_, buildErr := depotBuild(ctx, ios, ImageOptions{AppName: "test"}, "", b, DepotBuilderScopeOrganization)
			require.Error(t, buildErr)
			if scenario != "ensure_error" && scenario != "acquire_timeout" {
				require.Contains(t, buildErr.Error(), "connecting to Depot builder:")
			}
			if strings.HasPrefix(scenario, "cancel_") {
				require.ErrorIs(t, buildErr, context.Canceled)
			} else if scenario != "ensure_error" && scenario != "acquire_timeout" {
				require.Contains(t, buildErr.Error(), "timed out connecting to machine")
			}
			if scenario == "stalled_handshake_30s" {
				require.ErrorIs(t, buildErr, context.DeadlineExceeded)
			}
			if scenario == "tls_eof_timeout" || scenario == "stalled_handshake_30s" {
				require.Contains(t, buildErr.Error(), "authentication handshake failed")
			}
			received := make(chan uiex.FinishBuildRequest, 1)
			rawReceived := make(chan []byte, 1)
			web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/api/v1/builds/finish", r.URL.Path)
				raw, err := io.ReadAll(r.Body)
				if !assert.NoError(t, err) {
					w.WriteHeader(400)
					return
				}
				if scenario == "ensure_error" || scenario == "acquire_timeout" {
					assert.NotContains(t, string(raw), "depot_acquisition_target")
				}
				var request uiex.FinishBuildRequest
				if !assert.NoError(t, json.Unmarshal(raw, &request)) {
					w.WriteHeader(400)
					return
				}
				rawReceived <- raw
				received <- request
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"id":42,"status":"failed"}`))
			}))
			defer web.Close()
			base, _ := url.Parse(web.URL)
			webClient, err := uiex.NewWithOptions(ctx, uiex.NewClientOpts{BaseURL: base})
			require.NoError(t, err)
			ctx = uiexutil.NewContextWithClient(ctx, webClient)
			resolver := &Resolver{appName: "test", dockerFactory: &dockerClientFactory{appName: "test"}}
			_, err = resolver.finishBuild(ctx, b, true, buildErr.Error(), nil)
			require.NoError(t, err)
			request := <-received
			require.Equal(t, "failed", request.Status)
			require.Equal(t, buildErr.Error(), request.Logs)
			require.Empty(t, request.BuilderMeta.RemoteMachineId)
			if scenario == "ensure_error" || scenario == "acquire_timeout" {
				require.Empty(t, request.BuilderMeta.DepotAcquisitionTarget)
			} else {
				require.Equal(t, "82d1104c0034d8", request.BuilderMeta.DepotAcquisitionTarget)
			}
			body := <-rawReceived
			for _, secret := range []string{"BEGIN CERTIFICATE", "PRIVATE KEY", "local-token", "depot_transport_security"} {
				require.NotContains(t, string(body), secret)
			}
			stage, outcome := "connecting", "deadline_exceeded"
			if scenario == "acquire_timeout" {
				stage = "acquire"
			}
			if scenario == "ensure_error" {
				stage, outcome = "ensure_builder", "error"
			}
			if scenario == "tls_eof_timeout" || scenario == "cancel_after_tls_eof" || scenario == "stalled_handshake_30s" {
				stage = "tls_handshake"
			}
			if strings.HasPrefix(scenario, "cancel_") {
				outcome = "canceled"
			}
			require.Equal(t, stage, request.BuilderMeta.DepotConnectionStage)
			require.Equal(t, outcome, request.BuilderMeta.DepotConnectionOutcome)
			t.Logf("REPORTED scenario=%s body=%s", scenario, body)
		})
	}
}

// Cancellation before reporting is covered above; exercise cancellation in flight.
func TestDepotFinishCancellationInFlight(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	received := make(chan uiex.FinishBuildRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/builds/finish", r.URL.Path)
		var body uiex.FinishBuildRequest
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&body)) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		cancel()
		received <- body
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":42,"status":"failed"}`))
	}))
	defer server.Close()
	base, _ := url.Parse(server.URL)
	ctx = config.NewContext(ctx, &config.Config{Tokens: &tokens.Tokens{}})
	client, err := uiex.NewWithOptions(ctx, uiex.NewClientOpts{BaseURL: base})
	require.NoError(t, err)
	ctx = uiexutil.NewContextWithClient(ctx, client)

	b := newBuild(42, false)
	b.BuilderMeta.BuilderType = string(depotBuilderType)
	b.BuilderMeta.DepotAcquisitionTarget = "82d1104c0034d8"
	r := &Resolver{appName: "test", dockerFactory: &dockerClientFactory{appName: "test"}}
	result, err := r.finishBuild(ctx, b, true, context.Canceled.Error(), nil)
	require.NoError(t, err)
	require.Equal(t, "failed", result.Status)
	require.ErrorIs(t, ctx.Err(), context.Canceled)
	body := <-received
	require.Equal(t, "82d1104c0034d8", body.BuilderMeta.DepotAcquisitionTarget)
	require.Empty(t, body.BuilderMeta.RemoteMachineId)
	require.Equal(t, "failed", body.Status)
	require.Equal(t, context.Canceled.Error(), body.Logs)
}

func TestDepotFinishDeadlineAndOtherBuilders(t *testing.T) {
	for _, depot := range []bool{false, true} {
		t.Run(map[bool]string{false: "other builder stays canceled", true: "depot notification bounded"}[depot], func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			b := newBuild(42, false)
			b.BuilderMeta.BuilderType = "remote"
			if depot {
				b.BuilderMeta.BuilderType = string(depotBuilderType)
			}
			called := 0
			ctx = uiexutil.NewContextWithClient(ctx, &mock.UiexClient{FinishBuildFunc: func(callCtx context.Context, in uiex.FinishBuildRequest) (*uiex.BuildResponse, error) {
				called++
				if depot {
					require.NoError(t, callCtx.Err())
					select {
					case <-callCtx.Done():
					case <-time.After(10 * time.Second):
						t.Fatal("canceled notification exceeded grace period")
					}

					return nil, callCtx.Err()
				}
				require.ErrorIs(t, callCtx.Err(), context.Canceled)

				return nil, callCtx.Err()
			}})
			r := &Resolver{appName: "test", dockerFactory: &dockerClientFactory{appName: "test"}}
			_, err := r.finishBuild(ctx, b, true, "canceled", nil)
			require.ErrorIs(t, err, context.Canceled)
			require.Equal(t, 1, called)
			require.ErrorIs(t, ctx.Err(), context.Canceled)
		})
	}
}

// A slow response must retain the existing HTTP-client budget until cancellation.
func TestDepotFinishLiveParent(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx, cleanup := depotFinishContext(parent, 10*time.Millisecond)
	defer cleanup()
	select {
	case <-ctx.Done():
		t.Fatal("live notification acquired a new timeout")
	case <-time.After(50 * time.Millisecond):
	}
	cancel()
	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("notification did not stop after parent cancellation")
	}
}

func depotTestCert(t *testing.T) (string, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "depot test"}, DNSNames: []string{"builder.test"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return string(certPEM), string(keyPEM)
}

// A retry uses the same SDK client; Release remains the sole connection owner.
func TestDepotSDKReadinessRetry(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	for _, stall := range []bool{false, true} {
		t.Run(map[bool]string{false: "retry_success", true: "deadline_after_error"}[stall], func(t *testing.T) {
			certPEM, keyPEM := depotTestCert(t)
			cert, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
			require.NoError(t, err)
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			server := grpc.NewServer(grpc.Creds(credentials.NewTLS(&tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12})))
			service := &depotRetryControl{stall: stall}
			control.RegisterControlServer(server, service)
			go server.Serve(listener)
			defer server.Stop()
			machine := &depotmachine.Machine{Addr: "tcp://" + listener.Addr().String(), ServerName: "builder.test", CACert: certPEM, Cert: certPEM, Key: keyPEM}
			// Machine.Release also owns an acquisition health reporter, so obtain it through
			// the SDK Acquire path rather than constructing a partially initialized Machine.
			active, err := proto.Marshal(&cliv1.GetBuildKitConnectionResponse{Connection: &cliv1.GetBuildKitConnectionResponse_Active{Active: &cliv1.GetBuildKitConnectionResponse_ActiveConnection{Endpoint: machine.Addr, ServerName: machine.ServerName, Cert: &cliv1.Cert{Cert: certPEM, Key: keyPEM}, CaCert: &cliv1.Cert{Cert: certPEM}}}})
			require.NoError(t, err)
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/proto")
				if strings.HasSuffix(r.URL.Path, "/GetBuildKitConnection") {
					w.Write(active)
				} else {
					w.WriteHeader(http.StatusOK)
				}
			}))
			defer api.Close()
			t.Setenv("DEPOT_API_URL", api.URL)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			machine, err = depotmachine.Acquire(ctx, "local-build", "local-token", "amd64")
			require.NoError(t, err)
			defer func() { require.NoError(t, machine.Release()) }()
			meta := &uiex.BuilderMeta{}
			c, err := connectDepotSDK(ctx, machine, meta)
			require.Equal(t, int32(2), service.calls.Load())
			if stall {
				require.ErrorIs(t, err, context.DeadlineExceeded)
				require.Contains(t, err.Error(), "not ready yet")
				require.Nil(t, c)
				require.Equal(t, "connecting", meta.DepotConnectionStage)
				require.Equal(t, "deadline_exceeded", meta.DepotConnectionOutcome)
			} else {
				require.NoError(t, err)
				require.NotNil(t, c)
				require.Equal(t, "ready", meta.DepotConnectionStage)
				require.Equal(t, "success", meta.DepotConnectionOutcome)
			}
		})
	}
}

type depotRetryControl struct {
	control.UnimplementedControlServer
	calls atomic.Int32
	stall bool
}

func (s *depotRetryControl) ListWorkers(ctx context.Context, _ *control.ListWorkersRequest) (*control.ListWorkersResponse, error) {
	if s.calls.Add(1) == 1 {
		return nil, status.Error(codes.Unavailable, "not ready yet")
	}
	if s.stall {
		<-ctx.Done()
		return nil, ctx.Err()
	}

	return &control.ListWorkersResponse{}, nil
}
