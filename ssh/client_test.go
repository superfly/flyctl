package ssh

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestSessionTargetValidate(t *testing.T) {
	for _, tc := range []struct {
		name    string
		target  SessionTarget
		wantErr bool
	}{
		{
			// The machine decides where the session lands.
			name: "neither",
		},
		{
			name:   "container",
			target: SessionTarget{Container: "app"},
		},
		{
			name:   "machine",
			target: SessionTarget{Machine: true},
		},
		{
			// Mutually exclusive: rejected rather than silently preferring one.
			name:    "both",
			target:  SessionTarget{Container: "app", Machine: true},
			wantErr: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.target.validate()

			if tc.wantErr && err == nil {
				t.Fatal("expected an error, got none")
			}

			if !tc.wantErr && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	}
}

func TestConnectStopsWhenTheContextIsCanceled(t *testing.T) {
	// The far end never speaks SSH, so the handshake blocks on the version
	// exchange until something closes the socket under it.
	local, remote := net.Pipe()
	defer remote.Close()

	certificate, privateKey := testCredentials(t)

	client := &Client{
		Addr:        "192.0.2.1:22",
		User:        "root",
		Certificate: certificate,
		PrivateKey:  privateKey,
		Dial: func(context.Context, string, string) (net.Conn, error) {
			return local, nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := client.Connect(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	// A socket left open here strands the handshake goroutine on it.
	if _, err := remote.Read(make([]byte, 1)); err == nil {
		t.Fatal("expected the socket to be closed, it still reads")
	}
}

// testCredentials returns a self-signed user certificate and its private key,
// which is as much as Connect parses before it dials.
func testCredentials(t *testing.T) (certificate, privateKey string) {
	t.Helper()

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}

	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}

	cert := &ssh.Certificate{
		Key:         sshPub,
		CertType:    ssh.UserCert,
		ValidBefore: ssh.CertTimeInfinity,
	}
	if err := cert.SignCert(rand.Reader, signer); err != nil {
		t.Fatal(err)
	}

	return string(ssh.MarshalAuthorizedKey(cert)), string(MarshalED25519PrivateKey(priv, "test"))
}
