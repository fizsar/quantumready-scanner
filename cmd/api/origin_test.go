package main

import "testing"

func TestPublicOrigin(t *testing.T) {
	ok := map[string]string{
		"":                                 "",
		"https://quantumready.fizsar.com":  "https://quantumready.fizsar.com",
		"https://QuantumReady.fizsar.com/": "https://quantumready.fizsar.com",
		"http://localhost:5173":            "http://localhost:5173",
		"http://127.0.0.1:8080":            "http://127.0.0.1:8080",
	}
	for in, want := range ok {
		if got, err := publicOrigin(in); err != nil || got != want {
			t.Errorf("publicOrigin(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"quantumready.fizsar.com", "http://quantumready.fizsar.com", "https://q.example/app",
		"https://q.example?x=1", "https://u:p@q.example", "ftp://localhost"} {
		if got, err := publicOrigin(in); err == nil {
			t.Errorf("publicOrigin(%q) = %q, accepted", in, got)
		}
	}
}
