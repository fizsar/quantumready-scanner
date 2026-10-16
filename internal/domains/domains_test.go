package domains

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
)

// The embedded list is exactly the pinned one (update it with make psl).
func TestEmbeddedListIsPinned(t *testing.T) {
	sum := sha256.Sum256([]byte(pslData))
	if hex.EncodeToString(sum[:]) != PSLSHA256 {
		t.Fatalf("public_suffix_list.dat does not match psl_version.go: run make psl")
	}
	if len(psl.exact) < 5000 || len(psl.wildcard) == 0 || len(psl.exception) == 0 {
		t.Errorf("rules: %d exact, %d wildcard, %d exceptions", len(psl.exact), len(psl.wildcard), len(psl.exception))
	}
}

func TestPublicSuffix(t *testing.T) {
	for name, want := range map[string]string{
		"example.com":               "com",
		"www.example.co.uk":         "co.uk",
		"foo.github.io":             "github.io", // private section
		"a.b.kawasaki.jp":           "b.kawasaki.jp",
		"city.kawasaki.jp":          "kawasaki.jp",   // exception rule !city.kawasaki.jp
		"xn--85x722f.xn--55qx5d.cn": "xn--55qx5d.cn", // IDN rule in punycode
		"fizsar.com":                "com",
	} {
		if got, known := PublicSuffix(name); got != want || !known {
			t.Errorf("PublicSuffix(%s) = %s, %v; want %s", name, got, known, want)
		}
	}
	if _, known := PublicSuffix("foo.notatld"); known {
		t.Error("unknown TLD reported as known")
	}
}

func TestNormalize(t *testing.T) {
	ok := map[string]string{
		"FizSar.com":                 "fizsar.com",
		" quantumready.fizsar.com. ": "quantumready.fizsar.com",
		"bücher.de":                  "xn--bcher-kva.de",
		"shop.example.co.uk":         "shop.example.co.uk",
		"me.github.io":               "me.github.io", // a registrable name under a private suffix
	}
	for in, want := range ok {
		if got, err := Normalize(in); err != nil || got != want {
			t.Errorf("Normalize(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	bad := map[string]error{
		"com":                ErrInvalid, // one label
		"co.uk":              ErrPublicSuffix,
		"github.io":          ErrPublicSuffix,
		"foo.kawasaki.jp":    ErrPublicSuffix, // *.kawasaki.jp
		"192.0.2.1":          ErrIP,
		"[2001:db8::1]":      ErrIP,
		"2001:db8::1":        ErrIP,
		"fizsar.com:443":     ErrInvalid,
		"https://fizsar.com": ErrInvalid,
		"fizsar.com/x":       ErrInvalid,
		"_dmarc.fizsar.com":  ErrInvalid,
		"-bad.com":           ErrInvalid,
		"a..com":             ErrInvalid,
		"printer.local":      ErrSpecialUse,
		"app.localhost":      ErrSpecialUse,
		"site.test":          ErrSpecialUse,
		"1.2.3.in-addr.arpa": ErrSpecialUse,
		"router.home.arpa":   ErrSpecialUse,
		"x.internal":         ErrSpecialUse,
		"foo.notatld":        ErrUnknownTLD,
		"":                   ErrInvalid,
	}
	for in, want := range bad {
		if got, err := Normalize(in); !errors.Is(err, want) {
			t.Errorf("Normalize(%q) = %q, %v; want %v", in, got, err, want)
		}
	}
}

func TestRegistrable(t *testing.T) {
	for in, want := range map[string]string{
		"fizsar.com": "fizsar.com", "quantumready.fizsar.com": "fizsar.com", "a.b.shop.example.co.uk": "example.co.uk",
		"me.github.io": "me.github.io", "x.me.github.io": "me.github.io", "co.uk": "co.uk",
	} {
		if got := Registrable(in); got != want {
			t.Errorf("Registrable(%s) = %s, want %s", in, got, want)
		}
	}
}

func TestCovers(t *testing.T) {
	for _, tc := range []struct {
		domain, host string
		want         bool
	}{
		{"fizsar.com", "fizsar.com", true}, {"fizsar.com", "quantumready.fizsar.com", true},
		{"fizsar.com", "evilfizsar.com", false}, {"fizsar.com", "fizsar.com.evil.example", false},
		{"quantumready.fizsar.com", "fizsar.com", false},
	} {
		if got := Covers(tc.domain, tc.host); got != tc.want {
			t.Errorf("Covers(%s, %s) = %v", tc.domain, tc.host, got)
		}
	}
}
