package rawtls

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"slices"

	"golang.org/x/crypto/cryptobyte"
)

// Limits on what we read from a server. Real first flights (ServerHello,
// certificate chain, OCSP staple, ServerKeyExchange) fit comfortably.
const (
	MaxResponseBytes = 64 << 10            // per probe, every record included
	maxRecord        = maxPlaintext + 2048 // largest record length accepted
	maxMessages      = 6                   // ServerHello, Certificate, CertificateStatus, ServerKeyExchange, CertificateRequest, ServerHelloDone
)

var (
	// ErrMalformed means the server's answer does not parse. It is never a
	// "not supported".
	ErrMalformed = errors.New("rawtls: malformed server response")
	// ErrUnoffered means the server selected something the ClientHello did
	// not offer (version, suite, group, compression, extension).
	ErrUnoffered = errors.New("rawtls: server selected a parameter we did not offer")
	// ErrLimit means the server sent more than MaxResponseBytes before the
	// part we wait for.
	ErrLimit = errors.New("rawtls: server response exceeds the read limit")
)

func malformed(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrMalformed}, args...)...)
}

// UnofferedError reports what the server selected without it being offered.
type UnofferedError struct {
	What  string // "version", "cipher suite", "group", "compression", "extension"
	Value uint32
}

func (e *UnofferedError) Error() string {
	return fmt.Sprintf("rawtls: server selected %s 0x%04x, which we did not offer", e.What, e.Value)
}

// Unwrap makes errors.Is(err, ErrUnoffered) true.
func (e *UnofferedError) Unwrap() error { return ErrUnoffered }

// Alert is a TLS alert received from the server.
type Alert struct {
	Level       uint8 // 1 warning, 2 fatal
	Description uint8
}

// alertNames is the IANA TLS Alert Description registry.
var alertNames = map[uint8]string{
	0: "close_notify", 10: "unexpected_message", 20: "bad_record_mac", 21: "decryption_failed",
	22: "record_overflow", 30: "decompression_failure", 40: "handshake_failure", 41: "no_certificate",
	42: "bad_certificate", 43: "unsupported_certificate", 44: "certificate_revoked", 45: "certificate_expired",
	46: "certificate_unknown", 47: "illegal_parameter", 48: "unknown_ca", 49: "access_denied",
	50: "decode_error", 51: "decrypt_error", 52: "too_many_cids_requested", 60: "export_restriction",
	70: "protocol_version", 71: "insufficient_security", 80: "internal_error", 86: "inappropriate_fallback",
	90: "user_canceled", 100: "no_renegotiation", 109: "missing_extension", 110: "unsupported_extension",
	111: "certificate_unobtainable", 112: "unrecognized_name", 113: "bad_certificate_status_response",
	114: "bad_certificate_hash_value", 115: "unknown_psk_identity", 116: "certificate_required",
	117: "general_error", 120: "no_application_protocol", 121: "ech_required",
}

// AlertName is the IANA name of an alert description ("alert(n)" if unassigned).
func AlertName(description uint8) string {
	if n, ok := alertNames[description]; ok {
		return n
	}
	return fmt.Sprintf("alert(%d)", description)
}

func (a Alert) String() string { return AlertName(a.Description) }

// ServerHello is a parsed ServerHello or HelloRetryRequest.
type ServerHello struct {
	LegacyVersion uint16
	// Version is the negotiated version: supported_versions when present,
	// LegacyVersion otherwise.
	Version     uint16
	Random      [32]byte
	SessionID   []byte
	CipherSuite uint16
	Compression uint8
	Extensions  []uint16 // extension types, in the order received
	// HelloRetry marks a HelloRetryRequest (TLS 1.3): the server wants a key
	// share for KeyShareGroup.
	HelloRetry bool
	// KeyShareGroup is the TLS 1.3 key_share group (or the HRR selected_group).
	KeyShareGroup uint16
	// KeyShareData is the server's key share (TLS 1.3 ServerHello).
	KeyShareData []byte
	// DowngradeSentinel is set when a TLS 1.3 server negotiated a lower
	// version and said so in Random (RFC 8446, section 4.1.3).
	DowngradeSentinel bool
}

// helloRetryRandom is SHA-256("HelloRetryRequest") (RFC 8446, section 4.1.3).
var helloRetryRandom = [32]byte{
	0xCF, 0x21, 0xAD, 0x74, 0xE5, 0x9A, 0x61, 0x11, 0xBE, 0x1D, 0x8C, 0x02, 0x1E, 0x65, 0xB8, 0x91,
	0xC2, 0xA2, 0x11, 0x16, 0x7A, 0xBB, 0x8C, 0x5E, 0x07, 0x9E, 0x09, 0xE2, 0xC8, 0xA8, 0x33, 0x9C,
}

