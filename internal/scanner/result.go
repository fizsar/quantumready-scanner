package scanner

import (
	"fmt"
	"slices"
	"time"
)

// SchemaVersion identifies the JSON layout of Result. Bump it on breaking
// changes: results are stored as JSON and scored later.
//
// Version 2 (milestone 2.8.4) adds, all optional, so a version 1 result reads
// the same: ProbeLog, an ErrorCode beside every Error (TrustErrorCode beside
// TrustError) and the OIDs of the chain. Scoring reads none of them.
const SchemaVersion = 2

// Status is the outcome of a single probe.
type Status string

const (
	// Supported means the handshake completed with the offered parameter.
	Supported Status = "supported"
	// NotSupported means the server refused it (alert, closed connection or
	// selected something we did not offer).
	NotSupported Status = "not_supported"
	// ProbeFailed means the probe could not decide (timeout, network error).
	// It must never be read as "not supported".
	ProbeFailed Status = "error"
)

// Result is everything a scan observed about one TLS endpoint. It contains
// facts only; judging them is the job of internal/scoring.
type Result struct {
	SchemaVersion int       `json:"schema_version"`
	Host          string    `json:"host"`
	Port          int       `json:"port"`
	IP            string    `json:"ip"`           // address actually scanned
	ResolvedIPs   []string  `json:"resolved_ips"` // every address the name resolved to
	StartedAt     time.Time `json:"started_at"`
	DurationMS    int64     `json:"duration_ms"`

	// Default is the handshake with a browser-like offer (see BrowserGroups).
	Default     Handshake      `json:"default"`
	KeyExchange KeyExchange    `json:"key_exchange"`
	Versions    []VersionProbe `json:"versions"`

	// TLS12Suites lists the TLS 1.2 suites the server accepted, among those
	// crypto/tls can offer, in the order they were found (not the server's
	// preference order: Go sorts the suites it offers).
	TLS12Suites []CipherSuite `json:"tls12_cipher_suites"`
	// TLS12SuitesComplete is false when the enumeration stopped on an error.
	TLS12SuitesComplete bool `json:"tls12_cipher_suites_complete"`
	// TLS10Suites and TLS11Suites list the suites accepted in TLS 1.0 and 1.1
	// (empty when the version is refused). Weak suites such as 3DES sometimes
	// survive only there.
	TLS10Suites []CipherSuite `json:"tls10_cipher_suites,omitempty"`
	TLS11Suites []CipherSuite `json:"tls11_cipher_suites,omitempty"`
	// LegacySuitesComplete is false when the TLS 1.0/1.1 enumeration stopped on an error.
	LegacySuitesComplete bool `json:"legacy_cipher_suites_complete"`
	// TLS13Suite is the suite negotiated in TLS 1.3 (the full list cannot be enumerated).
	TLS13Suite string `json:"tls13_cipher_suite,omitempty"`

	Certificate *Certificate `json:"certificate,omitempty"`

	// NotEvaluated lists the checks this scan could not perform (see docs/scanner.md).
	NotEvaluated []string `json:"not_evaluated"`

	// Fields added in milestone 2.7.3; all optional, so results stored
	// before keep their meaning (SchemaVersion is unchanged).

	// Intensity is "basic" (crypto/tls handshakes only) or "full" (also raw
	// ClientHello probes, see Raw).
	Intensity Intensity `json:"intensity,omitempty"`
	// Connections is the number of TCP connections the scan opened.
	Connections int `json:"connections,omitempty"`
	// Raw holds what the raw ClientHello probes found (intensity full).
	Raw *RawResult `json:"raw,omitempty"`

	// ProbeLog (schema 2) is every connection the scan made, as it went on
	// the wire.
	ProbeLog *ProbeLog `json:"probe_log,omitempty"`
}

