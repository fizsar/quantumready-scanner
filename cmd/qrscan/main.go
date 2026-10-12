// Command qrscan runs one scan from the command line (development tool; not
// shipped in the images).
//
//	go run ./cmd/qrscan quantumready.fizsar.com
//	go run ./cmd/qrscan -json -port 8443 example.com
//
// The godebug settings let crypto/tls complete handshakes with very old
// servers (SHA-1 signatures in TLS 1.2, RSA keys under 1024 bits) so they can
// be analyzed. The scanner only observes these connections and never trusts
// them; cmd/worker will use the same settings (milestone 2.3).
//
//go:debug tlssha1=1
//go:debug rsa1024min=0
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/fizsar/quantumready-saas/internal/scanner"
)

func main() {
	port := flag.Int("port", 443, "TCP port")
	asJSON := flag.Bool("json", false, "print the full result as JSON")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: qrscan [-port N] [-json] host")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}

	r, err := scanner.New(scanner.Options{}).Scan(context.Background(), flag.Arg(0), *port)
	if err != nil {
		fmt.Fprintln(os.Stderr, "qrscan:", err)
		os.Exit(1)
	}
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(r)
		return
	}
	printSummary(r)
}

func printSummary(r *scanner.Result) {
	fmt.Printf("%s:%d (%s) in %d ms\n", r.Host, r.Port, r.IP, r.DurationMS)
	fmt.Printf("default handshake: %s %s %s group=%s\n", r.Default.Status, r.Default.Version, r.Default.CipherSuite, r.Default.Group)
	fmt.Printf("key exchange: selected=%q hybrid=%v\n", r.KeyExchange.Selected, r.KeyExchange.SelectedHybrid)
	for _, g := range r.KeyExchange.Groups {
		fmt.Printf("  %-20s %s\n", g.Name, g.Status)
	}
	fmt.Println("versions:")
	for _, v := range r.Versions {
		fmt.Printf("  %-8s %s\n", v.Version, v.Status)
	}
	fmt.Printf("TLS 1.3 suite: %s\n", r.TLS13Suite)
	fmt.Printf("TLS 1.2 suites (complete=%v):\n", r.TLS12SuitesComplete)
	for _, s := range r.TLS12Suites {
		fmt.Printf("  %-48s fs=%v aead=%v insecure=%v\n", s.Name, s.ForwardSecrecy, s.AEAD, s.Insecure)
	}
	if c := r.Certificate; c != nil {
		fmt.Printf("certificate: %s, issuer %q %v, %s %d %s, sig %s\n", c.Subject, c.Issuer, c.IssuerOrg, c.KeyAlgorithm, c.KeySize, c.Curve, c.SignatureAlgorithm)
		fmt.Printf("  trusted=%v hostname=%v expired=%v validity=%dd expires_in=%dd ocsp=%v scts=%d chain=%d\n",
			c.Trusted, c.HostnameMatch, c.Expired, c.ValidityDays, c.DaysToExpiry, c.OCSPStapled, c.SCTs, len(c.Chain))
		if c.TrustError != "" {
			fmt.Printf("  trust error: %s\n", c.TrustError)
		}
	}
	fmt.Printf("not evaluated: %s\n", strings.Join(r.NotEvaluated, ", "))
}
