package scanner

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"slices"
	"strings"
	"syscall"
)

// probe is the raw outcome of one handshake.
type probe struct {
	status Status
	state  *tls.ConnectionState // nil unless Supported
	certs  []*x509.Certificate  // captured even if the handshake fails later
	err    error
}

func (p probe) errString() string {
	if p.err == nil {
		return ""
	}
	return p.err.Error()
}

func (p probe) summary() Handshake {
	h := Handshake{Status: p.status, Error: p.errString()}
	if p.state != nil {
		h.Version = tls.VersionName(p.state.Version)
		h.CipherSuite = tls.CipherSuiteName(p.state.CipherSuite)
		if p.state.CurveID != 0 {
			h.Group = groupName(p.state.CurveID)
			h.GroupID = uint16(p.state.CurveID)
		}
	}
	return h
}

// handshake runs one TLS handshake against t with cfg. Certificate validation
// is disabled here on purpose: the chain is captured and judged separately
// (analyzeCertificate), so that an invalid certificate does not hide the
// protocol facts.
func (s *Scanner) handshake(ctx context.Context, t target, cfg *tls.Config) probe {
	if err := s.acquire(ctx); err != nil {
		return probe{status: ProbeFailed, err: err}
	}
	defer s.release()

	ctx, cancel := context.WithTimeout(ctx, s.opts.HandshakeTimeout)
	defer cancel()

	conn, err := s.dial(ctx, t.addr)
	if err != nil {
		return probe{status: ProbeFailed, err: fmt.Errorf("connect: %w", err)}
	}
	defer conn.Close()

	var certs []*x509.Certificate
	cfg = cfg.Clone()
	cfg.ServerName = t.sni
	cfg.InsecureSkipVerify = true
	// Every probe must be a full handshake: no session resumption.
	cfg.SessionTicketsDisabled = true
	cfg.ClientSessionCache = nil
	cfg.VerifyPeerCertificate = func(raw [][]byte, _ [][]*x509.Certificate) error {
		for _, der := range raw {
			c, err := x509.ParseCertificate(der)
			if err != nil {
				break // crypto/tls has already parsed them; keep what we have
			}
			certs = append(certs, c)
		}
		return nil
	}

	tc := tls.Client(conn, cfg)
	if err := tc.HandshakeContext(ctx); err != nil {
		return probe{status: classify(err), certs: certs, err: err}
	}
	st := tc.ConnectionState()
	return probe{status: Supported, state: &st, certs: certs}
}

// classify decides whether a failed handshake means "the server refuses what
// we offered" or "we could not tell".
func classify(err error) Status {
	// Alerts received from the server (handshake_failure, protocol_version…)
	// surface as *net.OpError{Op: "remote error"}; tls.AlertError is for QUIC.
	var op *net.OpError
	if errors.As(err, &op) && op.Op == "remote error" {
		return NotSupported
	}
	var alert tls.AlertError
	if errors.As(err, &alert) {
		return NotSupported
	}
	var nerr net.Error
	if errors.As(err, &nerr) && nerr.Timeout() {
		return ProbeFailed
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return ProbeFailed
	}
	// Servers that reject a ClientHello by closing the connection.
	if errors.Is(err, io.EOF) || errors.Is(err, syscall.ECONNRESET) {
		return NotSupported
	}
	// The server answered with parameters we did not offer.
	msg := err.Error()
	for _, s := range []string{
		"server selected unsupported protocol version",
		"server chose an unconfigured cipher suite",
		"server selected unsupported group",
		"server selected an unsupported group",
		"no supported versions",
	} {
		if strings.Contains(msg, s) {
			return NotSupported
		}
	}
	return ProbeFailed
}

func isHybrid(id tls.CurveID) bool {
	for _, h := range HybridGroups {
		if id == h {
			return true
		}
	}
	return false
}

// groupName returns the IANA name of a group (x25519, secp256r1, X25519MLKEM768…).
func groupName(id tls.CurveID) string {
	switch id {
	case tls.X25519:
		return "x25519"
	case tls.CurveP256:
		return "secp256r1"
	case tls.CurveP384:
		return "secp384r1"
	case tls.CurveP521:
		return "secp521r1"
	case tls.X25519MLKEM768, tls.SecP256r1MLKEM768, tls.SecP384r1MLKEM1024:
		return id.String()
	}
	return fmt.Sprintf("0x%04x", uint16(id))
}

func groupNames(ids []tls.CurveID) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = groupName(id)
	}
	return out
}

// allTLS12Suites returns every TLS 1.0–1.2 suite crypto/tls can negotiate,
// insecure ones included: the point is to find out what the server accepts.
func allTLS12Suites() []uint16 {
	var ids []uint16
	for _, list := range [][]*tls.CipherSuite{tls.CipherSuites(), tls.InsecureCipherSuites()} {
		for _, cs := range list {
			for _, v := range cs.SupportedVersions {
				if v <= tls.VersionTLS12 {
					ids = append(ids, cs.ID)
					break
				}
			}
		}
	}
	return ids
}

// suitesFor returns the suites crypto/tls can negotiate in version v (TLS 1.0–1.2).
func suitesFor(v uint16) []uint16 {
	var ids []uint16
	for _, list := range [][]*tls.CipherSuite{tls.CipherSuites(), tls.InsecureCipherSuites()} {
		for _, cs := range list {
			if slices.Contains(cs.SupportedVersions, v) {
				ids = append(ids, cs.ID)
			}
		}
	}
	return ids
}

func ecdheSuites() []uint16 {
	var ids []uint16
	for _, id := range allTLS12Suites() {
		if strings.HasPrefix(tls.CipherSuiteName(id), "TLS_ECDHE_") {
			ids = append(ids, id)
		}
	}
	return ids
}

func suiteInfo(id uint16) CipherSuite {
	name := tls.CipherSuiteName(id)
	insecure := false
	for _, cs := range tls.InsecureCipherSuites() {
		if cs.ID == id {
			insecure = true
		}
	}
	return CipherSuite{
		Name:           name,
		ID:             id,
		ForwardSecrecy: strings.HasPrefix(name, "TLS_ECDHE_") || strings.HasPrefix(name, "TLS_DHE_"),
		AEAD:           strings.Contains(name, "_GCM_") || strings.Contains(name, "CHACHA20_POLY1305"),
		Insecure:       insecure,
	}
}
