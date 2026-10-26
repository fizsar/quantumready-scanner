package rawtls

import (
	"bytes"
	"errors"
	"io"
	"slices"

	"golang.org/x/crypto/cryptobyte"
)

// Parsers for the bytes a client actually exchanged with a server, as
// recorded on the connection: what was offered and what came back, for the
// scanner's per-probe log. They only observe; nothing here is sent.

const (
	extALPN                    uint16 = 16
	extSignatureAlgorithmsCert uint16 = 50
)

// ClientHelloInfo is what a ClientHello offered, read from its bytes.
type ClientHelloInfo struct {
	// SSLv2 marks an SSLv2 CLIENT-HELLO: only LegacyVersion and SSLv2Ciphers are set.
	SSLv2         bool
	LegacyVersion uint16
	Suites        []uint16 // in the order sent, GREASE and SCSV included
	SSLv2Ciphers  []uint32
	// Extensions are the extension types in the order sent.
	Extensions []uint16
	ServerName string
	// SupportedVersions is the supported_versions extension (TLS 1.3 clients).
	SupportedVersions    []uint16
	Groups               []uint16 // supported_groups
	KeyShares            []uint16 // the groups of the key_share entries
	SignatureSchemes     []uint16 // signature_algorithms
	SignatureSchemesCert []uint16 // signature_algorithms_cert
	ALPN                 []string
}

// ParseClientHello parses the first ClientHello in b: TLS records as the
// client wrote them (a ClientHello may span records), or an SSLv2
// CLIENT-HELLO.
func ParseClientHello(b []byte) (*ClientHelloInfo, error) {
	if len(b) >= 2 && b[0]&0x80 != 0 {
		return ParseSSLv2ClientHello(b)
	}
	rr := newRecordReader(bytes.NewReader(b), len(b))
	typ, body, alert, err := rr.next()
	switch {
	case err != nil:
		return nil, err
	case alert != nil:
		return nil, malformed("alert instead of a ClientHello")
	case typ != typeClientHello:
		return nil, malformed("handshake message %d instead of ClientHello", typ)
	}
	return parseClientHelloBody(body)
}

func parseClientHelloBody(body []byte) (*ClientHelloInfo, error) {
	s := cryptobyte.String(body)
	ch := &ClientHelloInfo{}
	var sessionID, suites, compression cryptobyte.String
	if !s.ReadUint16(&ch.LegacyVersion) || !s.Skip(32) ||
		!s.ReadUint8LengthPrefixed(&sessionID) || !s.ReadUint16LengthPrefixed(&suites) ||
		!s.ReadUint8LengthPrefixed(&compression) {
		return nil, malformed("short ClientHello")
	}
	if len(suites)%2 != 0 {
		return nil, malformed("odd cipher suite list")
	}
	ch.Suites = make([]uint16, 0, len(suites)/2)
	for !suites.Empty() {
		var id uint16
		suites.ReadUint16(&id)
		ch.Suites = append(ch.Suites, id)
	}
	if s.Empty() {
		return ch, nil // no extensions (SSLv3 style)
	}
	var exts cryptobyte.String
	if !s.ReadUint16LengthPrefixed(&exts) || !s.Empty() {
		return nil, malformed("bad ClientHello extensions block")
	}
	for !exts.Empty() {
		var typ uint16
		var data cryptobyte.String
		if !exts.ReadUint16(&typ) || !exts.ReadUint16LengthPrefixed(&data) {
			return nil, malformed("truncated ClientHello extension")
		}
		ch.Extensions = append(ch.Extensions, typ)
		ok := true
		switch typ {
		case extServerName:
			var list cryptobyte.String
			ok = data.ReadUint16LengthPrefixed(&list)
			for ok && !list.Empty() {
				var kind uint8
				var name cryptobyte.String
				ok = list.ReadUint8(&kind) && list.ReadUint16LengthPrefixed(&name)
				if ok && kind == 0 && ch.ServerName == "" {
					ch.ServerName = string(name)
				}
			}
		case extSupportedGroups:
			ch.Groups, ok = readUint16List(&data)
		case extSignatureAlgorithms:
			ch.SignatureSchemes, ok = readUint16List(&data)
		case extSignatureAlgorithmsCert:
			ch.SignatureSchemesCert, ok = readUint16List(&data)
		case extSupportedVersions:
			var list cryptobyte.String
			ok = data.ReadUint8LengthPrefixed(&list) && len(list)%2 == 0
			for ok && !list.Empty() {
				var v uint16
				list.ReadUint16(&v)
				ch.SupportedVersions = append(ch.SupportedVersions, v)
			}
		case extKeyShare:
			var list cryptobyte.String
			ok = data.ReadUint16LengthPrefixed(&list)
			ch.KeyShares = []uint16{} // present, possibly empty (asks for a HelloRetryRequest)
			for ok && !list.Empty() {
				var g uint16
				var key cryptobyte.String
				ok = list.ReadUint16(&g) && list.ReadUint16LengthPrefixed(&key)
				if ok {
					ch.KeyShares = append(ch.KeyShares, g)
				}
			}
		case extALPN:
			var list cryptobyte.String
			ok = data.ReadUint16LengthPrefixed(&list)
			for ok && !list.Empty() {
				var proto cryptobyte.String
				ok = list.ReadUint8LengthPrefixed(&proto) && !proto.Empty()
				if ok {
					ch.ALPN = append(ch.ALPN, string(proto))
				}
			}
		}
		if !ok {
			return nil, malformed("bad ClientHello extension %d", typ)
		}
	}
	return ch, nil
}

