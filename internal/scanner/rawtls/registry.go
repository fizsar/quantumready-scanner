package rawtls

import (
	"bytes"
	_ "embed"
	"encoding/binary"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Copies of the IANA registries "TLS Cipher Suites" and "TLS Supported
// Groups" (https://www.iana.org/assignments/tls-parameters, updated
// 2026-09-18). To update: replace the files and run the tests.
var (
	//go:embed iana/tls-parameters-4.csv
	suitesCSV []byte
	//go:embed iana/tls-parameters-8.csv
	groupsCSV []byte
)

// Protocol versions.
const (
	VersionSSL20 uint16 = 0x0002
	VersionSSL30 uint16 = 0x0300
	VersionTLS10 uint16 = 0x0301
	VersionTLS11 uint16 = 0x0302
	VersionTLS12 uint16 = 0x0303
	VersionTLS13 uint16 = 0x0304
)

// VersionName returns "SSLv3", "TLS 1.2"… (the names crypto/tls uses).
func VersionName(v uint16) string {
	switch v {
	case VersionSSL20:
		return "SSLv2"
	case VersionSSL30:
		return "SSLv3"
	case VersionTLS10:
		return "TLS 1.0"
	case VersionTLS11:
		return "TLS 1.1"
	case VersionTLS12:
		return "TLS 1.2"
	case VersionTLS13:
		return "TLS 1.3"
	}
	return fmt.Sprintf("0x%04x", v)
}

// Mode is how a suite protects the records.
type Mode string

// Cipher modes.
const (
	ModeNone   Mode = "none"   // NULL encryption, or integrity-only TLS 1.3 suites
	ModeStream Mode = "stream" // RC4
	ModeCBC    Mode = "cbc"
	ModeGCM    Mode = "gcm"
	ModeCCM    Mode = "ccm"
	ModeCCM8   Mode = "ccm8" // CCM with a 64-bit tag
	ModeChaCha Mode = "chacha20_poly1305"
	ModeAEAD   Mode = "aead"  // other AEADs: AEGIS, Ascon, MGM
	ModeOther  Mode = "other" // GOST CTR/CNT with OMAC/IMIT
)

// Suite describes an IANA cipher suite (plus the few non-IANA ones old
// OpenSSL releases still negotiate). Attributes are derived from the name.
type Suite struct {
	ID   uint16
	Name string
	// Kx is the key exchange: RSA, DH, DHE, ECDH, ECDHE, PSK, DHE_PSK,
	// RSA_PSK, ECDHE_PSK, SRP, KRB5, ECCPWD, GOST or NULL; "" for TLS 1.3.
	Kx string
	// Auth is the authentication: RSA, DSS, ECDSA, anon, PSK, SRP, KRB5,
	// ECCPWD, GOST or NULL; "" for TLS 1.3.
	Auth   string
	Cipher string // e.g. AES_128_GCM, 3DES_EDE_CBC, RC4_40, NULL
	Bits   int    // symmetric key strength in bits (3DES: 112, NULL: 0)
	Mode   Mode
	MAC    string // MD5, SHA, SHA256, SHA384, SM3…; "" for CCM in TLS 1.2
	Export bool
	TLS13  bool
	// Recommended is the IANA column: 'Y', 'N', 'D' (discouraged), or 0
	// for suites outside the registry.
	Recommended byte
	// Offerable is false for suites that need credentials or algorithms we do
	// not have (PSK, SRP, Kerberos, ECCPWD, GOST): a server cannot pick them
	// for an anonymous client, so they cannot be tested this way.
	Offerable bool
}

// ForwardSecrecy reports an ephemeral key exchange (DHE, ECDHE or TLS 1.3).
func (s Suite) ForwardSecrecy() bool {
	return s.TLS13 || s.Kx == "DHE" || s.Kx == "ECDHE"
}

// AEAD reports an authenticated-encryption cipher.
func (s Suite) AEAD() bool {
	switch s.Mode {
	case ModeGCM, ModeCCM, ModeCCM8, ModeChaCha, ModeAEAD:
		return true
	}
	return false
}

// Anonymous reports a suite without server authentication.
func (s Suite) Anonymous() bool { return s.Auth == "anon" || s.Auth == "NULL" }

// MinVersion is the lowest protocol version that can negotiate the suite.
func (s Suite) MinVersion() uint16 {
	switch {
	case s.TLS13:
		return VersionTLS13
	case s.AEAD() || s.MAC == "SHA256" || s.MAC == "SHA384":
		return VersionTLS12
	}
	return VersionSSL30
}

// MaxVersion is the highest protocol version that can negotiate the suite.
func (s Suite) MaxVersion() uint16 {
	if s.TLS13 {
		return VersionTLS13
	}
	return VersionTLS12
}

// GroupKind classifies a key exchange group.
type GroupKind string

// Group kinds.
const (
	GroupECDHE       GroupKind = "ecdhe"        // x25519, x448, secp256r1…
	GroupECDHEWeak   GroupKind = "ecdhe_weak"   // prime curves under 256 bits (secp224r1, secp192r1…)
	GroupBinary      GroupKind = "binary"       // sect* binary curves (deprecated)
	GroupBrainpool   GroupKind = "brainpool"    // brainpool, TLS 1.2 and TLS 1.3 variants
	GroupFFDHE       GroupKind = "ffdhe"        // RFC 7919 finite-field groups
	GroupHybrid      GroupKind = "hybrid_mlkem" // standard hybrid ML-KEM (RFC 10024)
	GroupHybridDraft GroupKind = "hybrid_draft" // hybrid ML-KEM still in draft (ML-KEM-512, SM2)
	GroupPureMLKEM   GroupKind = "pure_mlkem"   // ML-KEM alone (FIPS 203)
	GroupKyberDraft  GroupKind = "kyber_draft"  // pre-standard Kyber, obsolete
	GroupOther       GroupKind = "other"        // GOST, SM2, explicit curves
)

// Group describes an IANA supported group.
type Group struct {
	ID   uint16
	Name string
	Kind GroupKind
	// TLS13 reports whether the group can be used in TLS 1.3 (key_share);
	// TLS12 whether it can be used for ECDHE in TLS 1.0–1.2.
	TLS13, TLS12 bool
	Recommended  byte
}

// PQC reports a group with a post-quantum component (standard or not).
func (g Group) PQC() bool {
	switch g.Kind {
	case GroupHybrid, GroupHybridDraft, GroupPureMLKEM, GroupKyberDraft:
		return true
	}
	return false
}

var (
	suites    []Suite
	suiteByID map[uint16]Suite
	groups    []Group
	groupByID map[uint16]Group
)

func init() {
	var err error
	if suites, err = parseSuites(suitesCSV); err != nil {
		panic(err)
	}
	suiteByID = make(map[uint16]Suite, len(suites))
	for _, s := range suites {
		suiteByID[s.ID] = s
	}
	if groups, err = parseGroups(groupsCSV); err != nil {
		panic(err)
	}
	groupByID = make(map[uint16]Group, len(groups))
	for _, g := range groups {
		groupByID[g.ID] = g
	}
}

// Suites returns every known cipher suite, sorted by ID.
func Suites() []Suite { return slices.Clone(suites) }

// SuiteByID looks up a suite.
func SuiteByID(id uint16) (Suite, bool) {
	s, ok := suiteByID[id]
	return s, ok
}

// SuiteName returns the IANA name of a suite, or its hex value.
func SuiteName(id uint16) string {
	if s, ok := suiteByID[id]; ok {
		return s.Name
	}
	return fmt.Sprintf("0x%04x", id)
}

// Groups returns every known group, sorted by ID.
func Groups() []Group { return slices.Clone(groups) }

// GroupByID looks up a group.
func GroupByID(id uint16) (Group, bool) {
	g, ok := groupByID[id]
	return g, ok
}

// GroupName returns the IANA name of a group, or its hex value.
func GroupName(id uint16) string {
	if g, ok := groupByID[id]; ok {
		return g.Name
	}
	return fmt.Sprintf("0x%04x", id)
}

// extraSuites are not in the IANA registry (0x0060–0x0066 is "reserved to
// avoid conflicts with widely deployed implementations") but old OpenSSL
// releases negotiate them, and testssl.sh checks them.
var extraSuites = []struct {
	id   uint16
	name string
}{
	{0x0060, "TLS_RSA_EXPORT1024_WITH_RC4_56_MD5"},
	{0x0061, "TLS_RSA_EXPORT1024_WITH_RC2_CBC_56_MD5"},
	{0x0062, "TLS_RSA_EXPORT1024_WITH_DES_CBC_SHA"},
	{0x0063, "TLS_DHE_DSS_EXPORT1024_WITH_DES_CBC_SHA"},
	{0x0064, "TLS_RSA_EXPORT1024_WITH_RC4_56_SHA"},
	{0x0065, "TLS_DHE_DSS_EXPORT1024_WITH_RC4_56_SHA"},
	{0x0066, "TLS_DHE_DSS_WITH_RC4_128_SHA"},
}

func parseSuites(data []byte) ([]Suite, error) {
	rows, err := csv.NewReader(bytes.NewReader(data)).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("rawtls: suites registry: %w", err)
	}
	var out []Suite
	for _, row := range rows[1:] {
		if len(row) < 4 {
			return nil, fmt.Errorf("rawtls: suites registry: short row %q", row)
		}
		value, name := row[0], strings.TrimSpace(row[1])
		if strings.Contains(value, "-") || !strings.HasPrefix(name, "TLS_") || strings.HasSuffix(name, "_SCSV") {
			continue // ranges, Unassigned, Reserved (GREASE…), signalling values
		}
		var id []byte
		parts := strings.Split(value, ",")
		if len(parts) == 2 {
			id, err = hex.DecodeString(strings.TrimPrefix(parts[0], "0x") + strings.TrimPrefix(parts[1], "0x"))
		}
		if len(parts) != 2 || err != nil || len(id) != 2 {
			return nil, fmt.Errorf("rawtls: suites registry: bad value %q", value)
		}
		var rec byte
		if r := strings.TrimSpace(row[3]); len(r) == 1 {
			rec = r[0]
		}
		s, err := describeSuite(binary.BigEndian.Uint16(id), name, rec)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	for _, e := range extraSuites {
		s, err := describeSuite(e.id, e.name, 0)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	slices.SortFunc(out, func(a, b Suite) int { return int(a.ID) - int(b.ID) })
	for i := 1; i < len(out); i++ {
		if out[i].ID == out[i-1].ID {
			return nil, fmt.Errorf("rawtls: suites registry: duplicate 0x%04x", out[i].ID)
		}
	}
	return out, nil
}

// kxAuth maps the part before _WITH_ to key exchange and authentication.
var kxAuth = map[string][2]string{
	"NULL":            {"NULL", "NULL"},
	"RSA":             {"RSA", "RSA"},
	"DH_DSS":          {"DH", "DSS"},
	"DH_RSA":          {"DH", "RSA"},
	"DHE_DSS":         {"DHE", "DSS"},
	"DHE_RSA":         {"DHE", "RSA"},
	"DH_anon":         {"DHE", "anon"}, // anonymous DH is ephemeral
	"ECDH_ECDSA":      {"ECDH", "ECDSA"},
	"ECDH_RSA":        {"ECDH", "RSA"},
	"ECDHE_ECDSA":     {"ECDHE", "ECDSA"},
	"ECDHE_RSA":       {"ECDHE", "RSA"},
	"ECDH_anon":       {"ECDHE", "anon"},
	"KRB5":            {"KRB5", "KRB5"},
	"PSK":             {"PSK", "PSK"},
	"PSK_DHE":         {"DHE_PSK", "PSK"},
	"DHE_PSK":         {"DHE_PSK", "PSK"},
	"RSA_PSK":         {"RSA_PSK", "PSK"},
	"ECDHE_PSK":       {"ECDHE_PSK", "PSK"},
	"SRP_SHA":         {"SRP", "SRP"},
	"SRP_SHA_RSA":     {"SRP", "RSA"},
	"SRP_SHA_DSS":     {"SRP", "DSS"},
	"ECCPWD":          {"ECCPWD", "ECCPWD"},
	"GOSTR341112_256": {"GOST", "GOST"},
}

// cipherBits gives the strength of the cipher part of a name.
var cipherBits = map[string]int{
	"NULL": 0, "RC4_40": 40, "RC4_56": 56, "RC4_128": 128, "RC2_CBC_40": 40, "RC2_CBC_56": 56,
	"DES40_CBC": 40, "DES_CBC_40": 40, "DES_CBC": 56, "3DES_EDE_CBC": 112, "IDEA_CBC": 128, "SEED_CBC": 128,
	"AES_128_CBC": 128, "AES_256_CBC": 256, "AES_128_GCM": 128, "AES_256_GCM": 256,
	"AES_128_CCM": 128, "AES_256_CCM": 256, "AES_128_CCM_8": 128, "AES_256_CCM_8": 256,
	"CAMELLIA_128_CBC": 128, "CAMELLIA_256_CBC": 256, "CAMELLIA_128_GCM": 128, "CAMELLIA_256_GCM": 256,
	"ARIA_128_CBC": 128, "ARIA_256_CBC": 256, "ARIA_128_GCM": 128, "ARIA_256_GCM": 256,
	"CHACHA20_POLY1305": 256, "SM4_GCM": 128, "SM4_CCM": 128, "ASCONAEAD128": 128,
	"AEGIS_256": 256, "AEGIS_128L": 128,
	"KUZNYECHIK_CTR": 256, "MAGMA_CTR": 256, "28147_CNT": 256, "KUZNYECHIK_MGM_L": 256,
	"MAGMA_MGM_L": 256, "KUZNYECHIK_MGM_S": 256, "MAGMA_MGM_S": 256,
}

var macs = []string{"SHA512", "SHA384", "SHA256", "SHA", "MD5", "SM3", "ASCONHASH256", "OMAC", "IMIT"}

// describeSuite derives a Suite from its IANA name.
func describeSuite(id uint16, name string, rec byte) (Suite, error) {
	s := Suite{ID: id, Name: name, Recommended: rec, Offerable: true}
	body := strings.TrimPrefix(name, "TLS_")
	left, right, hasWith := strings.Cut(body, "_WITH_")
	if strings.HasPrefix(left, "GOSTR341112_256") && hasWith && strings.Contains(right, "MGM") {
		s.TLS13 = true // RFC 9367: TLS 1.3 GOST suites keep a _WITH_ in their name
	}
	if !hasWith { // TLS 1.3: TLS_<cipher>_<hash>
		s.TLS13 = true
		right = body
	} else if !s.TLS13 {
		kx := left
		if base, ok := strings.CutSuffix(kx, "_EXPORT1024"); ok {
			kx, s.Export = base, true
		} else if base, ok := strings.CutSuffix(kx, "_EXPORT"); ok {
			kx, s.Export = base, true
		}
		ka, ok := kxAuth[kx]
		if !ok {
			return Suite{}, fmt.Errorf("rawtls: suite %s: unknown key exchange %q", name, kx)
		}
		s.Kx, s.Auth = ka[0], ka[1]
	}
	if s.TLS13 && strings.HasPrefix(left, "GOSTR341112_256") {
		s.Kx, s.Auth = "", ""
		s.Offerable = false
	}

	// Split <cipher>_<mac>. CCM suites of TLS 1.2 have no MAC part.
	cipher := right
	for _, m := range macs {
		if c, ok := strings.CutSuffix(right, "_"+m); ok {
			cipher, s.MAC = c, m
			break
		}
	}
	switch cipher {
	case "SHA256", "SHA384": // RFC 9150 integrity-only TLS 1.3 suites (TLS_SHA256_SHA256)
		cipher = "NULL"
	case "NULL_NULL": // TLS_NULL_WITH_NULL_NULL
		cipher, s.MAC = "NULL", "NULL"
	}
	bits, ok := cipherBits[cipher]
	if !ok {
		return Suite{}, fmt.Errorf("rawtls: suite %s: unknown cipher %q", name, cipher)
	}
	s.Cipher, s.Bits = cipher, bits
	switch {
	case cipher == "NULL":
		s.Mode = ModeNone
	case strings.HasPrefix(cipher, "RC4"):
		s.Mode = ModeStream
	case strings.HasSuffix(cipher, "_CCM_8"):
		s.Mode = ModeCCM8
	case strings.HasSuffix(cipher, "_CCM"):
		s.Mode = ModeCCM
	case strings.HasSuffix(cipher, "_GCM"):
		s.Mode = ModeGCM
	case cipher == "CHACHA20_POLY1305":
		s.Mode = ModeChaCha
	case strings.Contains(cipher, "CBC"):
		s.Mode = ModeCBC
	case strings.Contains(cipher, "MGM") || strings.HasPrefix(cipher, "AEGIS") || strings.HasPrefix(cipher, "ASCON"):
		s.Mode = ModeAEAD
	default:
		s.Mode = ModeOther
	}
	switch s.Kx {
	case "PSK", "DHE_PSK", "RSA_PSK", "ECDHE_PSK", "SRP", "KRB5", "ECCPWD", "GOST":
		s.Offerable = false
	}
	return s, nil
}

// groupKinds classifies the groups by IANA name; anything else is GroupOther.
var groupKinds = map[string]GroupKind{
	"x25519": GroupECDHE, "x448": GroupECDHE, "secp256r1": GroupECDHE, "secp384r1": GroupECDHE,
	"secp521r1": GroupECDHE, "secp256k1": GroupECDHE,
	"secp160k1": GroupECDHEWeak, "secp160r1": GroupECDHEWeak, "secp160r2": GroupECDHEWeak,
	"secp192k1": GroupECDHEWeak, "secp192r1": GroupECDHEWeak, "secp224k1": GroupECDHEWeak, "secp224r1": GroupECDHEWeak,
	"brainpoolP256r1": GroupBrainpool, "brainpoolP384r1": GroupBrainpool, "brainpoolP512r1": GroupBrainpool,
	"brainpoolP256r1tls13": GroupBrainpool, "brainpoolP384r1tls13": GroupBrainpool, "brainpoolP512r1tls13": GroupBrainpool,
	"ffdhe2048": GroupFFDHE, "ffdhe3072": GroupFFDHE, "ffdhe4096": GroupFFDHE, "ffdhe6144": GroupFFDHE, "ffdhe8192": GroupFFDHE,
	"X25519MLKEM768": GroupHybrid, "SecP256r1MLKEM768": GroupHybrid, "SecP384r1MLKEM1024": GroupHybrid,
	"SecP256r1MLKEM512": GroupHybridDraft, "MLKEM512X25519": GroupHybridDraft, "curveSM2MLKEM768": GroupHybridDraft,
	"MLKEM512": GroupPureMLKEM, "MLKEM768": GroupPureMLKEM, "MLKEM1024": GroupPureMLKEM,
	"X25519Kyber768Draft00": GroupKyberDraft, "SecP256r1Kyber768Draft00": GroupKyberDraft,
}

func parseGroups(data []byte) ([]Group, error) {
	rows, err := csv.NewReader(bytes.NewReader(data)).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("rawtls: groups registry: %w", err)
	}
	var out []Group
	for _, row := range rows[1:] {
		if len(row) < 4 {
			return nil, fmt.Errorf("rawtls: groups registry: short row %q", row)
		}
		name := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(row[1]), "(OBSOLETE)"))
		if strings.Contains(row[0], "-") || name == "Reserved" || name == "Unassigned" {
			continue
		}
		id, err := strconv.ParseUint(row[0], 10, 16)
		if err != nil {
			return nil, fmt.Errorf("rawtls: groups registry: bad value %q", row[0])
		}
		g := Group{ID: uint16(id), Name: name, Kind: GroupOther}
		if r := strings.TrimSpace(row[3]); len(r) == 1 {
			g.Recommended = r[0]
		}
		if k, ok := groupKinds[name]; ok {
			g.Kind = k
		}
		switch {
		case strings.HasPrefix(name, "sect"):
			g.Kind, g.TLS12 = GroupBinary, true
		case g.Kind == GroupECDHE || g.Kind == GroupECDHEWeak:
			g.TLS12 = true
			// RFC 8446 allows only these elliptic curves in TLS 1.3.
			g.TLS13 = slices.Contains([]string{"x25519", "x448", "secp256r1", "secp384r1", "secp521r1"}, name)
		case g.Kind == GroupBrainpool:
			g.TLS13 = strings.HasSuffix(name, "tls13")
			g.TLS12 = !g.TLS13
		case g.Kind == GroupFFDHE:
			g.TLS13 = true // in TLS 1.2 a DHE server picks its own group: see ServerKeyExchange
		case g.PQC():
			g.TLS13 = true
		case strings.HasPrefix(name, "GC"), name == "curveSM2":
			g.TLS13 = true
		case strings.HasPrefix(name, "arbitrary_explicit"):
			g.TLS12 = true
		}
		out = append(out, g)
	}
	slices.SortFunc(out, func(a, b Group) int { return int(a.ID) - int(b.ID) })
	return out, nil
}
