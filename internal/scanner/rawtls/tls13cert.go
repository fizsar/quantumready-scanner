package rawtls

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/mlkem"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"io"
	"slices"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/cryptobyte"
)

// A TLS 1.3 server sends its certificate and the scheme of its
// CertificateVerify encrypted, with keys only the two ends can derive. To see
// which certificate a server presents for a given offer (with or without
// ML-DSA among the signature schemes), ExchangeCertificate keeps the private
// part of its key share, derives the server's handshake traffic keys
// (RFC 8446, section 7.1) and decrypts the server's first flight. It stops
// after CertificateVerify: it never sends Finished, checks no signature and
// throws the keys away with the connection.

const (
	typeEncryptedExtensions uint8 = 8
	typeCertificateVerify   uint8 = 15
	typeFinished            uint8 = 20
)

// ErrRetry means the server asked for a key share in another group
// (HelloRetryRequest): ExchangeCertificate sends one key share only.
var ErrRetry = errors.New("rawtls: the server asked for another key share (HelloRetryRequest)")

// CertificateSuites are the TLS 1.3 suites ExchangeCertificate can decrypt.
var CertificateSuites = []uint16{0x1301, 0x1302, 0x1303}

// keyExchange is a key share and how to finish the exchange with the
// server's share.
type keyExchange struct {
	share  KeyShare
	finish func(server []byte) ([]byte, error)
}

func ecdhExchange(group uint16, curve ecdh.Curve) (*keyExchange, error) {
	k, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return &keyExchange{KeyShare{group, k.PublicKey().Bytes()}, func(server []byte) ([]byte, error) {
		pub, err := curve.NewPublicKey(server)
		if err != nil {
			return nil, malformed("server key share: %v", err)
		}
		return k.ECDH(pub)
	}}, nil
}

type kem interface {
	EncapsulationKey() []byte
	Decapsulate(ciphertext []byte) ([]byte, error)
}

type kem768 struct{ *mlkem.DecapsulationKey768 }

func (k kem768) EncapsulationKey() []byte { return k.DecapsulationKey768.EncapsulationKey().Bytes() }

type kem1024 struct{ *mlkem.DecapsulationKey1024 }

func (k kem1024) EncapsulationKey() []byte { return k.DecapsulationKey1024.EncapsulationKey().Bytes() }

// hybridExchange is an ECDHE-MLKEM hybrid (draft-ietf-tls-ecdhe-mlkem):
// X25519MLKEM768 puts the ML-KEM part first, the NIST-curve ones the ECDH part.
func hybridExchange(group uint16, curve ecdh.Curve, k kem, ciphertextSize int, mlkemFirst bool) (*keyExchange, error) {
	e, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	ek, pub := k.EncapsulationKey(), e.PublicKey().Bytes()
	share := append(slices.Clone(pub), ek...)
	if mlkemFirst {
		share = append(slices.Clone(ek), pub...)
	}
	return &keyExchange{KeyShare{group, share}, func(server []byte) ([]byte, error) {
		if len(server) != ciphertextSize+len(pub) {
			return nil, malformed("hybrid key share of %d bytes", len(server))
		}
		ct, point := server[len(pub):], server[:len(pub)]
		if mlkemFirst {
			ct, point = server[:ciphertextSize], server[ciphertextSize:]
		}
		ss, err := k.Decapsulate(ct)
		if err != nil {
			return nil, malformed("ML-KEM ciphertext: %v", err)
		}
		p, err := curve.NewPublicKey(point)
		if err != nil {
			return nil, malformed("server key share: %v", err)
		}
		es, err := e.ECDH(p)
		if err != nil {
			return nil, err
		}
		if mlkemFirst {
			return append(ss, es...), nil
		}
		return append(es, ss...), nil
	}}, nil
}

// KeyShareSupported reports whether ExchangeCertificate can make a key share
// in group.
func KeyShareSupported(group uint16) bool {
	return slices.Contains([]uint16{0x001d, 0x0017, 0x0018, 0x0019, 0x11ec, 0x11eb, 0x11ed}, group)
}

func newKeyExchange(group uint16) (*keyExchange, error) {
	switch group {
	case 0x001d:
		return ecdhExchange(group, ecdh.X25519())
	case 0x0017:
		return ecdhExchange(group, ecdh.P256())
	case 0x0018:
		return ecdhExchange(group, ecdh.P384())
	case 0x0019:
		return ecdhExchange(group, ecdh.P521())
	case 0x11ec, 0x11eb: // X25519MLKEM768, SecP256r1MLKEM768
		k, err := mlkem.GenerateKey768()
		if err != nil {
			return nil, err
		}
		if group == 0x11ec {
			return hybridExchange(group, ecdh.X25519(), kem768{k}, mlkem.CiphertextSize768, true)
		}
		return hybridExchange(group, ecdh.P256(), kem768{k}, mlkem.CiphertextSize768, false)
	case 0x11ed: // SecP384r1MLKEM1024
		k, err := mlkem.GenerateKey1024()
		if err != nil {
			return nil, err
		}
		return hybridExchange(group, ecdh.P384(), kem1024{k}, mlkem.CiphertextSize1024, false)
	}
	return nil, fmt.Errorf("rawtls: no key share for group 0x%04x", group)
}

