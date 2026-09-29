//go:build js && wasm

package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"syscall/js"
	"time"

	"github.com/superfly/flyctl/ssh"
	"github.com/superfly/flyctl/wg"
	"golang.org/x/crypto/curve25519"
	sshcrypto "golang.org/x/crypto/ssh"
)

// generateKey implements FlySSH.generateKey(): Promise<KeyPair>.
//
// The keypair is made in the browser so the private key never leaves it:
// the page hands the public key to whatever issues the certificate and
// passes both back to connect.
func generateKey(_ js.Value, _ []js.Value) any {
	return promise(func() (any, error) {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, err
		}

		sshPub, err := sshcrypto.NewPublicKey(pub)
		if err != nil {
			return nil, err
		}

		return map[string]any{
			"publicKey":  strings.TrimSpace(string(sshcrypto.MarshalAuthorizedKey(sshPub))),
			"privateKey": string(ssh.MarshalED25519PrivateKey(priv, "flyctl-wasm")),
		}, nil
	})
}

func wireguardKeypair() (pub, priv string, err error) {
	var private [32]byte
	if _, err := rand.Read(private[:]); err != nil {
		return "", "", err
	}

	public, err := curve25519.X25519(private[:], curve25519.Basepoint)
	if err != nil {
		return "", "", err
	}

	return base64.StdEncoding.EncodeToString(public), base64.StdEncoding.EncodeToString(private[:]), nil
}

// tokenSource turns ConnectOptions.token (a string, or a function
// returning a string or a Promise of one) into what the tunnel calls on
// every reconnect, so refreshed macaroons get picked up.
func tokenSource(v js.Value) (func() string, error) {
	switch v.Type() {
	case js.TypeString:
		token := v.String()

		return func() string { return token }, nil
	case js.TypeFunction:
		return func() string {
			result, err := await(v.Invoke())
			if err != nil {
				log.Printf("token callback failed: %s", err)

				return ""
			}
			if result.Type() != js.TypeString {
				log.Printf("token callback returned a %s, not a string", result.Type())

				return ""
			}

			return result.String()
		}, nil
	default:
		return nil, errors.New("token must be a string or a function")
	}
}

type connection struct {
	ctx    context.Context
	cancel context.CancelFunc
	tunnel *wg.Tunnel
	client *ssh.Client
	target ssh.SessionTarget

	closeOnce sync.Once
}

// connect implements FlySSH.connect(options: ConnectOptions):
// Promise<Connection>. The resolved object carries peerIP, address, and
// the shell, sftp and close methods below.
func connect(_ js.Value, args []js.Value) any {
	if len(args) < 1 || args[0].Type() != js.TypeObject {
		return promise(func() (any, error) { return nil, errors.New("connect: options object required") })
	}
	opts := args[0]

	return promise(func() (any, error) {
		tokenFn, err := tokenSource(opts.Get("token"))
		if err != nil {
			return nil, err
		}

		host := optString(opts, "host", "")
		if host == "" {
			return nil, errors.New("connect: host is required")
		}
		orgSlug := optString(opts, "orgSlug", "")
		if orgSlug == "" {
			return nil, errors.New("connect: orgSlug is required")
		}
		cert := optString(opts, "certificate", "")
		key := optString(opts, "privateKey", "")
		if cert == "" || key == "" {
			return nil, errors.New("connect: certificate and privateKey are required")
		}

		wgPub, wgPriv, err := wireguardKeypair()
		if err != nil {
			return nil, err
		}

		ctx, cancel := context.WithCancel(context.Background())

		state := &wg.WireGuardState{
			Org:          orgSlug,
			LocalPublic:  wgPub,
			LocalPrivate: wgPriv,
		}

		started := time.Now()

		tunnel, err := wg.ConnectToken(ctx, state, optString(opts, "gateway", wg.DefaultTokenGateway), &wg.TokenAuth{
			Token:       tokenFn,
			OrgSlug:     orgSlug,
			NetworkName: optString(opts, "network", ""),
		})
		if err != nil {
			cancel()

			return nil, fmt.Errorf("wireguard: %w", err)
		}

		log.Printf("tunnel provisioned after %s", time.Since(started).Round(time.Millisecond))

		addr := host
		if net.ParseIP(host) == nil {
			ips, err := tunnel.LookupAAAA(ctx, host)
			if err != nil {
				tunnel.Close()
				cancel()

				return nil, fmt.Errorf("resolve %s: %w", host, err)
			}
			if len(ips) == 0 {
				tunnel.Close()
				cancel()

				return nil, fmt.Errorf("resolve %s: no addresses", host)
			}
			addr = ips[0].String()
			log.Printf("resolved %s to %s after %s", host, addr, time.Since(started).Round(time.Millisecond))
		}

		client := &ssh.Client{
			Addr:        net.JoinHostPort(addr, strconv.Itoa(optInt(opts, "port", 22))),
			User:        optString(opts, "user", "root"),
			Dial:        tunnel.DialContext,
			Certificate: cert,
			PrivateKey:  key,
		}

		if err := client.Connect(ctx); err != nil {
			tunnel.Close()
			cancel()

			return nil, fmt.Errorf("ssh: %w", err)
		}
		log.Printf("ssh connected to %s after %s", client.Addr, time.Since(started).Round(time.Millisecond))

		c := &connection{
			ctx:    ctx,
			cancel: cancel,
			tunnel: tunnel,
			client: client,
			target: ssh.SessionTarget{
				Container: optString(opts, "container", ""),
				Machine:   optBool(opts, "machine"),
			},
		}

		if onClose := optFunc(opts, "onClose"); !onClose.IsUndefined() {
			go func() {
				<-tunnel.Done()
				c.close()

				reason := js.Null()
				if err := tunnel.Err(); err != nil {
					reason = js.ValueOf(err.Error())
				}
				onClose.Invoke(reason)
			}()
		}

		prov := tunnel.Provision()

		return map[string]any{
			"peerIP":  prov.PeerIP,
			"address": addr,
			"shell":   js.FuncOf(c.shell),
			"sftp":    js.FuncOf(c.sftp),
			"close": js.FuncOf(func(_ js.Value, _ []js.Value) any {
				return promise(func() (any, error) {
					c.close()

					return nil, nil
				})
			}),
		}, nil
	})
}

func (c *connection) close() {
	c.closeOnce.Do(func() {
		c.cancel()
		_ = c.client.Close()
		_ = c.tunnel.Close()
	})
}