// RawResult is what the raw ClientHello probes (internal/scanner/rawtls)
// observed: what crypto/tls cannot offer. A "complete" flag is false when an
// enumeration stopped on an error or on the probe budget; true means the
// enumeration finished, whatever it found.
//
// Suites, TLS13Groups and TLS12Curves list only what crypto/tls did not
// already find or probe (ListsExcludeCryptoTLS): an empty list means
// "nothing beyond key_exchange.groups and the tls1x_cipher_suites lists",
// not "none at all". Results written before showcase S6d may hold null
// instead of an empty list and lack ListsExcludeCryptoTLS, with the same
// meaning; Normalize brings them to the current form.
type RawResult struct {
	Probes          int  `json:"probes"` // raw connections used
	BudgetExhausted bool `json:"budget_exhausted,omitempty"`

	// ListsExcludeCryptoTLS is always true: see above.
	ListsExcludeCryptoTLS bool `json:"lists_exclude_crypto_tls"`

	SSLv2 RawProtocol `json:"sslv2"`
	SSLv3 RawProtocol `json:"sslv3"`

	// Versions: TLS 1.0, 1.1 and 1.2 (oldest first) as seen by the raw
	// probes, for the versions crypto/tls did not find (it cannot offer
	// their suites).
	Versions []VersionProbe `json:"versions,omitempty"`

	// Suites lists, per version (TLS 1.2, then 1.1, 1.0 and, only if the
	// server speaks it, SSLv3), the accepted suites not already found by
	// crypto/tls in that version nor by the raw probes in a higher version.
	Suites         []RawSuite `json:"suites"`
	SuitesComplete bool       `json:"suites_complete"`

	// TLS13Suites is every TLS 1.3 suite accepted (crypto/tls only sees the
	// negotiated one).
	TLS13Suites         []RawSuite `json:"tls13_suites"`
	TLS13SuitesComplete bool       `json:"tls13_suites_complete"`

	// DH lists the finite-field DH groups seen in ServerKeyExchange.
	DH []DHParams `json:"dh,omitempty"`

	// TLS13Groups are TLS 1.3 groups crypto/tls does not probe (ffdhe,
	// brainpool, x448, pure ML-KEM, draft hybrids, pre-standard Kyber…),
	// found through HelloRetryRequest.
	TLS13Groups         []RawGroup `json:"tls13_groups"`
	TLS13GroupsComplete bool       `json:"tls13_groups_complete"`
	// TLS12Curves are ECDHE curves crypto/tls does not probe (secp224r1,
	// brainpool, binary curves…), found through ServerKeyExchange.
	TLS12Curves         []RawGroup `json:"tls12_curves"`
	TLS12CurvesComplete bool       `json:"tls12_curves_complete"`
	ExplicitCurve       bool       `json:"explicit_curve,omitempty"` // EC parameters sent explicitly
}

// Normalize brings a raw result to the current form: empty lists instead of
// null, ListsExcludeCryptoTLS set, Versions oldest first. Idempotent; the
// scanner applies it to every new result, and it reads old ones the same way.
func (r *RawResult) Normalize() {
	if r == nil {
		return
	}
	r.ListsExcludeCryptoTLS = true
	if r.Suites == nil {
		r.Suites = []RawSuite{}
	}
	if r.TLS13Suites == nil {
		r.TLS13Suites = []RawSuite{}
	}
	if r.TLS13Groups == nil {
		r.TLS13Groups = []RawGroup{}
	}
	if r.TLS12Curves == nil {
		r.TLS12Curves = []RawGroup{}
	}
	slices.SortStableFunc(r.Versions, func(a, b VersionProbe) int { return int(a.ID) - int(b.ID) })
}

// RawProtocol is the result of offering SSLv2 or SSLv3.
type RawProtocol struct {
	Status Status `json:"status"`
	// Ciphers: for SSLv2 the cipher kinds the server listed, for SSLv3 the
	// suite it picked.
	Ciphers   []string `json:"ciphers,omitempty"`
	Error     string   `json:"error,omitempty"`
	ErrorCode string   `json:"error_code,omitempty"` // stable, see errorCode
}

// RawSuite is a suite accepted in a raw probe, with the attributes scoring needs.
type RawSuite struct {
	Version        string `json:"version"`
	Name           string `json:"name"`
	ID             uint16 `json:"id"`
	ForwardSecrecy bool   `json:"forward_secrecy"`
	AEAD           bool   `json:"aead"`
	Mode           string `json:"mode"` // none, stream, cbc, gcm, ccm, ccm8, chacha20_poly1305, aead, other
	Bits           int    `json:"bits"` // symmetric strength (3DES 112, NULL 0)
	Export         bool   `json:"export,omitempty"`
	Anonymous      bool   `json:"anonymous,omitempty"`
}