var downgradeTLS12 = []byte("DOWNGRD\x01")
var downgradeTLS11 = []byte("DOWNGRD\x00")

// ParseServerHello parses the body of a ServerHello handshake message
// (without the 4-byte message header). It checks syntax only; Exchange checks
// the values against what was offered.
func ParseServerHello(body []byte) (*ServerHello, error) {
	s := cryptobyte.String(body)
	sh := &ServerHello{}
	var random, sessionID []byte
	if !s.ReadUint16(&sh.LegacyVersion) || !s.ReadBytes(&random, 32) ||
		!s.ReadUint8LengthPrefixed((*cryptobyte.String)(&sessionID)) ||
		!s.ReadUint16(&sh.CipherSuite) || !s.ReadUint8(&sh.Compression) {
		return nil, malformed("short ServerHello")
	}
	if len(sessionID) > 32 {
		return nil, malformed("session_id of %d bytes", len(sessionID))
	}
	copy(sh.Random[:], random)
	sh.SessionID = slices.Clone(sessionID)
	sh.HelloRetry = sh.Random == helloRetryRandom
	sh.Version = sh.LegacyVersion

	if !s.Empty() {
		var exts cryptobyte.String
		if !s.ReadUint16LengthPrefixed(&exts) || !s.Empty() {
			return nil, malformed("bad extensions block")
		}
		for !exts.Empty() {
			var typ uint16
			var data cryptobyte.String
			if !exts.ReadUint16(&typ) || !exts.ReadUint16LengthPrefixed(&data) {
				return nil, malformed("truncated extension")
			}
			if slices.Contains(sh.Extensions, typ) {
				return nil, malformed("duplicate extension %d", typ)
			}
			sh.Extensions = append(sh.Extensions, typ)
			switch typ {
			case extSupportedVersions:
				if !data.ReadUint16(&sh.Version) || !data.Empty() {
					return nil, malformed("bad supported_versions")
				}
			case extKeyShare:
				if !data.ReadUint16(&sh.KeyShareGroup) {
					return nil, malformed("bad key_share")
				}
				if !sh.HelloRetry {
					var key cryptobyte.String
					if !data.ReadUint16LengthPrefixed(&key) || key.Empty() {
						return nil, malformed("bad key_share")
					}
					sh.KeyShareData = slices.Clone([]byte(key))
				}
				if !data.Empty() {
					return nil, malformed("bad key_share")
				}
			}
		}
	}
	if sh.HelloRetry && !slices.Contains(sh.Extensions, extSupportedVersions) {
		return nil, malformed("HelloRetryRequest without supported_versions")
	}
	tail := sh.Random[24:]
	sh.DowngradeSentinel = bytes.Equal(tail, downgradeTLS12) || bytes.Equal(tail, downgradeTLS11)
	return sh, nil
}

// KeyExchange holds the server's ephemeral parameters from ServerKeyExchange.
type KeyExchange struct {
	// Kind is "DH" (DHE and anonymous DH), "EC" (ECDHE) or "RSA" (the
	// temporary key of RSA_EXPORT suites).
	Kind           string
	DHPrimeBits    int    // size of the DH prime p
	Group          uint16 // EC named curve
	ExplicitCurve  bool   // EC parameters sent explicitly instead of named
	RSAModulusBits int
}

// ParseServerKeyExchange parses the parameters at the start of a
// ServerKeyExchange body for the negotiated suite. The signature after them is
// not checked: we only observe.
func ParseServerKeyExchange(body []byte, suite Suite) (*KeyExchange, error) {
	s := cryptobyte.String(body)
	switch {
	case suite.Kx == "DHE":
		var p, g, ys cryptobyte.String
		if !s.ReadUint16LengthPrefixed(&p) || !s.ReadUint16LengthPrefixed(&g) || !s.ReadUint16LengthPrefixed(&ys) ||
			p.Empty() || g.Empty() || ys.Empty() {
			return nil, malformed("bad DH parameters")
		}
		return &KeyExchange{Kind: "DH", DHPrimeBits: bitLen(p)}, nil
	case suite.Kx == "ECDHE":
		var curveType uint8
		if !s.ReadUint8(&curveType) {
			return nil, malformed("bad EC parameters")
		}
		switch curveType {
		case 1, 2: // explicit_prime, explicit_char2 (RFC 4492); deprecated, not parsed further
			return &KeyExchange{Kind: "EC", ExplicitCurve: true}, nil
		case 3:
			ke := &KeyExchange{Kind: "EC"}
			var point cryptobyte.String
			if !s.ReadUint16(&ke.Group) || !s.ReadUint8LengthPrefixed(&point) || point.Empty() {
				return nil, malformed("bad EC parameters")
			}
			return ke, nil
		}
		return nil, malformed("EC curve type %d", curveType)
	case suite.Kx == "RSA" && suite.Export:
		var mod, exp cryptobyte.String
		if !s.ReadUint16LengthPrefixed(&mod) || !s.ReadUint16LengthPrefixed(&exp) || mod.Empty() || exp.Empty() {
			return nil, malformed("bad RSA parameters")
		}
		return &KeyExchange{Kind: "RSA", RSAModulusBits: bitLen(mod)}, nil
	}
	return nil, malformed("unexpected ServerKeyExchange for %s", suite.Name)
}

