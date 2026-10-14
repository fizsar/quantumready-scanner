// Command kyber is a TLS 1.3 server whose only post-quantum group is the
// pre-standard X25519Kyber768Draft00, as deployed in 2023–2024 before ML-KEM:
// "PQC, but obsolete". It also accepts x25519, so classical clients connect.
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"log"
	"math/big"
	"net/http"
	"time"
)

// x25519Kyber768Draft00 is unexported in Go 1.23 but enabled by default (GODEBUG tlskyber=1).
const x25519Kyber768Draft00 tls.CurveID = 0x6399

func main() {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		log.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "kyber.tlslab.test"},
		DNSNames:     []string{"kyber"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().AddDate(1, 0, 0),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, key.Public(), key)
	if err != nil {
		log.Fatal(err)
	}
	srv := &http.Server{
		Addr:              ":443",
		ReadHeaderTimeout: 5 * time.Second,
		Handler:           http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("kyber lab\n")) }),
		TLSConfig: &tls.Config{
			MinVersion:       tls.VersionTLS13,
			CurvePreferences: []tls.CurveID{x25519Kyber768Draft00, tls.X25519},
			Certificates:     []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}},
		},
	}
	log.Fatal(srv.ListenAndServeTLS("", ""))
}
