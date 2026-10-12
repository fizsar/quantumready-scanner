package scanner

import "time"

// SchemaVersion identifies the JSON layout of Result. Bump it on breaking
// changes: results are stored as JSON and scored later.
const SchemaVersion = 1

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
	// TLS13Suite is the suite negotiated in TLS 1.3 (the full list cannot be enumerated).
	TLS13Suite string `json:"tls13_cipher_suite,omitempty"`

	Certificate *Certificate `json:"certificate,omitempty"`

	// NotEvaluated lists the checks crypto/tls cannot perform (see docs/scanner.md).
	NotEvaluated []string `json:"not_evaluated"`
}

// Handshake summarizes one full handshake.
type Handshake struct {
	Status      Status `json:"status"`
	Version     string `json:"version,omitempty"`
	CipherSuite string `json:"cipher_suite,omitempty"`
	Group       string `json:"group,omitempty"`
	GroupID     uint16 `json:"group_id,omitempty"`
	Error       string `json:"error,omitempty"`
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
	Name   string `json:"name"`
	ID     uint16 `json:"id"`
	Hybrid bool   `json:"hybrid"` // classical + ML-KEM hybrid
	Status Status `json:"status"`
	Error  string `json:"error,omitempty"`
}

// VersionProbe is the result of offering a single protocol version.
type VersionProbe struct {
	Version string `json:"version"`
	ID      uint16 `json:"id"`
	Status  Status `json:"status"`
	Error   string `json:"error,omitempty"`
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

	Trusted       bool   `json:"trusted"` // chains to a trusted root, valid now
	TrustError    string `json:"trust_error,omitempty"`
	HostnameMatch bool   `json:"hostname_match"`
	Expired       bool   `json:"expired"`
	NotYetValid   bool   `json:"not_yet_valid"`
	SelfSigned    bool   `json:"self_signed"`

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
