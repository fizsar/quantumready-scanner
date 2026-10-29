package rawtls

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"slices"

	"golang.org/x/crypto/cryptobyte"
)

// SSLv2Cipher is an SSLv2 cipher kind (3 bytes on the wire).
type SSLv2Cipher struct {
	ID   uint32
	Name string
}

// SSLv2Ciphers are all SSLv2 cipher kinds; every one is broken.
var SSLv2Ciphers = []SSLv2Cipher{
	{0x010080, "SSL_CK_RC4_128_WITH_MD5"},
	{0x020080, "SSL_CK_RC4_128_EXPORT40_WITH_MD5"},
	{0x030080, "SSL_CK_RC2_128_CBC_WITH_MD5"},
	{0x040080, "SSL_CK_RC2_128_CBC_EXPORT40_WITH_MD5"},
	{0x050080, "SSL_CK_IDEA_128_CBC_WITH_MD5"},
	{0x060040, "SSL_CK_DES_64_CBC_WITH_MD5"},
	{0x0700C0, "SSL_CK_DES_192_EDE3_CBC_WITH_MD5"},
}

const (
	ssl2MsgError       = 0
	ssl2MsgClientHello = 1
	ssl2MsgServerHello = 4
	ssl2CertX509       = 1 // SSL_X509_CERTIFICATE
)

// SSLv2ServerHello is a parsed SSLv2 SERVER-HELLO.
type SSLv2ServerHello struct {
	Version      uint16
	SessionIDHit bool
	CertType     uint8
	CertLen      int
	// Certificate is the server's certificate (DER) when CertType is X.509
	// (1): SSLv2 sends the leaf alone.
	Certificate []byte
	// Ciphers are the cipher kinds the server has in common with our offer
	// (possibly none: SSLv2 is enabled but without a common cipher).
	Ciphers []uint32
}

// SSLv2Response is what a server answered to an SSLv2 CLIENT-HELLO.
type SSLv2Response struct {
	ServerHello *SSLv2ServerHello
	// ErrorCode is set when the server answered with an SSLv2 ERROR message.
	ErrorCode *uint16
	// TLSAlert or TLSVersion: the server answered in TLS, so it does not
	// speak SSLv2.
	TLSAlert   *Alert
	TLSVersion uint16
}

// MarshalSSLv2Hello builds an SSLv2 CLIENT-HELLO offering ciphers.
func MarshalSSLv2Hello(ciphers []uint32) ([]byte, error) {
	if len(ciphers) == 0 || len(ciphers) > 1000 {
		return nil, fmt.Errorf("rawtls: %d SSLv2 ciphers", len(ciphers))
	}
	var challenge [16]byte
	if _, err := rand.Read(challenge[:]); err != nil {
		return nil, err
	}
	var b cryptobyte.Builder
	b.AddUint8(ssl2MsgClientHello)
	b.AddUint16(VersionSSL20)
	b.AddUint16(uint16(3 * len(ciphers))) //nolint:gosec // at most 3000
	b.AddUint16(0)                        // session id length
	b.AddUint16(uint16(len(challenge)))
	for _, c := range ciphers {
		b.AddUint24(c)
	}
	b.AddBytes(challenge[:])
	msg, err := b.Bytes()
	if err != nil {
		return nil, err
	}
	// Two-byte record header: high bit set, 15-bit length, no padding.
	n := len(msg)
	if n > 0x7fff {
		return nil, fmt.Errorf("rawtls: SSLv2 CLIENT-HELLO of %d bytes", n)
	}
	return append(binary.BigEndian.AppendUint16(nil, 0x8000|uint16(n)), msg...), nil
}

// ExchangeSSLv2 writes an SSLv2 CLIENT-HELLO and reads the answer.
func ExchangeSSLv2(conn io.ReadWriter, ciphers []uint32) (*SSLv2Response, error) {
	msg, err := MarshalSSLv2Hello(ciphers)
	if err != nil {
		return nil, err
	}
	if _, err := conn.Write(msg); err != nil {
		return nil, err
	}
	return readSSLv2Response(conn, ciphers)
}

