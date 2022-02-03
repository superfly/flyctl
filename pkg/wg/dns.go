package wg

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"time"

	"github.com/miekg/dns"
)

type Resolver struct {
	t  *Tunnel
	ns *net.UDPAddr
}

var (
	ErrDNSTransient = errors.New("transient error")
)

func NewResolver(t *Tunnel, nsaddr string) (*Resolver, error) {
	r := &Resolver{
		t: t,
		ns: &net.UDPAddr{
			IP:   net.ParseIP(nsaddr),
			Port: 53,
		},
	}

	if r.ns.IP == nil {
		return nil, fmt.Errorf("resolver: bad ns addr")
	}

	return r, nil
}

func (r *Resolver) LookupHost(ctx context.Context, name string) ([]string, error) {
	var m dns.Msg
	m.SetQuestion(dns.Fqdn(name), dns.TypeAAAA)

	reply, err := r.roundTrip(ctx, &m)
	if err != nil {
		return nil, fmt.Errorf("lookup: %w", err)
	}

	results := []string{}

	for _, a := range reply.Answer {
		results = append(results, a.(*dns.AAAA).AAAA.String())
	}

	return results, nil
}

func (r *Resolver) LookupAAAA(ctx context.Context, name string) ([]net.IP, error) {
	var m dns.Msg
	m.SetQuestion(dns.Fqdn(name), dns.TypeAAAA)

	reply, err := r.roundTrip(ctx, &m)
	if err != nil {
		return nil, fmt.Errorf("lookup: %w", err)
	}

	results := []net.IP{}

	for _, a := range reply.Answer {
		results = append(results, a.(*dns.AAAA).AAAA)
	}

	return results, nil
}

func (r *Resolver) LookupTXT(ctx context.Context, name string) ([]string, error) {
	var m dns.Msg
	m.SetQuestion(dns.Fqdn(name), dns.TypeTXT)

	reply, err := r.roundTrip(ctx, &m)
	if err != nil {
		return nil, fmt.Errorf("lookup: %w", err)
	}

	results := []string{}

	for _, a := range reply.Answer {
		txt := a.(*dns.TXT)

		results = append(results, txt.Txt...)
	}

	return results, nil
}

func qid() uint16 {
	var buf [2]byte

	_, err := rand.Read(buf[:])
	if err != nil {
		log.Panicf("read from random: %s", err)
	}

	return binary.LittleEndian.Uint16(buf[:])
}

func (r *Resolver) roundTrip(ctx context.Context, msg *dns.Msg) (*dns.Msg, error) {
	reply, err := r.roundTripUDP(ctx, msg)
	if err == nil || !errors.Is(err, ErrDNSTransient) {
		return reply, err
	}

	client := dns.Client{
		Net: "tcp",
		Dialer: &net.Dialer{
			Resolver: r.t.resolv,
		},
	}

	c, err := r.t.DialContext(ctx, "tcp", net.JoinHostPort(r.t.dnsIP.String(), "53"))
	if err != nil {
		return nil, err
	}
	defer c.Close()

	conn := &dns.Conn{Conn: c}
	defer conn.Close()

	reply, _, err = client.ExchangeWithConn(msg, conn)
	return reply, err
}

// returns ErrDNSTransient on truncated UDP plus routine network
// I/O issues and timeout; other errors aren't transient, and don't
// fall back
func (r *Resolver) roundTripUDP(ctx context.Context, msg *dns.Msg) (*dns.Msg, error) {
	uc, err := r.t.net.DialUDP(nil, r.ns)
	if err != nil {
		return nil, fmt.Errorf("dns round trip: contact %s: %w", r.ns, err)
	}

	defer uc.Close()

	msg.Id = qid()

	raw, err := msg.Pack()
	if err != nil {
		return nil, fmt.Errorf("dns round trip: marshal: %w", err)
	}

	// 50, 150, 450
	deadline := time.Duration(50 * time.Millisecond)

	for attempt := 0; attempt < 3; attempt++ {
		_, err := uc.Write(raw)
		if err != nil {
			return nil, fmt.Errorf("dns round trip: send to %s: %s (%w)", r.ns, err, ErrDNSTransient)
		}

		var replyBuf [1500]byte

		deadline *= time.Duration(attempt + 1)
		uc.SetReadDeadline(time.Now().Add(deadline))
		n64, err := uc.Read(replyBuf[:])
		if err != nil && errors.Is(err, os.ErrDeadlineExceeded) {
			continue
		} else if err != nil {
			return nil, fmt.Errorf("dns round trip: read from %s: %s (%w)", r.ns, err, ErrDNSTransient)
		}

		rb := replyBuf[:n64]
		reply := &dns.Msg{}
		if err = reply.Unpack(rb); err != nil || reply.Id != msg.Id {
			return nil, fmt.Errorf("dns round trip: unpack: %s (%w)", r.ns, err, ErrDNSTransient)
		}

		if reply.MsgHdr.Truncated {
			return nil, fmt.Errorf("dns round trip: udp message truncated (%w)", ErrDNSTransient)
		}

		return reply, nil
	}

	return nil, fmt.Errorf("dns round trip: timed out reading from %s (%w)", r.ns, ErrDNSTransient)
}
