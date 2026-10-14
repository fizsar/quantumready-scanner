package rawtls

import (
	"crypto/ecdh"
	"crypto/rand"
	"errors"
	"fmt"
	"slices"

	"golang.org/x/crypto/cryptobyte"
)

// Extension and message numbers used here.
const (
	extServerName          uint16 = 0
	extSupportedGroups     uint16 = 10
	extECPointFormats      uint16 = 11
	extSignatureAlgorithms uint16 = 13
	extPadding             uint16 = 21
	extSupportedVersions   uint16 = 43
	extKeyShare            uint16 = 51

	typeClientHello       uint8 = 1
	typeServerHello       uint8 = 2
	typeCertificate       uint8 = 11
	typeServerKeyExchange uint8 = 12
	typeCertificateReq    uint8 = 13
	typeServerHelloDone   uint8 = 14
	typeCertificateStatus uint8 = 22

	recordChangeCipherSpec uint8 = 20
	recordAlert            uint8 = 21
	recordHandshake        uint8 = 22

	// SCSVRenegotiation (TLS_EMPTY_RENEGOTIATION_INFO_SCSV) is added to every
	// TLS ClientHello: it signals secure renegotiation without an extension,
	// which old servers handle better.
	SCSVRenegotiation uint16 = 0x00FF

	maxPlaintext = 1 << 14 // TLS record payload limit
)

// DefaultSignatureSchemes is a broad signature_algorithms list, legacy SHA-1
// and DSA included, so that old servers find something to sign with: the
// point is to learn what they accept, not to trust them.
var DefaultSignatureSchemes = []uint16{
	0x0403, 0x0503, 0x0603, // ecdsa_secp256r1_sha256, secp384r1_sha384, secp521r1_sha512
	0x0807, 0x0808, // ed25519, ed448
	0x0804, 0x0805, 0x0806, // rsa_pss_rsae_sha256/384/512
	0x0809, 0x080a, 0x080b, // rsa_pss_pss_sha256/384/512
	0x0401, 0x0501, 0x0601, // rsa_pkcs1_sha256/384/512
	0x081a, 0x081b, 0x081c, // ecdsa_brainpool*tls13
	0x0904, 0x0905, 0x0906, // mldsa44/65/87
	0x0301, 0x0302, 0x0303, // sha224 rsa/dsa/ecdsa
	0x0402, 0x0502, 0x0602, // dsa_sha256/384/512
	0x0201, 0x0202, 0x0203, // rsa_pkcs1_sha1, dsa_sha1, ecdsa_sha1
}

// KeyShare is one entry of the TLS 1.3 key_share extension.
type KeyShare struct {
	Group uint16
	Data  []byte
}

// X25519KeyShare returns a fresh x25519 key share. The private key is thrown
// away: we never get as far as deriving secrets.
func X25519KeyShare() (KeyShare, error) {
	k, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return KeyShare{}, err
	}
	return KeyShare{Group: 0x001d, Data: k.PublicKey().Bytes()}, nil
}

// Hello is a ClientHello to send. Everything the server may pick from must be
// listed here: Exchange checks the answer against it.
type Hello struct {
	// Version is the ClientHello legacy_version: VersionSSL30…VersionTLS12.
	// Without SupportedVersions, a server may answer with any version from
	// SSLv3 up to this one.
	Version uint16
	// SupportedVersions, when set, sends the supported_versions extension
	// (TLS 1.3) and Version must be VersionTLS12.
	SupportedVersions []uint16
	Suites            []uint16
	ServerName        string   // SNI; "" for IP literals
	Groups            []uint16 // supported_groups; nil omits the extension
	// KeyShares is sent when SupportedVersions includes TLS 1.3, even if
	// empty: an empty list makes a server that supports one of Groups answer
	// with a HelloRetryRequest naming it (RFC 8446, section 4.2.8).
	KeyShares        []KeyShare
	SignatureSchemes []uint16 // nil: DefaultSignatureSchemes (TLS 1.2 and up)
	// NoExtensions sends a bare ClientHello, as SSLv3 clients did.
	NoExtensions bool
}

// offersTLS13 reports whether the hello offers TLS 1.3.
func (h *Hello) offersTLS13() bool { return slices.Contains(h.SupportedVersions, VersionTLS13) }