func readSSLv2Response(r io.Reader, ciphers []uint32) (*SSLv2Response, error) {
	lr := &limitedReader{r: r, n: MaxResponseBytes}
	hdr, err := lr.readFull(2)
	if err != nil {
		return nil, err
	}
	// A TLS record (alert or handshake, version 3.x): the server ignored SSLv2.
	if (hdr[0] == recordAlert || hdr[0] == recordHandshake) && hdr[1] == 3 {
		rr := newRecordReader(io.MultiReader(bytes.NewReader(hdr), lr.r), lr.n+2)
		typ, body, alert, err := rr.next()
		if err != nil {
			return nil, err
		}
		if alert != nil {
			return &SSLv2Response{TLSAlert: alert}, nil
		}
		if typ != typeServerHello {
			return nil, malformed("handshake message %d instead of ServerHello", typ)
		}
		sh, err := ParseServerHello(body)
		if err != nil {
			return nil, err
		}
		return &SSLv2Response{TLSVersion: sh.Version}, nil
	}

	var length, padding int
	if hdr[0]&0x80 != 0 {
		length = int(hdr[0]&0x7f)<<8 | int(hdr[1])
	} else {
		length = int(hdr[0]&0x3f)<<8 | int(hdr[1])
		p, err := lr.readFull(1)
		if err != nil {
			return nil, err
		}
		padding = int(p[0])
	}
	if length == 0 || padding >= length {
		return nil, malformed("SSLv2 record length %d, padding %d", length, padding)
	}
	body, err := lr.readFull(length)
	if err != nil {
		return nil, err
	}
	body = body[:length-padding]
	switch body[0] {
	case ssl2MsgError:
		if len(body) != 3 {
			return nil, malformed("SSLv2 ERROR of %d bytes", len(body))
		}
		code := uint16(body[1])<<8 | uint16(body[2])
		return &SSLv2Response{ErrorCode: &code}, nil
	case ssl2MsgServerHello:
		sh, err := ParseSSLv2ServerHello(body[1:])
		if err != nil {
			return nil, err
		}
		for _, c := range sh.Ciphers {
			if !slices.Contains(ciphers, c) {
				return nil, &UnofferedError{"SSLv2 cipher", c}
			}
		}
		return &SSLv2Response{ServerHello: sh}, nil
	}
	return nil, malformed("SSLv2 message type %d", body[0])
}

// ParseSSLv2ServerHello parses an SSLv2 SERVER-HELLO after its type byte.
func ParseSSLv2ServerHello(body []byte) (*SSLv2ServerHello, error) {
	s := cryptobyte.String(body)
	sh := &SSLv2ServerHello{}
	var hit uint8
	var certLen, specsLen, connIDLen uint16
	if !s.ReadUint8(&hit) || !s.ReadUint8(&sh.CertType) || !s.ReadUint16(&sh.Version) ||
		!s.ReadUint16(&certLen) || !s.ReadUint16(&specsLen) || !s.ReadUint16(&connIDLen) {
		return nil, malformed("short SSLv2 SERVER-HELLO")
	}
	var cert, specs, connID []byte
	if !s.ReadBytes(&cert, int(certLen)) || !s.ReadBytes(&specs, int(specsLen)) || !s.ReadBytes(&connID, int(connIDLen)) || !s.Empty() {
		return nil, malformed("SSLv2 SERVER-HELLO lengths do not add up")
	}
	if specsLen%3 != 0 || connIDLen < 16 || connIDLen > 32 {
		return nil, malformed("SSLv2 SERVER-HELLO: %d cipher bytes, connection id of %d", specsLen, connIDLen)
	}
	sh.SessionIDHit = hit != 0
	sh.CertLen = int(certLen)
	if sh.CertType == ssl2CertX509 && len(cert) > 0 {
		sh.Certificate = slices.Clone(cert)
	}
	for i := 0; i < len(specs); i += 3 {
		sh.Ciphers = append(sh.Ciphers, uint32(specs[i])<<16|uint32(specs[i+1])<<8|uint32(specs[i+2]))
	}
	return sh, nil
}
