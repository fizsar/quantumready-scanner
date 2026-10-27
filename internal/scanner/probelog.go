package scanner

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"net"
	"slices"
	"sync"
	"time"

	"github.com/fizsar/quantumready-saas/internal/scanner/rawtls"
)

// ProbeLog is every connection of a scan as it went on the wire (schema 2).
// The offer is read back from the bytes the client wrote, the response from
// the bytes the server sent: nothing is reconstructed from configuration.
type ProbeLog struct {
	// Offers are the distinct ClientHellos sent, in order of first use;
	// probes refer to them by index (many probes send the same one). Use
	// Offer(i): an entry may be stored as a difference (see Offer.Like).
	Offers []Offer       `json:"offers"`
	Probes []ProbeRecord `json:"probes"`
}

// Offer is one ClientHello as sent. Lists keep the wire order; GREASE and
// the renegotiation SCSV appear as sent.
type Offer struct {
	SSLv2             bool     `json:"sslv2,omitempty"` // an SSLv2 CLIENT-HELLO
	LegacyVersion     uint16   `json:"legacy_version,omitempty"`
	SupportedVersions []uint16 `json:"supported_versions,omitempty"`
	Suites            []uint16 `json:"suites,omitempty"`
	SSLv2Ciphers      []uint32 `json:"sslv2_ciphers,omitempty"`
	Groups            []uint16 `json:"groups,omitempty"`
	// KeyShares are the groups with a key share; with extension 51 listed
	// and no key share, the client asks for a HelloRetryRequest.
	KeyShares            []uint16 `json:"key_shares,omitempty"`
	SignatureSchemes     []uint16 `json:"signature_schemes,omitempty"`
	SignatureSchemesCert []uint16 `json:"signature_schemes_cert,omitempty"`
	ALPN                 []string `json:"alpn,omitempty"`
	Extensions           []uint16 `json:"extensions,omitempty"` // types, in the order sent

	// Like abbreviates an offer of an enumeration: it is Offers[Like] (an
	// earlier entry) without SuitesRemoved and GroupsRemoved, which the
	// previous probes found. Every other field is then empty.
	Like          *int     `json:"like,omitempty"`
	SuitesRemoved []uint16 `json:"suites_removed,omitempty"`
	GroupsRemoved []uint16 `json:"groups_removed,omitempty"`
}

// Offer returns offer i in full, expanding differences.
func (l *ProbeLog) Offer(i int) (Offer, bool) {
	if i < 0 || i >= len(l.Offers) {
		return Offer{}, false
	}
	o := l.Offers[i]
	if o.Like == nil {
		return o, true
	}
	if *o.Like >= i { // only earlier entries: no cycles
		return Offer{}, false
	}
	base, ok := l.Offer(*o.Like)
	if !ok {
		return Offer{}, false
	}
	base.Suites = slices.DeleteFunc(slices.Clone(base.Suites), func(id uint16) bool { return slices.Contains(o.SuitesRemoved, id) })
	base.Groups = slices.DeleteFunc(slices.Clone(base.Groups), func(id uint16) bool { return slices.Contains(o.GroupsRemoved, id) })
	return base, true
}

// ProbeRecord is one connection.
type ProbeRecord struct {
	// Client is "crypto/tls" or "raw" (internal/scanner/rawtls).
	Client string `json:"client"`
	// Purpose says what the probe was for; Subject what it tested, when it
	// tested one thing (a group, a version).
	Purpose string `json:"purpose"`
	Subject string `json:"subject,omitempty"`
	// Offer indexes ProbeLog.Offers; absent when nothing was sent.
	Offer *int `json:"offer,omitempty"`
	// RetryOffer indexes the second ClientHello, sent after a
	// HelloRetryRequest (with the key share the server asked for).
	RetryOffer *int          `json:"retry_offer,omitempty"`
	Response   ProbeResponse `json:"response"`
	// Status is the probe's verdict for its purpose, as in the rest of the
	// result; Error is the library's text (evidence only) and ErrorCode its
	// stable code.
	Status     Status `json:"status"`
	Error      string `json:"error,omitempty"`
	ErrorCode  string `json:"error_code,omitempty"`
	StartMS    int64  `json:"start_ms"` // since the scan started
	DurationMS int64  `json:"duration_ms"`

	at time.Time // for ordering
}