func readUint16List(data *cryptobyte.String) ([]uint16, bool) {
	var list cryptobyte.String
	if !data.ReadUint16LengthPrefixed(&list) || len(list)%2 != 0 {
		return nil, false
	}
	out := make([]uint16, 0, len(list)/2)
	for !list.Empty() {
		var v uint16
		list.ReadUint16(&v)
		out = append(out, v)
	}
	return out, true
}

// ParseSSLv2ClientHello parses an SSLv2 CLIENT-HELLO record (two-byte header).
func ParseSSLv2ClientHello(b []byte) (*ClientHelloInfo, error) {
	if len(b) < 2 || b[0]&0x80 == 0 {
		return nil, malformed("not an SSLv2 record")
	}
	n := int(b[0]&0x7f)<<8 | int(b[1])
	if len(b) < 2+n {
		return nil, malformed("truncated SSLv2 record")
	}
	s := cryptobyte.String(b[2 : 2+n])
	var msgType uint8
	var version, specsLen, sessionLen, challengeLen uint16
	if !s.ReadUint8(&msgType) || msgType != ssl2MsgClientHello || !s.ReadUint16(&version) ||
		!s.ReadUint16(&specsLen) || !s.ReadUint16(&sessionLen) || !s.ReadUint16(&challengeLen) ||
		specsLen%3 != 0 {
		return nil, malformed("bad SSLv2 CLIENT-HELLO")
	}
	var specs []byte
	if !s.ReadBytes(&specs, int(specsLen)) || !s.Skip(int(sessionLen)) || !s.Skip(int(challengeLen)) || !s.Empty() {
		return nil, malformed("SSLv2 CLIENT-HELLO lengths do not add up")
	}
	ch := &ClientHelloInfo{SSLv2: true, LegacyVersion: version}
	for i := 0; i < len(specs); i += 3 {
		ch.SSLv2Ciphers = append(ch.SSLv2Ciphers, uint32(specs[i])<<16|uint32(specs[i+1])<<8|uint32(specs[i+2]))
	}
	return ch, nil
}

// Flight is what a server sent in clear in answer to a ClientHello.
type Flight struct {
	// ServerHello is the first ServerHello, or the HelloRetryRequest.
	ServerHello *ServerHello
	// Retry is the ServerHello that followed a HelloRetryRequest.
	Retry *ServerHello
	// KeyExchange holds the ServerKeyExchange parameters (TLS 1.0–1.2).
	KeyExchange *KeyExchange
	// Alert is an alert sent in clear: instead of a ServerHello, or before
	// encryption started. Alerts sent encrypted are not visible here.
	Alert *Alert
	// Encrypted is set when the bytes went on past what can be read in clear.
	Encrypted bool
}

// final is the ServerHello that set the parameters: the one after a
// HelloRetryRequest, or the first.
func (f *Flight) final() *ServerHello {
	if f.Retry != nil {
		return f.Retry
	}
	return f.ServerHello
}

// ParseServerFlight reads b, the bytes a server sent on one connection, up to
// where encryption starts. It returns what it could read even with an error:
// the recording may stop mid-record (the client closed the connection) or
// hold something that is not TLS at all.
func ParseServerFlight(b []byte) (*Flight, error) {
	f := &Flight{}
	// Room for a whole record past the end: a recording cut mid-record ends
	// in io.EOF or io.ErrUnexpectedEOF, not ErrLimit.
	rr := newRecordReader(bytes.NewReader(b), len(b)+5+maxRecord)
	rr.observe = true
	for {
		typ, body, alert, err := rr.next()
		switch {
		case errors.Is(err, errEncrypted):
			f.Encrypted = true
			return f, nil
		case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
			return f, nil // the recording ends here (the client stopped reading)
		case err != nil:
			return f, err
		case alert != nil:
			f.Alert = alert
			return f, nil
		}
		switch {
		case f.ServerHello == nil || f.ServerHello.HelloRetry && f.Retry == nil:
			if typ != typeServerHello {
				return f, malformed("handshake message %d instead of ServerHello", typ)
			}
			sh, err := ParseServerHello(body)
			if err != nil {
				return f, err
			}
			if f.ServerHello == nil {
				f.ServerHello = sh
			} else {
				f.Retry = sh
			}
			if sh.HelloRetry {
				continue
			}
			if sh.Version > VersionTLS12 {
				continue // the rest is encrypted (after an optional ChangeCipherSpec)
			}
			rr.ccsEnds = true
		case f.final().Version > VersionTLS12:
			return f, malformed("handshake message %d in clear after a TLS 1.3 ServerHello", typ)
		case typ == typeServerKeyExchange:
			suite, ok := SuiteByID(f.ServerHello.CipherSuite)
			if !ok {
				return f, malformed("ServerKeyExchange for unknown suite 0x%04x", f.ServerHello.CipherSuite)
			}
			if f.KeyExchange, err = ParseServerKeyExchange(body, suite); err != nil {
				return f, err
			}
		case slices.Contains([]uint8{typeCertificate, typeCertificateStatus, typeCertificateReq, typeServerHelloDone}, typ):
			// Not needed here; after ServerHelloDone only an alert or the
			// ChangeCipherSpec can come in clear.
		default:
			return f, malformed("unexpected handshake message %d", typ)
		}
	}
}