// DHParams is a finite-field DH group observed in a ServerKeyExchange.
type DHParams struct {
	Version   string `json:"version"`
	Suite     string `json:"suite"`
	PrimeBits int    `json:"prime_bits"`
}

// RawGroup is a key exchange group found by a raw probe.
type RawGroup struct {
	Name string `json:"name"`
	ID   uint16 `json:"id"`
	Kind string `json:"kind"` // see rawtls.GroupKind
}

// Intensity selects which probes a scan runs.
type Intensity string

const (
	// IntensityBasic runs only crypto/tls handshakes (as before milestone 2.7).
	IntensityBasic Intensity = "basic"
	// IntensityFull adds the raw ClientHello probes. In phase 3 it is meant
	// for verified domains only.
	IntensityFull Intensity = "full"
)

// ParseIntensity validates an intensity name.
func ParseIntensity(s string) (Intensity, error) {
	switch Intensity(s) {
	case IntensityBasic, IntensityFull:
		return Intensity(s), nil
	}
	return "", fmt.Errorf("scanner: intensity %q: want basic or full", s)
}

// Handshake summarizes one full handshake.
type Handshake struct {
	Status      Status `json:"status"`
	Version     string `json:"version,omitempty"`
	CipherSuite string `json:"cipher_suite,omitempty"`
	Group       string `json:"group,omitempty"`
	GroupID     uint16 `json:"group_id,omitempty"`
	Error       string `json:"error,omitempty"`
	ErrorCode   string `json:"error_code,omitempty"`
}

// KeyExchange describes the key exchange groups: the one the server picks for
// a browser-like offer, and each group probed on its own.
type KeyExchange struct {
	// Offered is the browser-like preference list sent in the default handshake.
	Offered []string `json:"offered"`
	// Selected is the group the server chose from Offered ("" if none, e.g. RSA key exchange).
	Selected string `json:"selected,omitempty"`
	// SelectedHybrid reports whether Selected is a hybrid ML-KEM group.
	SelectedHybrid bool `json:"selected_hybrid"`
	// Groups holds one probe per group, offering only that group.
	Groups []GroupProbe `json:"groups"`
}

// GroupProbe is the result of offering a single key exchange group.
type GroupProbe struct {
	Name      string `json:"name"`
	ID        uint16 `json:"id"`
	Hybrid    bool   `json:"hybrid"` // classical + ML-KEM hybrid
	Status    Status `json:"status"`
	Error     string `json:"error,omitempty"`
	ErrorCode string `json:"error_code,omitempty"`
}

// VersionProbe is the result of offering a single protocol version.
type VersionProbe struct {
	Version   string `json:"version"`
	ID        uint16 `json:"id"`
	Status    Status `json:"status"`
	Error     string `json:"error,omitempty"`
	ErrorCode string `json:"error_code,omitempty"`
}

// CipherSuite is a TLS 1.2 (or earlier) cipher suite accepted by the server.
type CipherSuite struct {
	Name           string `json:"name"`
	ID             uint16 `json:"id"`
	ForwardSecrecy bool   `json:"forward_secrecy"` // ephemeral (EC)DHE key exchange
	AEAD           bool   `json:"aead"`            // GCM or ChaCha20-Poly1305
	Insecure       bool   `json:"insecure"`        // listed by tls.InsecureCipherSuites
}