// Response types.
const (
	RespServerHello       = "server_hello"
	RespHelloRetryRequest = "hello_retry_request" // then the ServerHello that followed, if any
	RespAlert             = "alert"
	RespClosed            = "closed"  // nothing received: connection closed or reset
	RespTimeout           = "timeout" // nothing received in time
	RespNotTLS            = "not_tls" // bytes that are not a TLS answer
	RespSSLv2Hello        = "sslv2_server_hello"
	RespSSLv2Error        = "sslv2_error"
	RespNoConnection      = "no_connection" // the TCP connection failed
	RespNotSent           = "not_sent"      // no ClientHello sent (budget, local error)
)

// ProbeResponse is what the server answered, read from the bytes it sent up
// to where encryption starts.
type ProbeResponse struct {
	Type            string `json:"type"`
	Version         uint16 `json:"version,omitempty"`
	CipherSuite     uint16 `json:"cipher_suite,omitempty"`
	CipherSuiteName string `json:"cipher_suite_name,omitempty"`
	// Group is the TLS 1.3 key share group, or the ECDHE curve of a TLS
	// 1.0–1.2 ServerKeyExchange.
	Group     uint16 `json:"group,omitempty"`
	GroupName string `json:"group_name,omitempty"`
	// RetryGroup is the group a HelloRetryRequest asked for.
	RetryGroup     uint16 `json:"retry_group,omitempty"`
	RetryGroupName string `json:"retry_group_name,omitempty"`
	DHBits         int    `json:"dh_bits,omitempty"`
	// SSLv2Ciphers is how many cipher kinds an SSLv2 SERVER-HELLO listed.
	SSLv2Ciphers int         `json:"sslv2_ciphers,omitempty"`
	Alert        *ProbeAlert `json:"alert,omitempty"`
	// Certificate is the leaf the server presented (milestone 2.8.5: the
	// browser-like probe and the certificate probe), and HandshakeSignature
	// the SignatureScheme of its CertificateVerify, when the probe read it
	// (the certificate probe decrypts it; crypto/tls does not report it).
	Certificate        *PresentedCert `json:"certificate,omitempty"`
	HandshakeSignature uint16         `json:"handshake_signature,omitempty"`
}

// ProbeAlert is an alert from the server.
type ProbeAlert struct {
	// Level is 1 (warning) or 2 (fatal); 0 when the alert came encrypted:
	// crypto/tls reports only its description.
	Level     uint8  `json:"level,omitempty"`
	Code      uint8  `json:"code"`
	Name      string `json:"name"`
	Encrypted bool   `json:"encrypted,omitempty"`
}

// Probe purposes.
const (
	PurposeDefault     = "default"      // browser-like handshake
	PurposeGroup       = "group"        // one group (Subject)
	PurposeVersion     = "version"      // one version (Subject)
	PurposeSuites      = "suites"       // suite enumeration in one version (Subject)
	PurposeSSLv2       = "sslv2"        // raw
	PurposeSSLv3       = "sslv3"        // raw
	PurposeTLS13Suites = "tls13_suites" // raw
	PurposeTLS13Groups = "tls13_groups" // raw, through HelloRetryRequest
	PurposeTLS12Curves = "tls12_curves" // raw, through ServerKeyExchange (Subject: version)
	PurposeCertificate = "certificate"  // raw TLS 1.3, decrypted: the certificate without ML-DSA (Subject)
)

const (
	clientCryptoTLS = "crypto/tls"
	clientRaw       = "raw"
)

// maxRecorded caps what is kept of each direction of one connection: more
// than any ClientHello and than the clear part of a server's first flight.
const maxRecorded = rawtls.MaxResponseBytes

// recordingConn keeps a copy of the first bytes written and read. One
// handshake uses it from one goroutine at a time.
type recordingConn struct {
	net.Conn
	out, in []byte
}

func appendCapped(b, p []byte) []byte {
	return append(b, p[:min(len(p), maxRecorded-len(b))]...)
}

func (c *recordingConn) Write(p []byte) (int, error) {
	c.out = appendCapped(c.out, p)
	return c.Conn.Write(p)
}

func (c *recordingConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	c.in = appendCapped(c.in, p[:n])
	return n, err
}

// probeTag names a probe in the log.
type probeTag struct {
	purpose, subject string
}

// probeLog collects the records of one scan, from concurrent probes.
type probeLog struct {
	start  time.Time
	mu     sync.Mutex
	offers []Offer
	index  map[string]int
	probes []ProbeRecord
}

func newProbeLog() *probeLog { return &probeLog{start: time.Now(), index: map[string]int{}} }

