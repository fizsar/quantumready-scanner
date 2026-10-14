package rawtls

import (
	"crypto/tls"
	"strings"
	"testing"
)

func TestSuiteRegistry(t *testing.T) {
	if n := len(suites); n < 330 {
		t.Fatalf("only %d suites parsed", n)
	}
	for _, tc := range []struct {
		id                    uint16
		kx, auth, cipher, mac string
		bits                  int
		mode                  Mode
		export, tls13, offer  bool
		minVersion            uint16
	}{
		{0x0000, "NULL", "NULL", "NULL", "NULL", 0, ModeNone, false, false, true, VersionSSL30},
		{0x0001, "RSA", "RSA", "NULL", "MD5", 0, ModeNone, false, false, true, VersionSSL30},
		{0x0003, "RSA", "RSA", "RC4_40", "MD5", 40, ModeStream, true, false, true, VersionSSL30},
		{0x0011, "DHE", "DSS", "DES40_CBC", "SHA", 40, ModeCBC, true, false, true, VersionSSL30},
		{0x0016, "DHE", "RSA", "3DES_EDE_CBC", "SHA", 112, ModeCBC, false, false, true, VersionSSL30},
		{0x0018, "DHE", "anon", "RC4_128", "MD5", 128, ModeStream, false, false, true, VersionSSL30},
		{0x0062, "RSA", "RSA", "DES_CBC", "SHA", 56, ModeCBC, true, false, true, VersionSSL30}, // EXPORT1024, not IANA
		{0x0084, "RSA", "RSA", "CAMELLIA_256_CBC", "SHA", 256, ModeCBC, false, false, true, VersionSSL30},
		{0x0096, "RSA", "RSA", "SEED_CBC", "SHA", 128, ModeCBC, false, false, true, VersionSSL30},
		{0x009E, "DHE", "RSA", "AES_128_GCM", "SHA256", 128, ModeGCM, false, false, true, VersionTLS12},
		{0x008C, "PSK", "PSK", "AES_128_CBC", "SHA", 128, ModeCBC, false, false, false, VersionSSL30},
		{0xC009, "ECDHE", "ECDSA", "AES_128_CBC", "SHA", 128, ModeCBC, false, false, true, VersionSSL30},
		{0xC018, "ECDHE", "anon", "AES_128_CBC", "SHA", 128, ModeCBC, false, false, true, VersionSSL30},
		{0xC024, "ECDHE", "ECDSA", "AES_256_CBC", "SHA384", 256, ModeCBC, false, false, true, VersionTLS12},
		{0xC051, "RSA", "RSA", "ARIA_256_GCM", "SHA384", 256, ModeGCM, false, false, true, VersionTLS12},
		{0xC09C, "RSA", "RSA", "AES_128_CCM", "", 128, ModeCCM, false, false, true, VersionTLS12},
		{0xC0A0, "RSA", "RSA", "AES_128_CCM_8", "", 128, ModeCCM8, false, false, true, VersionTLS12},
		{0xCCA9, "ECDHE", "ECDSA", "CHACHA20_POLY1305", "SHA256", 256, ModeChaCha, false, false, true, VersionTLS12},
		{0x1301, "", "", "AES_128_GCM", "SHA256", 128, ModeGCM, false, true, true, VersionTLS13},
		{0x1305, "", "", "AES_128_CCM_8", "SHA256", 128, ModeCCM8, false, true, true, VersionTLS13},
		{0x1306, "", "", "AEGIS_256", "SHA512", 256, ModeAEAD, false, true, true, VersionTLS13},
		{0x00C6, "", "", "SM4_GCM", "SM3", 128, ModeGCM, false, true, true, VersionTLS13},
		{0x0070, "", "", "AES_128_GCM", "ASCONHASH256", 128, ModeGCM, false, true, true, VersionTLS13},
		{0xC0B4, "", "", "NULL", "SHA256", 0, ModeNone, false, true, true, VersionTLS13}, // RFC 9150 integrity only
		{0xC100, "GOST", "GOST", "KUZNYECHIK_CTR", "OMAC", 256, ModeOther, false, false, false, VersionSSL30},
		{0xC103, "", "", "KUZNYECHIK_MGM_L", "", 256, ModeAEAD, false, true, false, VersionTLS13},
	} {
		s, ok := SuiteByID(tc.id)
		if !ok {
			t.Errorf("0x%04x missing", tc.id)
			continue
		}
		if s.Kx != tc.kx || s.Auth != tc.auth || s.Cipher != tc.cipher || s.MAC != tc.mac || s.Bits != tc.bits ||
			s.Mode != tc.mode || s.Export != tc.export || s.TLS13 != tc.tls13 || s.Offerable != tc.offer || s.MinVersion() != tc.minVersion {
			t.Errorf("%s = %+v (min %s), want %+v", s.Name, s, VersionName(s.MinVersion()), tc)
		}
	}
}