// CertificateProbe is a TLS 1.3 ClientHello sent to read the server's certificate.
type CertificateProbe struct {
	ServerName       string
	Suites           []uint16 // from CertificateSuites
	Groups           []uint16 // supported_groups
	KeyShareGroup    uint16   // the one key share sent: in Groups, and KeyShareSupported
	SignatureSchemes []uint16
}

// CertificateFlight is what the server answered, decrypted.
type CertificateFlight struct {
	ServerHello *ServerHello
	// Chain is the certificates as sent, leaf first (DER).
	Chain [][]byte
	// SignatureScheme is the scheme of CertificateVerify, 0 if it did not come.
	SignatureScheme uint16
	// Alert is an alert from the server, in clear or decrypted.
	Alert *Alert
	// Encrypted is set when Alert came encrypted (its level is then known too).
	Encrypted bool
}

// ExchangeCertificate sends p and reads, decrypting, up to the server's
// CertificateVerify. The caller sets deadlines on conn and closes it.
func ExchangeCertificate(conn io.ReadWriter, p CertificateProbe) (*CertificateFlight, error) {
	for _, s := range p.Suites {
		if !slices.Contains(CertificateSuites, s) {
			return nil, fmt.Errorf("rawtls: cannot decrypt suite 0x%04x", s)
		}
	}
	if !slices.Contains(p.Groups, p.KeyShareGroup) {
		return nil, fmt.Errorf("rawtls: key share group 0x%04x not in supported_groups", p.KeyShareGroup)
	}
	kx, err := newKeyExchange(p.KeyShareGroup)
	if err != nil {
		return nil, err
	}
	h := &Hello{Version: VersionTLS12, SupportedVersions: []uint16{VersionTLS13}, Suites: p.Suites, ServerName: p.ServerName,
		Groups: p.Groups, KeyShares: []KeyShare{kx.share}, SignatureSchemes: p.SignatureSchemes}
	records, err := h.Marshal()
	if err != nil {
		return nil, err
	}
	if _, err := conn.Write(records); err != nil {
		return nil, err
	}
	rr := newRecordReader(conn, MaxResponseBytes)
	typ, body, alert, err := rr.next()
	switch {
	case err != nil:
		return nil, err
	case alert != nil:
		return &CertificateFlight{Alert: alert}, nil
	case typ != typeServerHello:
		return nil, malformed("handshake message %d instead of ServerHello", typ)
	}
	sh, err := ParseServerHello(body)
	if err != nil {
		return nil, err
	}
	if err := checkServerHello(h, sh); err != nil {
		return nil, err
	}
	f := &CertificateFlight{ServerHello: sh}
	if sh.HelloRetry {
		return f, ErrRetry
	}
	if len(rr.hs) > 0 {
		return f, malformed("handshake data in clear after a TLS 1.3 ServerHello")
	}
	shared, err := kx.finish(sh.KeyShareData)
	if err != nil {
		return f, err
	}
	transcript := append(handshakeMessages(records), message(typeServerHello, body)...)
	dec, err := serverHandshakeDecrypter(sh.CipherSuite, shared, transcript)
	if err != nil {
		return f, err
	}
	return f, readEncryptedFlight(rr.lr, dec, f)
}

// handshakeMessages strips the record headers of a client's records.
func handshakeMessages(records []byte) []byte {
	var out []byte
	for len(records) >= 5 {
		n := int(records[3])<<8 | int(records[4])
		out = append(out, records[5:5+n]...)
		records = records[5+n:]
	}
	return out
}

func message(typ uint8, body []byte) []byte {
	var b cryptobyte.Builder
	b.AddUint8(typ)
	b.AddUint24LengthPrefixed(func(b *cryptobyte.Builder) { b.AddBytes(body) })
	return b.BytesOrPanic() // body came from records: far below 2^24 bytes
}

// decrypter opens the server's records in one direction (RFC 8446, 5.3).
type decrypter struct {
	aead cipher.AEAD
	iv   []byte
	seq  uint64
}

func (d *decrypter) open(header, payload []byte) ([]byte, error) {
	nonce := slices.Clone(d.iv)
	var seq [8]byte
	binary.BigEndian.PutUint64(seq[:], d.seq)
	for i, b := range seq {
		nonce[len(nonce)-8+i] ^= b
	}
	d.seq++
	return d.aead.Open(nil, nonce, payload, header)
}

func expandLabel(h func() hash.Hash, secret []byte, label string, context []byte, length int) ([]byte, error) {
	var b cryptobyte.Builder
	b.AddUint16(uint16(length)) //nolint:gosec // a key, IV or hash length
	b.AddUint8LengthPrefixed(func(b *cryptobyte.Builder) { b.AddBytes([]byte("tls13 " + label)) })
	b.AddUint8LengthPrefixed(func(b *cryptobyte.Builder) { b.AddBytes(context) })
	info, err := b.Bytes()
	if err != nil {
		return nil, err
	}
	return hkdf.Expand(h, secret, string(info), length)
}