// Marshal returns the ClientHello as TLS records, ready to write.
func (h *Hello) Marshal() ([]byte, error) {
	if h.Version < VersionSSL30 || h.Version > VersionTLS12 {
		return nil, fmt.Errorf("rawtls: legacy_version %s not allowed", VersionName(h.Version))
	}
	if len(h.SupportedVersions) > 0 && h.Version != VersionTLS12 {
		return nil, errors.New("rawtls: supported_versions requires legacy_version TLS 1.2")
	}
	if len(h.Suites) == 0 || len(h.Suites) > 0x7fff-1 {
		return nil, errors.New("rawtls: suite list empty or too long")
	}
	var random [32]byte
	if _, err := rand.Read(random[:]); err != nil {
		return nil, err
	}
	var sessionID []byte
	if h.offersTLS13() { // middlebox compatibility mode, as browsers do
		sessionID = make([]byte, 32)
		if _, err := rand.Read(sessionID); err != nil {
			return nil, err
		}
	}

	body, err := h.marshalBody(random, sessionID, nil)
	if err != nil {
		return nil, err
	}
	// Some TLS terminators hang on ClientHellos of 256–511 bytes; OpenSSL,
	// NSS and testssl.sh pad those to 512 (RFC 7685).
	if n := len(body) + 4; n > 0xff && n < 0x200 && !h.NoExtensions {
		pad := 0x200 - n
		if pad >= 4 {
			pad -= 4
		} else {
			pad = 0
		}
		if body, err = h.marshalBody(random, sessionID, make([]byte, pad)); err != nil {
			return nil, err
		}
	}
	if len(body) > 0xffffff {
		return nil, errors.New("rawtls: ClientHello too long")
	}

	var msg cryptobyte.Builder
	msg.AddUint8(typeClientHello)
	msg.AddUint24LengthPrefixed(func(b *cryptobyte.Builder) { b.AddBytes(body) })
	hs, err := msg.Bytes()
	if err != nil {
		return nil, err
	}
	recVersion := VersionTLS10 // what browsers and OpenSSL put in the first record
	if h.Version == VersionSSL30 {
		recVersion = VersionSSL30
	}
	var out []byte
	for len(hs) > 0 {
		n := min(len(hs), maxPlaintext)
		var rec cryptobyte.Builder
		rec.AddUint8(recordHandshake)
		rec.AddUint16(recVersion)
		rec.AddUint16LengthPrefixed(func(b *cryptobyte.Builder) { b.AddBytes(hs[:n]) })
		r, err := rec.Bytes()
		if err != nil {
			return nil, err
		}
		out = append(out, r...)
		hs = hs[n:]
	}
	return out, nil
}

func (h *Hello) marshalBody(random [32]byte, sessionID, padding []byte) ([]byte, error) {
	var b cryptobyte.Builder
	b.AddUint16(h.Version)
	b.AddBytes(random[:])
	b.AddUint8LengthPrefixed(func(b *cryptobyte.Builder) { b.AddBytes(sessionID) })
	b.AddUint16LengthPrefixed(func(b *cryptobyte.Builder) {
		for _, s := range h.Suites {
			b.AddUint16(s)
		}
		if !h.NoExtensions && !slices.Contains(h.Suites, SCSVRenegotiation) {
			b.AddUint16(SCSVRenegotiation)
		}
	})
	b.AddUint8LengthPrefixed(func(b *cryptobyte.Builder) { b.AddUint8(0) }) // null compression only
	if h.NoExtensions {
		return b.Bytes()
	}
	b.AddUint16LengthPrefixed(func(b *cryptobyte.Builder) {
		if h.ServerName != "" {
			ext(b, extServerName, func(b *cryptobyte.Builder) {
				b.AddUint16LengthPrefixed(func(b *cryptobyte.Builder) {
					b.AddUint8(0) // host_name
					b.AddUint16LengthPrefixed(func(b *cryptobyte.Builder) { b.AddBytes([]byte(h.ServerName)) })
				})
			})
		}
		if len(h.Groups) > 0 {
			ext(b, extSupportedGroups, func(b *cryptobyte.Builder) {
				b.AddUint16LengthPrefixed(func(b *cryptobyte.Builder) {
					for _, g := range h.Groups {
						b.AddUint16(g)
					}
				})
			})
			ext(b, extECPointFormats, func(b *cryptobyte.Builder) {
				b.AddUint8LengthPrefixed(func(b *cryptobyte.Builder) { b.AddUint8(0) }) // uncompressed
			})
		}
		if h.Version >= VersionTLS12 {
			schemes := h.SignatureSchemes
			if schemes == nil {
				schemes = DefaultSignatureSchemes
			}
			ext(b, extSignatureAlgorithms, func(b *cryptobyte.Builder) {
				b.AddUint16LengthPrefixed(func(b *cryptobyte.Builder) {
					for _, s := range schemes {
						b.AddUint16(s)
					}
				})
			})
		}
		if len(h.SupportedVersions) > 0 {
			ext(b, extSupportedVersions, func(b *cryptobyte.Builder) {
				b.AddUint8LengthPrefixed(func(b *cryptobyte.Builder) {
					for _, v := range h.SupportedVersions {
						b.AddUint16(v)
					}
				})
			})
		}
		if h.offersTLS13() {
			ext(b, extKeyShare, func(b *cryptobyte.Builder) {
				b.AddUint16LengthPrefixed(func(b *cryptobyte.Builder) {
					for _, ks := range h.KeyShares {
						b.AddUint16(ks.Group)
						b.AddUint16LengthPrefixed(func(b *cryptobyte.Builder) { b.AddBytes(ks.Data) })
					}
				})
			})
		}
		if padding != nil {
			ext(b, extPadding, func(b *cryptobyte.Builder) { b.AddBytes(padding) })
		}
	})
	return b.Bytes() // errors on a length overflow (a huge key share or name)
}

func ext(b *cryptobyte.Builder, typ uint16, data func(*cryptobyte.Builder)) {
	b.AddUint16(typ)
	b.AddUint16LengthPrefixed(data)
}
