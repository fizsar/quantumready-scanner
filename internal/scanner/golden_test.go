package scanner

import (
	"crypto/tls"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// goldenServers are deterministic local servers whose basic-intensity results
// are pinned in testdata/basic_golden.json. The file was written by the
// scanner before the raw probes existed (milestone 2.7.3), so this test
// proves that intensity "basic" still returns exactly the same facts.
func goldenServers(t *testing.T) (*testPKI, map[string]int) {
	pki := newTestPKI(t)
	ecdsaCert := pki.leaf(t, leafOpts{})
	rsaCert := pki.leaf(t, leafOpts{key: "rsa2048"})
	return pki, map[string]int{
		"modern": startTLS(t, &tls.Config{Certificates: []tls.Certificate{ecdsaCert},
			CurvePreferences: []tls.CurveID{tls.X25519MLKEM768, tls.X25519, tls.CurveP256}}),
		"tls12_only": startTLS(t, &tls.Config{Certificates: []tls.Certificate{rsaCert}, MaxVersion: tls.VersionTLS12}),
		"legacy": startTLS(t, &tls.Config{Certificates: []tls.Certificate{rsaCert}, MinVersion: tls.VersionTLS10, MaxVersion: tls.VersionTLS12,
			CipherSuites: []uint16{tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA, tls.TLS_RSA_WITH_AES_128_CBC_SHA, tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256}}),
	}
}

// goldenNormalize turns a result into comparable JSON: the port is random,
// and fields added after the golden file (all optional) are dropped, with
// schema_version set back to 1: so the test also proves that schema 2 only
// adds fields.
func goldenNormalize(t *testing.T, r *Result, dropNew bool) map[string]any {
	t.Helper()
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	m["port"] = float64(0) // as encoding/json decodes numbers
	if dropNew {
		for _, k := range newOptionalFields {
			delete(m, k)
		}
		dropKeys(m, schema2Keys)
		m["schema_version"] = float64(1)
	}
	return m
}

// newOptionalFields are the Result fields added in milestone 2.7.3.
var newOptionalFields = []string{"intensity", "connections", "raw"}

// schema2Keys are the keys schema 2 (milestone 2.8.4) adds, at any depth.
var schema2Keys = []string{"probe_log", "error_code", "trust_error_code", "signature_oid", "public_key_oid"}

func dropKeys(v any, keys []string) {
	switch x := v.(type) {
	case map[string]any:
		for _, k := range keys {
			delete(x, k)
		}
		for _, e := range x {
			dropKeys(e, keys)
		}
	case []any:
		for _, e := range x {
			dropKeys(e, keys)
		}
	}
}

func TestBasicIntensityMatchesGolden(t *testing.T) {
	path := filepath.Join("testdata", "basic_golden.json")
	got := map[string]map[string]any{}
	pki, ports := goldenServers(t)
	for name, port := range ports {
		r := scan(t, newTestScanner(pki, Options{}), port)
		got[name] = goldenNormalize(t, r, true)
	}
	if os.Getenv("QR_UPDATE_GOLDEN") == "1" {
		b, _ := json.MarshalIndent(got, "", "  ")
		if err := os.WriteFile(path, append(b, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Skip("golden file written")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var want map[string]map[string]any
	if err := json.Unmarshal(b, &want); err != nil {
		t.Fatal(err)
	}
	for name := range want {
		if !reflect.DeepEqual(got[name], want[name]) {
			g, _ := json.MarshalIndent(got[name], "", "  ")
			w, _ := json.MarshalIndent(want[name], "", "  ")
			t.Errorf("%s: basic result differs from the golden file\n--- got\n%s\n--- want\n%s", name, g, w)
		}
	}
}