// serverHandshakeDecrypter derives server_handshake_traffic_secret and its
// key and IV from the (EC)DHE secret and the ClientHello…ServerHello transcript.
func serverHandshakeDecrypter(suite uint16, shared, transcript []byte) (*decrypter, error) {
	h, keyLen := sha256.New, 16
	switch suite {
	case 0x1302:
		h, keyLen = sha512.New384, 32
	case 0x1303:
		keyLen = 32
	}
	size := h().Size()
	early, err := hkdf.Extract(h, make([]byte, size), nil)
	if err != nil {
		return nil, err
	}
	empty := h()
	derived, err := expandLabel(h, early, "derived", empty.Sum(nil), size)
	if err != nil {
		return nil, err
	}
	hs, err := hkdf.Extract(h, shared, derived)
	if err != nil {
		return nil, err
	}
	th := h()
	th.Write(transcript)
	secret, err := expandLabel(h, hs, "s hs traffic", th.Sum(nil), size)
	if err != nil {
		return nil, err
	}
	key, err := expandLabel(h, secret, "key", nil, keyLen)
	if err != nil {
		return nil, err
	}
	iv, err := expandLabel(h, secret, "iv", nil, 12)
	if err != nil {
		return nil, err
	}
	var aead cipher.AEAD
	if suite == 0x1303 {
		aead, err = chacha20poly1305.New(key)
	} else {
		var block cipher.Block
		if block, err = aes.NewCipher(key); err == nil {
			aead, err = cipher.NewGCM(block)
		}
	}
	if err != nil {
		return nil, err
	}
	return &decrypter{aead: aead, iv: iv}, nil
}

// readEncryptedFlight reads the server's records after its ServerHello,
// decrypting them, until CertificateVerify, Finished or an alert.
func readEncryptedFlight(lr *limitedReader, dec *decrypter, f *CertificateFlight) error {
	var hs []byte
	for {
		hdr, err := lr.readFull(5)
		if err != nil {
			return err
		}
		length := int(hdr[3])<<8 | int(hdr[4])
		if hdr[1] != 3 || length == 0 || length > maxRecord {
			return malformed("record type %d, length %d", hdr[0], length)
		}
		payload, err := lr.readFull(length)
		if err != nil {
			return err
		}
		switch hdr[0] {
		case recordChangeCipherSpec: // middlebox compatibility; ignored
			continue
		case recordAlert:
			if length != 2 {
				return malformed("alert record of %d bytes", length)
			}
			f.Alert = &Alert{Level: payload[0], Description: payload[1]}
			return nil
		case recordApplicationData:
		default:
			return malformed("record type %d after a TLS 1.3 ServerHello", hdr[0])
		}
		plain, err := dec.open(hdr, payload)
		if err != nil {
			return malformed("cannot decrypt the server's flight: %v", err)
		}
		i := len(plain) - 1
		for i >= 0 && plain[i] == 0 {
			i--
		}
		if i < 0 {
			return malformed("empty inner record")
		}
		inner, content := plain[i], plain[:i]
		switch inner {
		case recordAlert:
			if len(content) != 2 {
				return malformed("alert of %d bytes", len(content))
			}
			f.Alert, f.Encrypted = &Alert{Level: content[0], Description: content[1]}, true
			return nil
		case recordHandshake:
			hs = append(hs, content...)
		default:
			return malformed("inner content type %d", inner)
		}
		for len(hs) >= 4 {
			n := int(hs[1])<<16 | int(hs[2])<<8 | int(hs[3])
			if n > MaxResponseBytes {
				return malformed("handshake message of %d bytes", n)
			}
			if len(hs) < 4+n {
				break
			}
			typ, body := hs[0], hs[4:4+n]
			hs = hs[4+n:]
			switch typ {
			case typeEncryptedExtensions, typeCertificateReq:
			case typeCertificate:
				if f.Chain, err = ParseTLS13Certificate(body); err != nil {
					return err
				}
			case typeCertificateVerify:
				s := cryptobyte.String(body)
				if !s.ReadUint16(&f.SignatureScheme) {
					return malformed("short CertificateVerify")
				}
				return nil
			case typeFinished:
				return nil
			default:
				return malformed("unexpected handshake message %d", typ)
			}
		}
	}
}

// ParseTLS13Certificate parses the body of a TLS 1.3 Certificate message
// (RFC 8446, 4.4.2) and returns the certificates, leaf first.
func ParseTLS13Certificate(body []byte) ([][]byte, error) {
	s := cryptobyte.String(body)
	var context, list cryptobyte.String
	if !s.ReadUint8LengthPrefixed(&context) || !s.ReadUint24LengthPrefixed(&list) || !s.Empty() {
		return nil, malformed("bad Certificate message")
	}
	var chain [][]byte
	for !list.Empty() {
		var cert, exts cryptobyte.String
		if !list.ReadUint24LengthPrefixed(&cert) || cert.Empty() || !list.ReadUint16LengthPrefixed(&exts) {
			return nil, malformed("bad CertificateEntry")
		}
		chain = append(chain, slices.Clone([]byte(cert)))
	}
	return chain, nil
}
