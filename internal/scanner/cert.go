package scanner

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"time"
)

// analyzeCertificate describes the chain sent by the server (leaf first) and
// verifies it independently of the handshake: trust at `now` against roots
// (nil = system pool), and the host name.
func analyzeCertificate(chain []*x509.Certificate, st *tls.ConnectionState, host string, now time.Time, roots *x509.CertPool) *Certificate {
	leaf := chain[0]
	alg, size, curve := publicKeyInfo(leaf)
	c := &Certificate{
		Subject:            leaf.Subject.CommonName,
		Issuer:             leaf.Issuer.CommonName,
		IssuerOrg:          leaf.Issuer.Organization,
		NotBefore:          leaf.NotBefore.UTC(),
		NotAfter:           leaf.NotAfter.UTC(),
		ValidityDays:       int(leaf.NotAfter.Sub(leaf.NotBefore).Hours() / 24),
		DaysToExpiry:       int(leaf.NotAfter.Sub(now).Hours() / 24),
		KeyAlgorithm:       alg,
		KeySize:            size,
		Curve:              curve,
		SignatureAlgorithm: leaf.SignatureAlgorithm.String(),
		Expired:            now.After(leaf.NotAfter),
		NotYetValid:        now.Before(leaf.NotBefore),
		SelfSigned:         bytes.Equal(leaf.RawSubject, leaf.RawIssuer) && leaf.CheckSignatureFrom(leaf) == nil,
		SANs:               append([]string{}, leaf.DNSNames...),
	}
	for _, ip := range leaf.IPAddresses {
		c.SANs = append(c.SANs, ip.String())
	}
	if c.Subject == "" {
		c.Subject = leaf.Subject.String()
	}
	if st != nil {
		c.OCSPStapled = len(st.OCSPResponse) > 0
		c.SCTs = len(st.SignedCertificateTimestamps)
	}

	inter := x509.NewCertPool()
	for _, ic := range chain[1:] {
		inter.AddCert(ic)
	}
	_, err := leaf.Verify(x509.VerifyOptions{
		Roots:         roots,
		Intermediates: inter,
		CurrentTime:   now,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	})
	c.Trusted = err == nil
	if err != nil {
		c.TrustError = err.Error()
	}
	c.HostnameMatch = leaf.VerifyHostname(host) == nil

	for _, cc := range chain {
		a, sz, _ := publicKeyInfo(cc)
		subj := cc.Subject.CommonName
		if subj == "" {
			subj = cc.Subject.String()
		}
		c.Chain = append(c.Chain, ChainCert{
			Subject:            subj,
			Issuer:             cc.Issuer.CommonName,
			KeyAlgorithm:       a,
			KeySize:            sz,
			SignatureAlgorithm: cc.SignatureAlgorithm.String(),
			NotAfter:           cc.NotAfter.UTC(),
		})
	}
	return c
}

// publicKeyInfo returns the key algorithm, its size in bits and, for ECDSA, the curve.
func publicKeyInfo(c *x509.Certificate) (alg string, bits int, curve string) {
	switch k := c.PublicKey.(type) {
	case *rsa.PublicKey:
		return "RSA", k.N.BitLen(), ""
	case *ecdsa.PublicKey:
		p := k.Params()
		return "ECDSA", p.BitSize, p.Name
	case ed25519.PublicKey:
		return "Ed25519", 256, ""
	}
	// Unknown to crypto/x509 (e.g. ML-DSA): report the algorithm Go saw.
	return c.PublicKeyAlgorithm.String(), 0, ""
}
