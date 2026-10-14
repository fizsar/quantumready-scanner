package rawtls

import (
	"bytes"
	"errors"
	"testing"

	"golang.org/x/crypto/cryptobyte"
)

func allSSLv2() []uint32 {
	var ids []uint32
	for _, c := range SSLv2Ciphers {
		ids = append(ids, c.ID)
	}
	return ids
}

func TestMarshalSSLv2Hello(t *testing.T) {
	wire, err := MarshalSSLv2Hello(allSSLv2())
	if err != nil {
		t.Fatal(err)
	}
	n := 9 + 3*len(SSLv2Ciphers) + 16
	if wire[0] != 0x80|byte(n>>8) || wire[1] != byte(n) || len(wire) != 2+n {
		t.Fatalf("header %x, length %d", wire[:2], len(wire))
	}
	want := []byte{ssl2MsgClientHello, 0, 2, 0, byte(3 * len(SSLv2Ciphers)), 0, 0, 0, 16, 0x01, 0x00, 0x80}
	if !bytes.Equal(wire[2:14], want) {
		t.Errorf("message starts %x, want %x", wire[2:14], want)
	}
	if _, err := MarshalSSLv2Hello(nil); err == nil {
		t.Error("empty cipher list accepted")
	}
}

// ssl2ServerHello builds an SSLv2 SERVER-HELLO record.
func ssl2ServerHello(ciphers []uint32, connID int, threeByteHeader bool) []byte {
	var b cryptobyte.Builder
	b.AddUint8(ssl2MsgServerHello)
	b.AddUint8(0)
	b.AddUint8(1) // X.509
	b.AddUint16(VersionSSL20)
	b.AddUint16(5)
	b.AddUint16(uint16(3 * len(ciphers)))
	b.AddUint16(uint16(connID))
	b.AddBytes([]byte{1, 2, 3, 4, 5})
	for _, c := range ciphers {
		b.AddUint24(c)
	}
	b.AddBytes(make([]byte, connID))
	msg := b.BytesOrPanic()
	if threeByteHeader {
		msg = append(msg, 0, 0) // two bytes of padding
		return append([]byte{byte(len(msg) >> 8), byte(len(msg)), 2}, msg...)
	}
	return append([]byte{0x80 | byte(len(msg)>>8), byte(len(msg))}, msg...)
}

func TestReadSSLv2Response(t *testing.T) {
	offer := allSSLv2()
	for _, tc := range []struct {
		name    string
		stream  []byte
		check   func(*SSLv2Response) bool
		wantErr error
	}{
		{"SERVER-HELLO", ssl2ServerHello([]uint32{0x010080, 0x0700C0}, 16, false),
			func(r *SSLv2Response) bool {
				return r.ServerHello.Version == VersionSSL20 && len(r.ServerHello.Ciphers) == 2 && r.ServerHello.CertLen == 5
			}, nil},
		{"SERVER-HELLO, 3-byte header with padding", ssl2ServerHello([]uint32{0x060040}, 32, true),
			func(r *SSLv2Response) bool { return r.ServerHello.Ciphers[0] == 0x060040 }, nil},
		{"SSLv2 enabled without a common cipher", ssl2ServerHello(nil, 16, false),
			func(r *SSLv2Response) bool { return r.ServerHello != nil && len(r.ServerHello.Ciphers) == 0 }, nil},
		{"SSLv2 ERROR", []byte{0x80, 3, ssl2MsgError, 0, 1},
			func(r *SSLv2Response) bool { return *r.ErrorCode == 1 }, nil},
		{"TLS alert", record(recordAlert, []byte{2, 70}),
			func(r *SSLv2Response) bool { return r.TLSAlert.Description == 70 }, nil},
		{"TLS ServerHello", record(recordHandshake, hsMsg(typeServerHello, serverHelloBody(shOpts{version: VersionTLS10, suite: 0x000A, noExtBlock: true}))),
			func(r *SSLv2Response) bool { return r.TLSVersion == VersionTLS10 && r.ServerHello == nil }, nil},

		{"cipher not offered", ssl2ServerHello([]uint32{0x080080}, 16, false), nil, ErrUnoffered},
		{"connection id too short", ssl2ServerHello([]uint32{0x010080}, 8, false), nil, ErrMalformed},
		{"cipher list not a multiple of 3", []byte{0x80, 13, 4, 0, 1, 0, 2, 0, 0, 0, 1, 0, 16, 1, 2}, nil, ErrMalformed},
		{"lengths beyond the record", []byte{0x80, 11, 4, 0, 1, 0, 2, 0xFF, 0xFF, 0, 0, 0, 16}, nil, ErrMalformed},
		{"padding swallows the record", []byte{0x00, 2, 2, 4, 0}, nil, ErrMalformed},
		{"empty record", []byte{0x80, 0}, nil, ErrMalformed},
		{"unknown message type", []byte{0x80, 1, 9}, nil, ErrMalformed},
		{"ERROR of 2 bytes", []byte{0x80, 2, 0, 1}, nil, ErrMalformed},
		{"truncated", []byte{0x80, 20, 4, 0}, nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := readSSLv2Response(bytes.NewReader(tc.stream), offer)
			if tc.check == nil {
				if err == nil || tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !tc.check(resp) {
				t.Errorf("response = %+v (ServerHello %+v)", resp, resp.ServerHello)
			}
		})
	}
}