// Certificate describes the server's leaf certificate and chain.
type Certificate struct {
	Subject      string    `json:"subject"`
	SANs         []string  `json:"sans"`
	Issuer       string    `json:"issuer"`
	IssuerOrg    []string  `json:"issuer_org,omitempty"`
	NotBefore    time.Time `json:"not_before"`
	NotAfter     time.Time `json:"not_after"`
	ValidityDays int       `json:"validity_days"`  // NotAfter - NotBefore
	DaysToExpiry int       `json:"days_to_expiry"` // negative when expired

	KeyAlgorithm       string `json:"key_algorithm"` // RSA, ECDSA, Ed25519 or the OID name
	KeySize            int    `json:"key_size"`      // bits (RSA modulus, curve size)
	Curve              string `json:"curve,omitempty"`
	SignatureAlgorithm string `json:"signature_algorithm"`

	Trusted        bool   `json:"trusted"` // chains to a trusted root, valid now
	TrustError     string `json:"trust_error,omitempty"`
	TrustErrorCode string `json:"trust_error_code,omitempty"` // stable, see trustErrorCode
	HostnameMatch  bool   `json:"hostname_match"`
	Expired        bool   `json:"expired"`
	NotYetValid    bool   `json:"not_yet_valid"`
	SelfSigned     bool   `json:"self_signed"`

	OCSPStapled bool `json:"ocsp_stapled"`
	// Signed Certificate Timestamps (Certificate Transparency): embedded in the
	// certificate by the CA, or delivered in the TLS handshake extension.
	SCTsEmbedded  int `json:"scts_embedded"`
	SCTsHandshake int `json:"scts_handshake"`

	Chain []ChainCert `json:"chain"` // as sent by the server, leaf first
}

// ChainCert is one certificate of the chain sent by the server.
type ChainCert struct {
	Subject            string    `json:"subject"`
	Issuer             string    `json:"issuer"`
	KeyAlgorithm       string    `json:"key_algorithm"`
	KeySize            int       `json:"key_size"`
	SignatureAlgorithm string    `json:"signature_algorithm"`
	NotAfter           time.Time `json:"not_after"`
	// The algorithm identifiers as sent (schema 2), also those crypto/x509
	// does not know (SLH-DSA, drafts…).
	SignatureOID string `json:"signature_oid,omitempty"`
	PublicKeyOID string `json:"public_key_oid,omitempty"`
}

// Checks crypto/tls cannot perform. Codes are stable; their explanation lives
// in docs/scanner.md and in the scoring catalog.
const (
	NotEvalSSL               = "ssl2_ssl3"
	NotEvalDHESuites         = "tls12_dhe_suites"
	NotEvalOtherSuites       = "tls12_ccm_camellia_aria_null_export_suites"
	NotEvalTLS13Suites       = "tls13_suite_list"
	NotEvalServerPreference  = "tls12_server_cipher_preference"
	NotEvalLegacyGroups      = "legacy_groups"
	NotEvalNonStandardPQC    = "non_standard_pqc_groups"
	NotEvalImplementationBug = "implementation_vulnerabilities"
)

// NotEvalCredentialSuites (intensity full): PSK, SRP, Kerberos and GOST
// suites need credentials or algorithms a scanner does not have.
const NotEvalCredentialSuites = "credential_suites" //nolint:gosec // a code name, not a credential

// notEvaluated is the list for intensity basic (unchanged since milestone 2.1).
var notEvaluated = []string{
	NotEvalSSL,
	NotEvalDHESuites,
	NotEvalOtherSuites,
	NotEvalTLS13Suites,
	NotEvalServerPreference,
	NotEvalLegacyGroups,
	NotEvalNonStandardPQC,
	NotEvalImplementationBug,
}

// notEvaluatedFull is what intensity full still cannot check, plus whatever
// raw enumeration did not complete.
func notEvaluatedFull(r *RawResult) []string {
	var out []string
	if r.SSLv2.Status == ProbeFailed || r.SSLv3.Status == ProbeFailed {
		out = append(out, NotEvalSSL)
	}
	if !r.SuitesComplete {
		out = append(out, NotEvalDHESuites, NotEvalOtherSuites)
	}
	if !r.TLS13SuitesComplete {
		out = append(out, NotEvalTLS13Suites)
	}
	out = append(out, NotEvalServerPreference)
	if !r.TLS13GroupsComplete || !r.TLS12CurvesComplete {
		out = append(out, NotEvalLegacyGroups)
	}
	if !r.TLS13GroupsComplete {
		out = append(out, NotEvalNonStandardPQC)
	}
	return append(out, NotEvalCredentialSuites, NotEvalImplementationBug)
}
