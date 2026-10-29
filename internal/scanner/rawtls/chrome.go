package rawtls

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"

	"golang.org/x/crypto/cryptobyte"
)

// Chrome 156's ClientHello (stable, captured on 26/10/2026 against the lab:
// testdata/clienthello/chrome156.hex). ChromeHello rebuilds it extension by
// extension, in the captured order and with the captured GREASE values; only
// what Chrome itself draws at random changes from one connection to the next
// (random, session_id, key shares, the GREASE ECH payload). Chrome also
// shuffles its extensions on every connection; a server must not care, and
// the order here is the captured one.
var (
	// ChromeSuites: TLS 1.3, then ECDHE (ECDSA before RSA), then RSA key exchange.
	ChromeSuites = []uint16{0x1301, 0x1302, 0x1303, 0xc02b, 0xc02f, 0xc02c, 0xc030, 0xcca9, 0xcca8, 0xc013, 0xc014, 0x009c, 0x009d, 0x002f, 0x0035}
	// ChromeGroups: X25519MLKEM768, x25519, P-256, P-384; key shares for the first two.
	ChromeGroups = []uint16{0x11ec, 0x001d, 0x0017, 0x0018}
	// ChromeSignatureSchemes: ML-DSA-44/65/87, then ECDSA before RSA-PSS
	// and PKCS#1; no Ed25519, no SHA-1.
	ChromeSignatureSchemes = []uint16{0x0904, 0x0905, 0x0906, 0x0403, 0x0804, 0x0401, 0x0503, 0x0805, 0x0501, 0x0806, 0x0601}
	ChromeALPN             = []string{"h2", "http/1.1"}
)

// GREASE values (RFC 8701) as captured.
const (
	chromeGreaseSuite    = 0x6a6a
	chromeGreaseFirstExt = 0xaaaa
	chromeGreaseSig      = 0x5a5a
	chromeGreaseGroup    = 0x7a7a // also in key_share and supported_versions
	chromeGreaseLastExt  = 0xbaba
)

// Extensions Chrome sends that the rest of this package does not.
const (
	extStatusRequest        uint16 = 5
	extSCT                  uint16 = 18
	extExtendedMasterSecret uint16 = 23
	extCompressCertificate  uint16 = 27
	extSessionTicket        uint16 = 35
	extPSKModes             uint16 = 45
	extChrome12E0           uint16 = 0x12e0 // two zero bytes; not in the IANA registry
	extALPS                 uint16 = 0x44cd // application_settings (new codepoint)
	extTrustAnchors         uint16 = 0xca34 // TLS Trust Anchor IDs (draft)
	extECH                  uint16 = 0xfe0d // encrypted_client_hello (GREASE)
	extRenegotiationInfo    uint16 = 0xff01
)

// chromeTrustAnchors is the trust_anchors extension as Chrome 156 sent it.
var chromeTrustAnchors = mustHex("00b80582df1302010582df1302060582df13020d0582df13020e0582df13020f0582df1302120582df1302130582df13" +
	"021408839a648c9b2d010708839a648c9b2d010808839a648c9b2d010908839a648c9b2d010a08839a648c9b2d010b08" +
	"839a648c9b2d010c08839a648c9b2d010d08839a648c9b2d011208839a648c9b2d011304d679090104d679090404d679" +
	"090504d679090604d679090704d679090804d679090a04d679090b04d679090c04d679090d04d679090f")

func mustHex(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}

// chromeECHPayload is the size of the GREASE ECH payload captured.
const chromeECHPayload = 240

