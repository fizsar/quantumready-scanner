package scanner

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/asn1"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"time"

	"golang.org/x/crypto/cryptobyte"
	cbasn1 "golang.org/x/crypto/cryptobyte/asn1"
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
		SelfSigned:         selfSigned(leaf),
		SANs:               append([]string{}, leaf.DNSNames...),
	}
	for _, ip := range leaf.IPAddresses {
		c.SANs = append(c.SANs, ip.String())
	}
	if c.Subject == "" {
		c.Subject = leaf.Subject.String()
	}
	// Post-quantum algorithms by their OID: crypto/x509 says only "ML-DSA"
	// for the key and does not know SLH-DSA at all.
	leafSig, leafKey := algorithmOIDs(leaf)
	if name, ok := pqAlgorithms[leafKey]; ok {
		c.KeyAlgorithm, c.KeySize, c.Curve = name, 0, ""
	}
	if name, ok := pqAlgorithms[leafSig]; ok {
		c.SignatureAlgorithm = name
	}
	c.SCTsEmbedded = countEmbeddedSCTs(leaf)
	if st != nil {
		c.OCSPStapled = len(st.OCSPResponse) > 0
		c.SCTsHandshake = len(st.SignedCertificateTimestamps)
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
		c.TrustError, c.TrustErrorCode = err.Error(), trustErrorCode(err)
	}
	c.HostnameMatch = leaf.VerifyHostname(host) == nil

	for _, cc := range chain {
		a, sz, _ := publicKeyInfo(cc)
		subj := cc.Subject.CommonName
		if subj == "" {
			subj = cc.Subject.String()
		}
		sigOID, keyOID := algorithmOIDs(cc)
		if name, ok := pqAlgorithms[keyOID]; ok {
			a, sz = name, 0
		}
		sigName := cc.SignatureAlgorithm.String()
		if name, ok := pqAlgorithms[sigOID]; ok {
			sigName = name
		}
		c.Chain = append(c.Chain, ChainCert{
			Subject:            subj,
			Issuer:             cc.Issuer.CommonName,
			KeyAlgorithm:       a,
			KeySize:            sz,
			SignatureAlgorithm: sigName,
			NotAfter:           cc.NotAfter.UTC(),
			SignatureOID:       sigOID,
			PublicKeyOID:       keyOID,
		})
	}
	return c
}

// selfSigned reports whether c is signed with its own key under its own name.
// Not CheckSignatureFrom(c): that also wants c to be a CA, and a self-signed
// leaf usually is not (until 2.8.7 those counted as merely untrusted).
func selfSigned(c *x509.Certificate) bool {
	return bytes.Equal(c.RawSubject, c.RawIssuer) && c.CheckSignature(c.SignatureAlgorithm, c.RawTBSCertificate, c.Signature) == nil
}

// algorithmOIDs reads, from the certificate as sent, the OID of its signature
// algorithm (tbsCertificate.signature) and of its public key
// (subjectPublicKeyInfo.algorithm), "" if they do not parse. crypto/x509 maps
// them to its own constants, and to "unknown" for any it does not support.
func algorithmOIDs(c *x509.Certificate) (sig, key string) {
	tbs := cryptobyte.String(c.RawTBSCertificate)
	var body, alg cryptobyte.String
	var oid asn1.ObjectIdentifier
	if tbs.ReadASN1(&body, cbasn1.SEQUENCE) &&
		body.SkipOptionalASN1(cbasn1.Tag(0).Constructed().ContextSpecific()) && // version
		body.SkipASN1(cbasn1.INTEGER) && // serialNumber
		body.ReadASN1(&alg, cbasn1.SEQUENCE) && alg.ReadASN1ObjectIdentifier(&oid) {
		sig = oid.String()
	}
	spki := cryptobyte.String(c.RawSubjectPublicKeyInfo)
	if spki.ReadASN1(&body, cbasn1.SEQUENCE) && body.ReadASN1(&alg, cbasn1.SEQUENCE) && alg.ReadASN1ObjectIdentifier(&oid) {
		key = oid.String()
	}
	return sig, key
}

// oidsOfDER is algorithmOIDs for a certificate as sent (DER), parsed or not
// by crypto/x509: the TBSCertificate's signature algorithm and its
// subjectPublicKeyInfo algorithm.
func oidsOfDER(der []byte) (sig, key string) {
	cert := cryptobyte.String(der)
	var outer, tbs, alg, spki cryptobyte.String
	var oid asn1.ObjectIdentifier
	if !cert.ReadASN1(&outer, cbasn1.SEQUENCE) || !outer.ReadASN1(&tbs, cbasn1.SEQUENCE) ||
		!tbs.SkipOptionalASN1(cbasn1.Tag(0).Constructed().ContextSpecific()) || !tbs.SkipASN1(cbasn1.INTEGER) ||
		!tbs.ReadASN1(&alg, cbasn1.SEQUENCE) || !alg.ReadASN1ObjectIdentifier(&oid) {
		return "", ""
	}
	sig = oid.String()
	ok := true
	for range 3 { // issuer, validity, subject; then subjectPublicKeyInfo
		ok = ok && tbs.SkipASN1(cbasn1.SEQUENCE)
	}
	if ok && tbs.ReadASN1(&spki, cbasn1.SEQUENCE) && spki.ReadASN1(&alg, cbasn1.SEQUENCE) && alg.ReadASN1ObjectIdentifier(&oid) {
		key = oid.String()
	}
	return sig, key
}