// bitLen is the bit length of a big-endian unsigned integer.
func bitLen(b []byte) int {
	for i, c := range b {
		if c != 0 {
			n := 0
			for ; c != 0; c >>= 1 {
				n++
			}
			return (len(b)-i-1)*8 + n
		}
	}
	return 0
}

// limitedReader reads at most n bytes in total and says so (ErrLimit)
// instead of reporting a plain EOF.
type limitedReader struct {
	r io.Reader
	n int
}

func (l *limitedReader) readFull(n int) ([]byte, error) {
	if n > l.n {
		return nil, ErrLimit
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(l.r, buf); err != nil {
		return nil, err
	}
	l.n -= n
	return buf, nil
}

// recordReader turns a stream of TLS records into handshake messages.
type recordReader struct {
	lr   *limitedReader
	hs   []byte // handshake bytes not yet returned
	seen int    // handshake messages returned
	// observe (ParseServerFlight) ends the stream with errEncrypted where
	// encryption starts: an application_data record, or a ChangeCipherSpec
	// once ccsEnds is set (TLS 1.0–1.2, where it precedes the encrypted
	// Finished). Exchange never reads that far.
	observe, ccsEnds bool
}

// errEncrypted marks the end of what an observer can read in clear.
var errEncrypted = errors.New("rawtls: encrypted records follow")

const recordApplicationData uint8 = 23

func newRecordReader(r io.Reader, limit int) *recordReader {
	if limit <= 0 {
		limit = MaxResponseBytes
	}
	return &recordReader{lr: &limitedReader{r: r, n: limit}}
}

// next returns the next handshake message, or the alert that came instead.
func (rr *recordReader) next() (typ uint8, body []byte, alert *Alert, err error) {
	if rr.seen >= maxMessages {
		return 0, nil, nil, malformed("more than %d handshake messages", maxMessages)
	}
	for {
		if len(rr.hs) >= 4 {
			n := int(rr.hs[1])<<16 | int(rr.hs[2])<<8 | int(rr.hs[3])
			if n > MaxResponseBytes {
				return 0, nil, nil, malformed("handshake message of %d bytes", n)
			}
			if len(rr.hs) >= 4+n {
				typ, body = rr.hs[0], rr.hs[4:4+n]
				rr.hs = rr.hs[4+n:]
				rr.seen++
				return typ, body, nil, nil
			}
		}
		hdr, err := rr.lr.readFull(5)
		if err != nil {
			return 0, nil, nil, err
		}
		ctype, length := hdr[0], int(hdr[3])<<8|int(hdr[4])
		if rr.observe && len(rr.hs) == 0 && (ctype == recordApplicationData || ctype == recordChangeCipherSpec && rr.ccsEnds) {
			return 0, nil, nil, errEncrypted
		}
		if ctype != recordHandshake && ctype != recordAlert && ctype != recordChangeCipherSpec {
			return 0, nil, nil, malformed("record type %d", ctype)
		}
		if hdr[1] != 3 {
			return 0, nil, nil, malformed("record version %d.%d", hdr[1], hdr[2])
		}
		if length == 0 || length > maxRecord {
			return 0, nil, nil, malformed("record length %d", length)
		}
		payload, err := rr.lr.readFull(length)
		if err != nil {
			return 0, nil, nil, err
		}
		switch ctype {
		case recordHandshake:
			rr.hs = append(rr.hs, payload...)
		case recordAlert:
			if len(rr.hs) > 0 {
				return 0, nil, nil, malformed("alert inside a handshake message")
			}
			if length != 2 {
				return 0, nil, nil, malformed("alert record of %d bytes", length)
			}
			return 0, nil, &Alert{Level: payload[0], Description: payload[1]}, nil
		case recordChangeCipherSpec: // TLS 1.3 middlebox compatibility; ignored
			if len(rr.hs) > 0 || length != 1 || payload[0] != 1 {
				return 0, nil, nil, malformed("unexpected ChangeCipherSpec")
			}
		}
	}
}

// Options configures Exchange.
type Options struct {
	// ReadKeyExchange keeps reading, in TLS 1.0–1.2, until ServerKeyExchange
	// (or ServerHelloDone) to learn the DH prime size or the ECDHE curve.
	ReadKeyExchange bool
	// MaxBytes caps what is read; default MaxResponseBytes.
	MaxBytes int
}

// Response is what the server answered.
type Response struct {
	// Alert is set when the server answered with an alert (instead of a
	// ServerHello, or after it).
	Alert       *Alert
	ServerHello *ServerHello
	KeyExchange *KeyExchange
}

// Exchange writes the ClientHello h to conn and reads the answer. The caller
// sets deadlines on conn and closes it afterwards. An error after a valid
// ServerHello is returned together with the Response holding it.
func Exchange(conn io.ReadWriter, h *Hello, opts Options) (*Response, error) {
	msg, err := h.Marshal()
	if err != nil {
		return nil, err
	}
	if _, err := conn.Write(msg); err != nil {
		return nil, err
	}
	return readResponse(conn, h, opts)
}

func readResponse(r io.Reader, h *Hello, opts Options) (*Response, error) {
	rr := newRecordReader(r, opts.MaxBytes)
	typ, body, alert, err := rr.next()
	if err != nil {
		return nil, err
	}
	if alert != nil {
		return &Response{Alert: alert}, nil
	}
	if typ != typeServerHello {
		return nil, malformed("handshake message %d instead of ServerHello", typ)
	}
	sh, err := ParseServerHello(body)
	if err != nil {
		return nil, err
	}
	if err := checkServerHello(h, sh); err != nil {
		return nil, err
	}
	resp := &Response{ServerHello: sh}
	if !opts.ReadKeyExchange || sh.Version > VersionTLS12 {
		return resp, nil
	}
	suite, _ := SuiteByID(sh.CipherSuite) // offered, so known
	for {
		typ, body, alert, err := rr.next()
		if err != nil {
			return resp, err
		}
		if alert != nil {
			resp.Alert = alert
			return resp, nil
		}
		switch typ {
		case typeCertificate, typeCertificateStatus:
			continue // not needed here: crypto/tls probes capture the chain
		case typeServerKeyExchange:
			resp.KeyExchange, err = ParseServerKeyExchange(body, suite)
			return resp, err
		case typeCertificateReq, typeServerHelloDone:
			return resp, nil // no ServerKeyExchange (static RSA, ECDH, DH)
		}
		return resp, malformed("unexpected handshake message %d", typ)
	}
}

// checkServerHello rejects anything the hello did not offer.
func checkServerHello(h *Hello, sh *ServerHello) error {
	if len(h.SupportedVersions) > 0 {
		if slices.Contains(sh.Extensions, extSupportedVersions) {
			if !slices.Contains(h.SupportedVersions, sh.Version) {
				return &UnofferedError{"version", uint32(sh.Version)}
			}
		} else if !slices.Contains(h.SupportedVersions, sh.Version) || sh.Version > VersionTLS12 {
			return &UnofferedError{"version", uint32(sh.Version)}
		}
	} else {
		if slices.Contains(sh.Extensions, extSupportedVersions) {
			return &UnofferedError{"extension", uint32(extSupportedVersions)}
		}
		if sh.Version < VersionSSL30 || sh.Version > h.Version {
			return &UnofferedError{"version", uint32(sh.Version)}
		}
	}
	if sh.CipherSuite == SCSVRenegotiation || !slices.Contains(h.Suites, sh.CipherSuite) {
		return &UnofferedError{"cipher suite", uint32(sh.CipherSuite)}
	}
	if sh.Compression != 0 {
		return &UnofferedError{"compression", uint32(sh.Compression)}
	}
	if sh.Version == VersionTLS13 {
		s, _ := SuiteByID(sh.CipherSuite)
		if !s.TLS13 {
			return &UnofferedError{"cipher suite", uint32(sh.CipherSuite)}
		}
		if !slices.Contains(sh.Extensions, extKeyShare) {
			return malformed("TLS 1.3 ServerHello without key_share")
		}
		if sh.HelloRetry {
			if !slices.Contains(h.Groups, sh.KeyShareGroup) {
				return &UnofferedError{"group", uint32(sh.KeyShareGroup)}
			}
		} else if !slices.ContainsFunc(h.KeyShares, func(k KeyShare) bool { return k.Group == sh.KeyShareGroup }) {
			return &UnofferedError{"group", uint32(sh.KeyShareGroup)}
		}
	} else {
		if sh.HelloRetry {
			return malformed("HelloRetryRequest below TLS 1.3")
		}
		if s, _ := SuiteByID(sh.CipherSuite); s.TLS13 || s.MinVersion() > sh.Version {
			return &UnofferedError{"cipher suite", uint32(sh.CipherSuite)} // suite not valid in this version
		}
	}
	return nil
}
