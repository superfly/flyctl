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

	depotmachine "github.com/depot/depot-go/machine"
	cliv1 "github.com/depot/depot-go/proto/depot/cli/v1"
	control "github.com/moby/buildkit/api/services/control"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go/tokens"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func TestDepotAcquisitionPrivacy(t *testing.T) {
	for _, name := range []string{"82d1104c0034d8", "", "82D1104C0034D8", "82d1104c0034d8.example", "82d1104c0034d8\n", "user:secret@host", "abcdef0123456", "abcdef012345678", "zzd1104c0034d8"} {
		t.Run(name, func(t *testing.T) {
			meta := &uiex.BuilderMeta{RemoteMachineId: "unchanged-mirror"}
			recordDepotAcquisition(meta, name)
			require.Equal(t, "unchanged-mirror", meta.RemoteMachineId)
			if name == "82d1104c0034d8" {
				require.Equal(t, name, meta.DepotAcquisitionTarget)
			} else {
				require.Empty(t, meta.DepotAcquisitionTarget)
			}
		})
	}
}

// Drive the real SDK against loopback fakes and serialize the resulting metadata
// through the real web FinishBuild client. Except for ensure_error, the build
// budget has already ended when finishBuild runs, so the detached result
// notification is exercised over real HTTP as well.
func TestDepotFailureReporting(t *testing.T) {
	certPEM, keyPEM := depotTestCert(t)
	for _, tc := range []struct {
		name, stage, outcome string
		target               bool // a builder was acquired, so its target is reported
		wantIs               error
		wantText             []string
		budget               time.Duration
	}{
		{"ensure_error", "ensure_builder", "error", false, nil, []string{"ensure sentinel"}, 0},
		{"acquire_timeout", "acquire", "deadline_exceeded", false, context.DeadlineExceeded, nil, 0},
		{"tls_eof_timeout", "tls_handshake", "deadline_exceeded", true, context.DeadlineExceeded, []string{"timed out connecting to machine", "authentication handshake failed"}, 0},
		{"stalled_timeout", "connecting", "deadline_exceeded", true, context.DeadlineExceeded, []string{"timed out connecting to machine"}, 0},
		// Exceed gRPC's 20-second attempt timeout so a stalled handshake becomes a
		// completed, classifiable error instead of only the outer deadline.
		{"stalled_handshake_30s", "tls_handshake", "deadline_exceeded", true, context.DeadlineExceeded, []string{"authentication handshake failed"}, 30 * time.Second},
		{"cancel_handshake", "connecting", "canceled", true, context.Canceled, nil, 0},
		{"cancel_after_tls_eof", "tls_handshake", "canceled", true, context.Canceled, []string{"authentication handshake failed"}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.budget == 0 {
				tc.budget = 800 * time.Millisecond
			} else if testing.Short() {
				t.Skip("requires the real gRPC handshake timeout")
			}
			ctx := config.NewContext(context.Background(), &config.Config{Tokens: &tokens.Tokens{}})
			ctx, cancel := context.WithTimeout(ctx, tc.budget)
			defer cancel()

			// A raw TCP builder that never completes a TLS handshake.
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
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
					if tc.name == "tls_eof_timeout" || tc.name == "cancel_after_tls_eof" {
						conn.Close()
						if tc.name == "cancel_after_tls_eof" {
							time.AfterFunc(100*time.Millisecond, cancel)
						}
					} else {
						if tc.name == "cancel_handshake" {
							cancel()
						}
						<-canceled
						conn.Close()
					}
				}
			}()
			defer func() { listener.Close(); <-done }()

			connection := activeConnection("tcp://"+listener.Addr().String(), "82d1104c0034d8", certPEM, keyPEM)
			if tc.name == "acquire_timeout" {
				connection = &cliv1.GetBuildKitConnectionResponse{Connection: &cliv1.GetBuildKitConnectionResponse_Pending{Pending: &cliv1.GetBuildKitConnectionResponse_PendingConnection{}}}
			}
			fakeDepotAPI(t, connection)
			ctx = uiexutil.NewContextWithClient(ctx, &mock.UiexClient{EnsureDepotBuilderFunc: func(context.Context, uiex.EnsureDepotBuilderRequest) (*uiex.EnsureDepotBuilderResponse, error) {
				if tc.name == "ensure_error" {
					return nil, errors.New("ensure sentinel")
				}
				id, token := "local-build", "local-token"
				return &uiex.EnsureDepotBuilderResponse{BuildId: &id, BuildToken: &token}, nil
			}})
			ios, _, _, _ := iostreams.Test()
			b := newBuild(42, false)
			_, buildErr := depotBuild(ctx, ios, ImageOptions{AppName: "test"}, "", b, DepotBuilderScopeOrganization)
			require.Error(t, buildErr)
			if tc.wantIs != nil {
				require.ErrorIs(t, buildErr, tc.wantIs)
			}
			for _, text := range tc.wantText {
				require.Contains(t, buildErr.Error(), text)
			}
			if tc.target {
				require.Contains(t, buildErr.Error(), "connecting to Depot builder:")
			}

			// Report through the real REST client to a loopback web.
			received := make(chan []byte, 1)
			web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/api/v1/builds/finish", r.URL.Path)
				raw, err := io.ReadAll(r.Body)
				assert.NoError(t, err)
				received <- raw
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

			body := <-received
			var request uiex.FinishBuildRequest
			require.NoError(t, json.Unmarshal(body, &request))
			require.Equal(t, "failed", request.Status)
			require.Equal(t, buildErr.Error(), request.Logs)
			require.Empty(t, request.BuilderMeta.RemoteMachineId)
			require.Equal(t, tc.stage, request.BuilderMeta.DepotConnectionStage)
			require.Equal(t, tc.outcome, request.BuilderMeta.DepotConnectionOutcome)
			if tc.target {
				require.Equal(t, "82d1104c0034d8", request.BuilderMeta.DepotAcquisitionTarget)
			} else {
				require.NotContains(t, string(body), "depot_acquisition_target")
			}
			for _, secret := range []string{"BEGIN CERTIFICATE", "PRIVATE KEY", "local-token"} {
				require.NotContains(t, string(body), secret)
			}
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

// A failed Depot report gets a bounded grace after the parent ends; other
// builders still hand the client the canceled context.
func TestDepotFinishDeadlineAndOtherBuilders(t *testing.T) {
	for _, tc := range []struct {
		name, builder string
		detached      bool
	}{
		{"other builder stays canceled", "remote", false},
		{"depot notification bounded", string(depotBuilderType), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			b := newBuild(42, false)
			b.BuilderMeta.BuilderType = tc.builder
			called := 0
			ctx = uiexutil.NewContextWithClient(ctx, &mock.UiexClient{FinishBuildFunc: func(callCtx context.Context, in uiex.FinishBuildRequest) (*uiex.BuildResponse, error) {
				called++
				if !tc.detached {
					require.ErrorIs(t, callCtx.Err(), context.Canceled)

					return nil, callCtx.Err()
				}
				require.NoError(t, callCtx.Err())
				select {
				case <-callCtx.Done():
				case <-time.After(10 * time.Second):
					t.Fatal("canceled notification exceeded grace period")
				}

				return nil, callCtx.Err()
			}})
			r := &Resolver{appName: "test", dockerFactory: &dockerClientFactory{appName: "test"}}
			_, err := r.finishBuild(ctx, b, true, "canceled", nil)
			require.ErrorIs(t, err, context.Canceled)
			require.Equal(t, 1, called)
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

// A retry reuses the SDK client with Release as the sole connection owner, and a
// completed readiness error survives the later deadline.
func TestDepotSDKReadinessRetry(t *testing.T) {
	for _, tc := range []struct {
		name  string
		stall bool
	}{
		{"retry_success", false},
		{"deadline_after_error", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			certPEM, keyPEM := depotTestCert(t)
			cert, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
			require.NoError(t, err)
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			server := grpc.NewServer(grpc.Creds(credentials.NewTLS(&tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12})))
			service := &depotRetryControl{stall: tc.stall}
			control.RegisterControlServer(server, service)
			go server.Serve(listener)
			defer server.Stop()
			// Machine.Release also owns the acquisition health reporter, so obtain the
			// machine through the SDK Acquire path rather than constructing it directly.
			fakeDepotAPI(t, activeConnection("tcp://"+listener.Addr().String(), "builder.test", certPEM, keyPEM))
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			machine, err := depotmachine.Acquire(ctx, "local-build", "local-token", "amd64")
			require.NoError(t, err)
			defer func() { require.NoError(t, machine.Release()) }()
			meta := &uiex.BuilderMeta{}
			c, err := connectDepotSDK(ctx, machine, meta)
			require.Equal(t, int32(2), service.calls.Load())
			if tc.stall {
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

func activeConnection(endpoint, serverName, certPEM, keyPEM string) *cliv1.GetBuildKitConnectionResponse {
	return &cliv1.GetBuildKitConnectionResponse{Connection: &cliv1.GetBuildKitConnectionResponse_Active{Active: &cliv1.GetBuildKitConnectionResponse_ActiveConnection{
		Endpoint: endpoint, ServerName: serverName, Cert: &cliv1.Cert{Cert: certPEM, Key: keyPEM}, CaCert: &cliv1.Cert{Cert: certPEM},
	}}}
}

// fakeDepotAPI answers the SDK's acquisition and reporting RPCs from loopback and
// points the SDK at it. The SDK writes key material to TMPDIR, so keep that per test.
func fakeDepotAPI(t *testing.T, connection *cliv1.GetBuildKitConnectionResponse) {
	t.Helper()
	t.Setenv("TMPDIR", t.TempDir())
	response, err := proto.Marshal(connection)
	require.NoError(t, err)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/proto")
		switch {
		case strings.HasSuffix(r.URL.Path, "/GetBuildKitConnection"):
			w.Write(response)
		case strings.HasSuffix(r.URL.Path, "/ReportBuildHealth"), strings.HasSuffix(r.URL.Path, "/FinishBuild"):
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("unexpected SDK path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(api.Close)
	t.Setenv("DEPOT_API_URL", api.URL)
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

// Model the interval where the clock deadline has passed but the context timer
// has not delivered cancellation yet. This must not depend on scheduler timing.
func TestDepotContextEndedBeforeTimer(t *testing.T) {
	for _, delta := range []time.Duration{-time.Hour, time.Hour} {
		ctx := depotClockDeadlineContext{Context: context.Background(), deadline: time.Now().Add(delta)}
		require.NoError(t, ctx.Err())
		require.Nil(t, ctx.Done())
		require.Equal(t, delta < 0, contextEnded(ctx))
	}
	require.False(t, contextEnded(context.Background()))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.True(t, contextEnded(ctx))
}

type depotClockDeadlineContext struct {
	context.Context
	deadline time.Time
}

func (c depotClockDeadlineContext) Deadline() (time.Time, bool) {
	return c.deadline, true
}