// pqAlgorithms are the post-quantum signature algorithms by OID (NIST
// Computer Security Objects Register, 2.16.840.1.101.3.4.3): ML-DSA (FIPS
// 204) and SLH-DSA (FIPS 205), pure and pre-hash. In a certificate the same
// OID names the key (subjectPublicKeyInfo) and the signature.
var pqAlgorithms = func() map[string]string {
	names := map[int]string{
		17: "ML-DSA-44", 18: "ML-DSA-65", 19: "ML-DSA-87",
		20: "SLH-DSA-SHA2-128s", 21: "SLH-DSA-SHA2-128f", 22: "SLH-DSA-SHA2-192s", 23: "SLH-DSA-SHA2-192f",
		24: "SLH-DSA-SHA2-256s", 25: "SLH-DSA-SHA2-256f", 26: "SLH-DSA-SHAKE-128s", 27: "SLH-DSA-SHAKE-128f",
		28: "SLH-DSA-SHAKE-192s", 29: "SLH-DSA-SHAKE-192f", 30: "SLH-DSA-SHAKE-256s", 31: "SLH-DSA-SHAKE-256f",
		32: "HashML-DSA-44-SHA512", 33: "HashML-DSA-65-SHA512", 34: "HashML-DSA-87-SHA512",
		35: "HashSLH-DSA-SHA2-128s-SHA256", 36: "HashSLH-DSA-SHA2-128f-SHA256", 37: "HashSLH-DSA-SHA2-192s-SHA512",
		38: "HashSLH-DSA-SHA2-192f-SHA512", 39: "HashSLH-DSA-SHA2-256s-SHA512", 40: "HashSLH-DSA-SHA2-256f-SHA512",
		41: "HashSLH-DSA-SHAKE-128s-SHAKE128", 42: "HashSLH-DSA-SHAKE-128f-SHAKE128", 43: "HashSLH-DSA-SHAKE-192s-SHAKE256",
		44: "HashSLH-DSA-SHAKE-192f-SHAKE256", 45: "HashSLH-DSA-SHAKE-256s-SHAKE256", 46: "HashSLH-DSA-SHAKE-256f-SHAKE256",
	}
	m := map[string]string{}
	for n, name := range names {
		m[fmt.Sprintf("2.16.840.1.101.3.4.3.%d", n)] = name
	}
	return m
}()

// PostQuantumOID reports whether oid is an ML-DSA or SLH-DSA algorithm, and its name.
func PostQuantumOID(oid string) (string, bool) {
	name, ok := pqAlgorithms[oid]
	return name, ok
}

// classicalAlgorithms names the usual classical key and signature OIDs.
var classicalAlgorithms = map[string]string{
	"1.2.840.10045.2.1": "ECDSA", "1.2.840.113549.1.1.1": "RSA", "1.2.840.113549.1.1.10": "RSA-PSS", "1.3.101.112": "Ed25519", "1.3.101.113": "Ed448",
	"1.2.840.10045.4.3.2": "ECDSA-SHA256", "1.2.840.10045.4.3.3": "ECDSA-SHA384", "1.2.840.10045.4.3.4": "ECDSA-SHA512",
	"1.2.840.113549.1.1.11": "SHA256-RSA", "1.2.840.113549.1.1.12": "SHA384-RSA", "1.2.840.113549.1.1.13": "SHA512-RSA",
	"1.2.840.113549.1.1.5": "SHA1-RSA", "1.2.840.10045.4.1": "ECDSA-SHA1",
}

func algorithmName(oid string) string {
	if name, ok := pqAlgorithms[oid]; ok {
		return name
	}
	return classicalAlgorithms[oid] // "" when unknown: the OID says it
}

// presented describes a chain as a server presented it (DER, leaf first).
func presented(chain [][]byte) *PresentedCert {
	if len(chain) == 0 {
		return nil
	}
	sig, key := oidsOfDER(chain[0])
	sum := sha256.Sum256(chain[0])
	p := &PresentedCert{
		KeyAlgorithm: algorithmName(key), KeyOID: key, SignatureAlgorithm: algorithmName(sig), SignatureOID: sig,
		SHA256: hex.EncodeToString(sum[:]),
	}
	_, p.PostQuantum = pqAlgorithms[key]
	for _, der := range chain {
		s, k := oidsOfDER(der)
		_, pqSig := pqAlgorithms[s]
		_, pqKey := pqAlgorithms[k]
		p.PostQuantumChain = p.PostQuantumChain || pqSig || pqKey
	}
	return p
}

// oidSCTList is the X.509 extension carrying embedded SCTs (RFC 6962, 3.3).
var oidSCTList = asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 11129, 2, 4, 2}

// countEmbeddedSCTs counts the SCTs in the certificate's SCT list extension:
// an OCTET STRING holding a TLS-encoded list (2-byte length, then entries
// each prefixed by a 2-byte length). Malformed lists count what parsed.
func countEmbeddedSCTs(c *x509.Certificate) int {
	for _, ext := range c.Extensions {
		if !ext.Id.Equal(oidSCTList) {
			continue
		}
		var list []byte
		if _, err := asn1.Unmarshal(ext.Value, &list); err != nil || len(list) < 2 {
			return 0
		}
		body := list[2:]
		if n := int(binary.BigEndian.Uint16(list)); n < len(body) {
			body = body[:n]
		}
		count := 0
		for len(body) >= 2 {
			n := int(binary.BigEndian.Uint16(body))
			if n == 0 || len(body) < 2+n {
				break
			}
			count++
			body = body[2+n:]
		}
		return count
	}
	return 0
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