func TestSuiteRegistryAgreesWithCryptoTLS(t *testing.T) {
	for _, list := range [][]*tls.CipherSuite{tls.CipherSuites(), tls.InsecureCipherSuites()} {
		for _, cs := range list {
			s, ok := SuiteByID(cs.ID)
			if !ok {
				t.Errorf("%s (0x%04x) missing", cs.Name, cs.ID)
				continue
			}
			if s.Name != cs.Name {
				t.Errorf("0x%04x: name %s, crypto/tls says %s", cs.ID, s.Name, cs.Name)
			}
			if s.TLS13 != (cs.SupportedVersions[0] == tls.VersionTLS13) {
				t.Errorf("%s: TLS13 = %v", s.Name, s.TLS13)
			}
			fs := strings.HasPrefix(cs.Name, "TLS_ECDHE_") || s.TLS13
			if s.ForwardSecrecy() != fs {
				t.Errorf("%s: ForwardSecrecy = %v", s.Name, s.ForwardSecrecy())
			}
		}
	}
}

func TestSuiteRegistryInvariants(t *testing.T) {
	for _, s := range suites {
		if s.Export && s.Bits > 56 {
			t.Errorf("%s: export suite with %d bits", s.Name, s.Bits)
		}
		if s.TLS13 && (s.Kx != "" || s.Export) {
			t.Errorf("%s: TLS 1.3 suite with key exchange %q", s.Name, s.Kx)
		}
		if s.Anonymous() && s.Offerable && !s.ForwardSecrecy() && s.Kx != "NULL" {
			t.Errorf("%s: anonymous static key exchange?", s.Name)
		}
		if s.MinVersion() > s.MaxVersion() {
			t.Errorf("%s: versions %x > %x", s.Name, s.MinVersion(), s.MaxVersion())
		}
	}
	if _, ok := SuiteByID(0x00FF); ok {
		t.Error("signalling value TLS_EMPTY_RENEGOTIATION_INFO_SCSV must not be a suite")
	}
	if _, ok := SuiteByID(0x0A0A); ok {
		t.Error("GREASE value must not be a suite")
	}
	if got := SuiteName(0xFFFE); got != "0xfffe" {
		t.Errorf("SuiteName(unknown) = %s", got)
	}
}

func TestGroupRegistry(t *testing.T) {
	for _, tc := range []struct {
		id           uint16
		name         string
		kind         GroupKind
		tls13, tls12 bool
	}{
		{0x0001, "sect163k1", GroupBinary, false, true},
		{0x0015, "secp224r1", GroupECDHEWeak, false, true},
		{0x0017, "secp256r1", GroupECDHE, true, true},
		{0x001a, "brainpoolP256r1", GroupBrainpool, false, true},
		{0x001d, "x25519", GroupECDHE, true, true},
		{0x001f, "brainpoolP256r1tls13", GroupBrainpool, true, false},
		{0x0100, "ffdhe2048", GroupFFDHE, true, false},
		{0x0201, "MLKEM768", GroupPureMLKEM, true, false},
		{0x11e9, "SecP256r1MLKEM512", GroupHybridDraft, true, false},
		{0x11ec, "X25519MLKEM768", GroupHybrid, true, false},
		{0x11ed, "SecP384r1MLKEM1024", GroupHybrid, true, false},
		{0x6399, "X25519Kyber768Draft00", GroupKyberDraft, true, false},
		{0x639a, "SecP256r1Kyber768Draft00", GroupKyberDraft, true, false},
		{0x0022, "GC256A", GroupOther, true, false},
	} {
		g, ok := GroupByID(tc.id)
		if !ok || g.Name != tc.name || g.Kind != tc.kind || g.TLS13 != tc.tls13 || g.TLS12 != tc.tls12 {
			t.Errorf("0x%04x = %+v (found %v), want %+v", tc.id, g, ok, tc)
		}
	}
	// The groups crypto/tls knows must agree.
	for _, id := range []tls.CurveID{tls.X25519, tls.CurveP256, tls.CurveP384, tls.CurveP521, tls.X25519MLKEM768, tls.SecP256r1MLKEM768, tls.SecP384r1MLKEM1024} {
		if _, ok := GroupByID(uint16(id)); !ok {
			t.Errorf("%v missing", id)
		}
	}
	for _, g := range groups {
		if g.PQC() && !g.TLS13 {
			t.Errorf("%s: PQC group must be TLS 1.3", g.Name)
		}
	}
}
