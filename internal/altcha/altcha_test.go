package altcha

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

var key = []byte(strings.Repeat("k", 32))

// solve does what the browser does.
func solve(t *testing.T, c Challenge) string {
	t.Helper()
	for n := int64(0); n <= c.MaxNumber; n++ {
		if hashHex(c.Salt, n) == c.Challenge {
			return encode(solution{Algorithm: c.Algorithm, Challenge: c.Challenge, Number: n, Salt: c.Salt, Signature: c.Signature})
		}
	}
	t.Fatal("no solution")
	return ""
}

func encode(s solution) string {
	b, _ := json.Marshal(s)
	return base64.StdEncoding.EncodeToString(b)
}

func TestSolveAndVerify(t *testing.T) {
	v, err := New(key, 2000, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	c, err := v.Challenge()
	if err != nil {
		t.Fatal(err)
	}
	if c.Algorithm != "SHA-256" || c.MaxNumber != 2000 || !strings.Contains(c.Salt, "?expires=") || len(c.Signature) != 64 {
		t.Fatalf("challenge: %+v", c)
	}
	p := solve(t, c)
	if err := v.Verify(p); err != nil {
		t.Fatalf("valid solution refused: %v", err)
	}
	if err := v.Verify(p); !errors.Is(err, ErrReplay) {
		t.Errorf("second use: %v", err)
	}
}

func TestForgeries(t *testing.T) {
	v, _ := New(key, 500, time.Minute)
	c, _ := v.Challenge()
	var good solution
	raw, _ := base64.StdEncoding.DecodeString(solve(t, c))
	_ = json.Unmarshal(raw, &good)

	other, _ := New([]byte(strings.Repeat("o", 32)), 500, time.Minute)
	foreign, _ := other.Challenge()

	bad := map[string]string{
		"garbage":      "not base64!",
		"empty":        "",
		"wrong number": encode(func() solution { s := good; s.Number = (s.Number + 1) % 501; return s }()),
		"wrong algo":   encode(func() solution { s := good; s.Algorithm = "SHA-1"; return s }()),
		"unsigned":     encode(func() solution { s := good; s.Signature = strings.Repeat("0", 64); return s }()),
		"later expiry": encode(func() solution { s := good; s.Salt = strings.Split(s.Salt, "?")[0] + "?expires=99999999999"; return s }()),
		"other key":    solve(t, foreign),
		"self-made": encode(func() solution {
			s := solution{Algorithm: Algorithm, Salt: "x?expires=99999999999", Number: 1}
			s.Challenge = hashHex(s.Salt, 1)
			return s
		}()),
		"number too big": encode(func() solution { s := good; s.Number = 501; return s }()),
	}
	for name, p := range bad {
		if err := v.Verify(p); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if err := v.Verify(encode(good)); err != nil { // the forgeries did not burn the real one
		t.Errorf("genuine solution: %v", err)
	}
}

func TestExpiry(t *testing.T) {
	v, _ := New(key, 100, time.Minute)
	now := time.Now()
	v.now = func() time.Time { return now }
	c, _ := v.Challenge()
	p := solve(t, c)
	v.now = func() time.Time { return now.Add(61 * time.Second) }
	if err := v.Verify(p); !errors.Is(err, ErrExpired) {
		t.Errorf("expired challenge: %v", err)
	}
	// Used challenges are forgotten once expired.
	v.now = func() time.Time { return now }
	c2, _ := v.Challenge()
	_ = v.Verify(solve(t, c2))
	v.now = func() time.Time { return now.Add(2 * time.Minute) }
	v.mu.Lock()
	v.sweep(v.now())
	n := len(v.used)
	v.mu.Unlock()
	if n != 0 {
		t.Errorf("%d used challenges kept after expiry", n)
	}
	if _, err := New([]byte("short"), 0, 0); err == nil {
		t.Error("short key accepted")
	}
}