// ChromeHello returns a ClientHello as Chrome 156 sends it to serverName ("":
// no SNI, as for an IP literal), as TLS records, and the Hello describing
// the offer, to check the server's answer against (Exchange does).
func ChromeHello(serverName string) ([]byte, *Hello, error) {
	random := make([]byte, 32+32+32+chromeECHPayload+1) // random, session_id, ECH enc, ECH payload, ECH config_id
	if _, err := rand.Read(random); err != nil {
		return nil, nil, err
	}
	var shares []KeyShare
	for _, g := range ChromeGroups[:2] {
		kx, err := newKeyExchange(g)
		if err != nil {
			return nil, nil, err
		}
		shares = append(shares, kx.share)
	}
	h := &Hello{Version: VersionTLS12, SupportedVersions: []uint16{VersionTLS13, VersionTLS12}, Suites: ChromeSuites,
		ServerName: serverName, Groups: ChromeGroups, KeyShares: shares, SignatureSchemes: ChromeSignatureSchemes}

	u16s := func(b *cryptobyte.Builder, vs ...uint16) {
		for _, v := range vs {
			b.AddUint16(v)
		}
	}
	empty := func(*cryptobyte.Builder) {}
	var body cryptobyte.Builder
	body.AddUint16(VersionTLS12)
	body.AddBytes(random[:32])
	body.AddUint8LengthPrefixed(func(b *cryptobyte.Builder) { b.AddBytes(random[32:64]) })
	body.AddUint16LengthPrefixed(func(b *cryptobyte.Builder) { u16s(b, chromeGreaseSuite); u16s(b, ChromeSuites...) })
	body.AddUint8LengthPrefixed(func(b *cryptobyte.Builder) { b.AddUint8(0) })
	body.AddUint16LengthPrefixed(func(b *cryptobyte.Builder) {
		ext(b, chromeGreaseFirstExt, empty)
		ext(b, extSignatureAlgorithms, func(b *cryptobyte.Builder) {
			b.AddUint16LengthPrefixed(func(b *cryptobyte.Builder) { u16s(b, chromeGreaseSig); u16s(b, ChromeSignatureSchemes...) })
		})
		ext(b, extSCT, empty)
		ext(b, extChrome12E0, func(b *cryptobyte.Builder) { b.AddUint16(0) })
		if serverName != "" {
			ext(b, extServerName, func(b *cryptobyte.Builder) {
				b.AddUint16LengthPrefixed(func(b *cryptobyte.Builder) {
					b.AddUint8(0)
					b.AddUint16LengthPrefixed(func(b *cryptobyte.Builder) { b.AddBytes([]byte(serverName)) })
				})
			})
		}
		ext(b, extRenegotiationInfo, func(b *cryptobyte.Builder) { b.AddUint8(0) })
		ext(b, extTrustAnchors, func(b *cryptobyte.Builder) { b.AddBytes(chromeTrustAnchors) })
		ext(b, extSessionTicket, empty)
		ext(b, extKeyShare, func(b *cryptobyte.Builder) {
			b.AddUint16LengthPrefixed(func(b *cryptobyte.Builder) {
				b.AddUint16(chromeGreaseGroup)
				b.AddUint16LengthPrefixed(func(b *cryptobyte.Builder) { b.AddUint8(0) })
				for _, ks := range shares {
					b.AddUint16(ks.Group)
					b.AddUint16LengthPrefixed(func(b *cryptobyte.Builder) { b.AddBytes(ks.Data) })
				}
			})
		})
		ext(b, extPSKModes, func(b *cryptobyte.Builder) {
			b.AddUint8LengthPrefixed(func(b *cryptobyte.Builder) { b.AddUint8(1) }) // psk_dhe_ke
		})
		ext(b, extALPS, func(b *cryptobyte.Builder) {
			b.AddUint16LengthPrefixed(func(b *cryptobyte.Builder) {
				b.AddUint8LengthPrefixed(func(b *cryptobyte.Builder) { b.AddBytes([]byte("h2")) })
			})
		})
		ext(b, extSupportedGroups, func(b *cryptobyte.Builder) {
			b.AddUint16LengthPrefixed(func(b *cryptobyte.Builder) { u16s(b, chromeGreaseGroup); u16s(b, ChromeGroups...) })
		})
		ext(b, extSupportedVersions, func(b *cryptobyte.Builder) {
			b.AddUint8LengthPrefixed(func(b *cryptobyte.Builder) { u16s(b, chromeGreaseGroup, VersionTLS13, VersionTLS12) })
		})
		ext(b, extExtendedMasterSecret, empty)
		ext(b, extECH, func(b *cryptobyte.Builder) {
			b.AddUint8(0)                     // outer ClientHello
			b.AddUint16(0x0001)               // HKDF-SHA256
			b.AddUint16(0x0001)               // AES-128-GCM
			b.AddUint8(random[len(random)-1]) // config_id
			b.AddUint16LengthPrefixed(func(b *cryptobyte.Builder) { b.AddBytes(random[64:96]) })
			b.AddUint16LengthPrefixed(func(b *cryptobyte.Builder) { b.AddBytes(random[96 : 96+chromeECHPayload]) })
		})
		ext(b, extStatusRequest, func(b *cryptobyte.Builder) { b.AddBytes([]byte{1, 0, 0, 0, 0}) }) // OCSP, no ids or extensions
		ext(b, extALPN, func(b *cryptobyte.Builder) {
			b.AddUint16LengthPrefixed(func(b *cryptobyte.Builder) {
				for _, p := range ChromeALPN {
					b.AddUint8LengthPrefixed(func(b *cryptobyte.Builder) { b.AddBytes([]byte(p)) })
				}
			})
		})
		ext(b, extECPointFormats, func(b *cryptobyte.Builder) {
			b.AddUint8LengthPrefixed(func(b *cryptobyte.Builder) { b.AddUint8(0) })
		})
		ext(b, extCompressCertificate, func(b *cryptobyte.Builder) {
			b.AddUint8LengthPrefixed(func(b *cryptobyte.Builder) { b.AddUint16(2) }) // brotli
		})
		ext(b, chromeGreaseLastExt, func(b *cryptobyte.Builder) { b.AddUint8(0) })
	})
	bb, err := body.Bytes()
	if err != nil {
		return nil, nil, err
	}
	var rec cryptobyte.Builder
	rec.AddUint8(recordHandshake)
	rec.AddUint16(VersionTLS10)
	rec.AddUint16LengthPrefixed(func(b *cryptobyte.Builder) {
		b.AddUint8(typeClientHello)
		b.AddUint24LengthPrefixed(func(b *cryptobyte.Builder) { b.AddBytes(bb) })
	})
	out, err := rec.Bytes()
	if err != nil || len(out) > 5+maxPlaintext {
		return nil, nil, errors.Join(errors.New("rawtls: Chrome ClientHello does not fit a record"), err)
	}
	return out, h, nil
}

// ExchangeChrome sends Chrome 156's ClientHello and reads the answer as
// Exchange does (the ServerHello; in TLS 1.2, with opts.ReadKeyExchange, up
// to the ServerKeyExchange, for the curve).
func ExchangeChrome(conn io.ReadWriter, serverName string, opts Options) (*Response, error) {
	msg, h, err := ChromeHello(serverName)
	if err != nil {
		return nil, err
	}
	if _, err := conn.Write(msg); err != nil {
		return nil, err
	}
	return readResponse(conn, h, opts)
}
