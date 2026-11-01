package altcha

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"
)

// solveN finds the number of a challenge (the browser's work).
func solveN(t *testing.T, c Challenge) int64 {
	t.Helper()
	for n := int64(0); n <= c.MaxNumber; n++ {
		if hashHex(c.Salt, n) == c.Challenge {
			return n
		}
	}
	t.Fatal("no solution")
	return 0
}

// TestSaltSpliceCannotExtendExpiry: the challenge is SHA-256(salt + n) and
// only the challenge is signed, so the digits of n must not be movable to
// the end of the salt, where they would extend ?expires= (the hashed string,
// and so the signed challenge, stays the same).
func TestSaltSpliceCannotExtendExpiry(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	v, _ := New(key, 50_000, time.Minute)
	v.now = func() time.Time { return now }
	var c Challenge
	var n int64
	for { // a challenge whose number has at least two digits (almost always)
		c, _ = v.Challenge()
		if n = solveN(t, c); n >= 10 {
			break
		}
	}
	digits := strconv.FormatInt(n, 10)
	last, _ := strconv.ParseInt(digits[len(digits)-1:], 10, 64)
	forged := solution{Algorithm: Algorithm, Challenge: c.Challenge, Number: last,
		Salt: c.Salt + digits[:len(digits)-1], Signature: c.Signature}

	now = now.Add(time.Hour) // the real challenge expired 59 minutes ago
	if err := v.Verify(encode(forged)); err == nil || errors.Is(err, ErrReplay) {
		t.Fatalf("expired challenge accepted with the salt %q and number %d (err %v)", forged.Salt, forged.Number, err)
	}
	// Within its life too: a spliced salt is never a valid one.
	now = now.Add(-time.Hour)
	if err := v.Verify(encode(forged)); !errors.Is(err, ErrInvalid) {
		t.Errorf("spliced salt within the TTL: %v, want ErrInvalid", err)
	}
}

// The salt has one exact form: 24 hex digits, ?expires= with 10 digits and
// a final &, so nothing can be appended to it.
func TestSaltFormat(t *testing.T) {
	v, _ := New(key, 100, time.Minute)
	c, _ := v.Challenge()
	if !saltRe.MatchString(c.Salt) || !strings.HasSuffix(c.Salt, "&") {
		t.Fatalf("issued salt %q", c.Salt)
	}
	base := c.Salt[:len(c.Salt)-1] // without the final &
	for name, salt := range map[string]string{
		"no separator":   base,
		"extra digit":    base + "1&",
		"extra param":    base + "&x=1&",
		"upper-case hex": strings.ToUpper(c.Salt[:24]) + c.Salt[24:],
		"short random":   c.Salt[2:],
	} {
		// Self-consistent hash, real signature over it: only the format is wrong.
		s := solution{Algorithm: Algorithm, Salt: salt, Number: 1}
		s.Challenge = hashHex(salt, 1)
		s.Signature = v.sign(s.Challenge)
		if err := v.Verify(encode(s)); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s (%q): %v, want ErrInvalid", name, salt, err)
		}
	}
}

// A signed challenge never expires later than now + TTL (a key shared with
// a misconfigured issuer, or a clock that jumped back, cannot mint long-lived
// proofs), and used challenges leave memory once expired.
func TestExpiryBoundedAndUsedSwept(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	v, _ := New(key, 100, time.Minute)
	v.now = func() time.Time { return now }

	far := solution{Algorithm: Algorithm, Salt: strings.Repeat("a", 24) + "?expires=" + strconv.FormatInt(now.Add(time.Hour).Unix(), 10) + "&", Number: 7}
	far.Challenge = hashHex(far.Salt, far.Number)
	far.Signature = v.sign(far.Challenge)
	if err := v.Verify(encode(far)); !errors.Is(err, ErrInvalid) {
		t.Errorf("expiry an hour ahead with a one-minute TTL: %v, want ErrInvalid", err)
	}

	for range 50 {
		c, _ := v.Challenge()
		if err := v.Verify(encode(solution{Algorithm: Algorithm, Challenge: c.Challenge, Number: solveN(t, c), Salt: c.Salt, Signature: c.Signature})); err != nil {
			t.Fatal(err)
		}
	}
	if len(v.used) != 50 {
		t.Fatalf("used: %d, want 50", len(v.used))
	}
	now = now.Add(2 * time.Minute) // all expired; the next Verify sweeps
	_ = v.Verify("x")
	if len(v.used) != 0 {
		t.Errorf("used keeps %d expired challenges", len(v.used))
	}
}