// add records one connection. rc is nil when no connection was made; resp,
// when set, replaces the response read from rc (SSLv2 answers).
func (l *probeLog) add(client string, tag probeTag, start time.Time, rc *recordingConn, status Status, err error, resp *ProbeResponse) {
	if l == nil {
		return
	}
	end := time.Now()
	rec := ProbeRecord{
		Client: client, Purpose: tag.purpose, Subject: tag.subject,
		Status: status, Error: errText(err), ErrorCode: errorCode(err),
		DurationMS: end.Sub(start).Milliseconds(), at: start,
	}
	var offers []*Offer
	switch {
	case resp != nil:
		rec.Response = *resp
	case rc == nil && errors.Is(err, errBudget):
		rec.Response.Type = RespNotSent
	case rc == nil:
		rec.Response.Type = RespNoConnection
	default:
		rec.Response = responseOf(rc.out, rc.in, err)
	}
	if rc != nil && len(rc.out) > 0 {
		if chs, perr := rawtls.ParseClientHellos(rc.out); perr == nil {
			for _, ch := range chs[:min(len(chs), 2)] {
				offers = append(offers, offerOf(ch))
			}
		}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(offers) > 0 {
		rec.Offer = l.intern(offers[0])
	}
	if len(offers) > 1 {
		rec.RetryOffer = l.intern(offers[1])
	}
	l.probes = append(l.probes, rec)
}

// intern returns the index of o in l.offers, adding it if new. l.mu is held.
func (l *probeLog) intern(o *Offer) *int {
	key, _ := json.Marshal(o)
	i, ok := l.index[string(key)]
	if !ok {
		i = len(l.offers)
		l.offers = append(l.offers, *o)
		l.index[string(key)] = i
	}
	return &i
}

// result is the log in start order, offers numbered in order of first use.
// The offers of an enumeration (the same client, purpose and subject, one
// probe after another) are stored as the difference from the previous one.
func (l *probeLog) result() *ProbeLog {
	l.mu.Lock()
	defer l.mu.Unlock()
	probes := slices.Clone(l.probes)
	slices.SortStableFunc(probes, func(a, b ProbeRecord) int { return a.at.Compare(b.at) })
	offers := []Offer{}
	renumbered := map[int]int{}  // index in l.offers → index in offers
	previous := map[string]int{} // enumeration → its last offer, in l.offers
	for i := range probes {
		p := &probes[i]
		p.StartMS = p.at.Sub(l.start).Milliseconds()
		if p.Offer == nil {
			continue
		}
		old, enum := *p.Offer, p.Client+"|"+p.Purpose+"|"+p.Subject
		n, seen := renumbered[old]
		if !seen {
			o := l.offers[old]
			if prev, ok := previous[enum]; ok {
				if d, ok := difference(l.offers[prev], o); ok {
					like := renumbered[prev]
					d.Like = &like
					o = d
				}
			}
			n = len(offers)
			offers = append(offers, o)
			renumbered[old] = n
		}
		p.Offer = &n
		previous[enum] = old
		if p.RetryOffer != nil { // in full: it differs from the first in its key share
			old := *p.RetryOffer
			r, seen := renumbered[old]
			if !seen {
				r = len(offers)
				offers = append(offers, l.offers[old])
				renumbered[old] = r
			}
			p.RetryOffer = &r
		}
	}
	if probes == nil {
		probes = []ProbeRecord{}
	}
	return &ProbeLog{Offers: offers, Probes: probes}
}

// difference expresses o as base without some of its suites and groups, when
// that is all that differs.
func difference(base, o Offer) (Offer, bool) {
	suites, ok1 := removed(base.Suites, o.Suites)
	groups, ok2 := removed(base.Groups, o.Groups)
	if !ok1 || !ok2 || len(suites)+len(groups) == 0 {
		return Offer{}, false
	}
	b, x := base, o
	b.Suites, b.Groups, x.Suites, x.Groups = nil, nil, nil, nil
	bj, _ := json.Marshal(b)
	xj, _ := json.Marshal(x)
	if string(bj) != string(xj) {
		return Offer{}, false
	}
	return Offer{SuitesRemoved: suites, GroupsRemoved: groups}, true
}

// removed returns what list lacks of base, when list is base with some
// elements left out, in order. Lists with repeated values (several GREASE
// entries) are not abbreviated: removing by value would be ambiguous.
func removed(base, list []uint16) ([]uint16, bool) {
	var out []uint16
	j := 0
	for i, id := range base {
		if slices.Contains(base[i+1:], id) {
			return nil, false
		}
		if j < len(list) && list[j] == id {
			j++
			continue
		}
		out = append(out, id)
	}
	return out, j == len(list)
}

func offerOf(ch *rawtls.ClientHelloInfo) *Offer {
	return &Offer{
		SSLv2: ch.SSLv2, LegacyVersion: ch.LegacyVersion, SupportedVersions: ch.SupportedVersions,
		Suites: ch.Suites, SSLv2Ciphers: ch.SSLv2Ciphers, Groups: ch.Groups, KeyShares: ch.KeyShares,
		SignatureSchemes: ch.SignatureSchemes, SignatureSchemesCert: ch.SignatureSchemesCert,
		ALPN: ch.ALPN, Extensions: ch.Extensions,
	}
}

// responseOf reads the server's answer from the bytes it sent (in), and the
// error the probe ended with for what the bytes cannot show: a timeout, or an
// alert that came encrypted.
func responseOf(out, in []byte, err error) ProbeResponse {
	var r ProbeResponse
	f, perr := rawtls.ParseServerFlight(in)
	switch {
	case f.ServerHello != nil && f.ServerHello.HelloRetry:
		r.Type = RespHelloRetryRequest
		r.RetryGroup, r.RetryGroupName = f.ServerHello.KeyShareGroup, rawtls.GroupName(f.ServerHello.KeyShareGroup)
		if f.Retry != nil {
			r.setHello(f.Retry, f.KeyExchange)
		} else {
			r.Version, r.CipherSuite, r.CipherSuiteName = f.ServerHello.Version, f.ServerHello.CipherSuite, rawtls.SuiteName(f.ServerHello.CipherSuite)
		}
	case f.ServerHello != nil:
		r.Type = RespServerHello
		r.setHello(f.ServerHello, f.KeyExchange)
	case f.Alert != nil:
		r.Type = RespAlert
	case len(out) == 0:
		r.Type = RespNotSent
	case len(in) == 0 && errorCode(err) == "timeout":
		r.Type = RespTimeout
	case len(in) == 0:
		r.Type = RespClosed
	case perr != nil:
		r.Type = RespNotTLS
	default: // only records that carry nothing to report (a ChangeCipherSpec)
		r.Type = RespClosed
	}
	if f.Alert != nil {
		r.Alert = &ProbeAlert{Level: f.Alert.Level, Code: f.Alert.Description, Name: rawtls.AlertName(f.Alert.Description)}
	} else if code, ok := remoteAlert(err); ok {
		r.Alert = &ProbeAlert{Code: code, Name: rawtls.AlertName(code), Encrypted: true}
	}
	return r
}

func (r *ProbeResponse) setHello(sh *rawtls.ServerHello, ke *rawtls.KeyExchange) {
	r.Version, r.CipherSuite, r.CipherSuiteName = sh.Version, sh.CipherSuite, rawtls.SuiteName(sh.CipherSuite)
	switch {
	case sh.Version >= tls.VersionTLS13:
		r.Group, r.GroupName = sh.KeyShareGroup, rawtls.GroupName(sh.KeyShareGroup)
	case ke != nil && ke.Kind == "EC" && !ke.ExplicitCurve:
		r.Group, r.GroupName = ke.Group, rawtls.GroupName(ke.Group)
	case ke != nil && ke.Kind == "DH":
		r.DHBits = ke.DHPrimeBits
	}
}

// sslv2Response is the response of the SSLv2 probe, which is not TLS records.
func sslv2Response(resp *rawtls.SSLv2Response, out, in []byte, err error) *ProbeResponse {
	switch {
	case resp == nil:
		r := responseOf(out, in, err)
		if r.Type == RespNotTLS && len(in) > 0 && in[0]&0x80 != 0 {
			r.Type = RespSSLv2Error // an SSLv2 record we could not read
		}
		return &r
	case resp.ServerHello != nil:
		return &ProbeResponse{Type: RespSSLv2Hello, Version: resp.ServerHello.Version, SSLv2Ciphers: len(resp.ServerHello.Ciphers)}
	case resp.ErrorCode != nil:
		return &ProbeResponse{Type: RespSSLv2Error}
	case resp.TLSAlert != nil:
		return &ProbeResponse{Type: RespAlert, Alert: &ProbeAlert{Level: resp.TLSAlert.Level, Code: resp.TLSAlert.Description, Name: resp.TLSAlert.String()}}
	}
	return &ProbeResponse{Type: RespServerHello, Version: resp.TLSVersion}
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
