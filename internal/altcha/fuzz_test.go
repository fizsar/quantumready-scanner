package altcha

import (
	"encoding/base64"
	"encoding/json"
	"strconv"
	"testing"
	"time"
)

// FuzzVerify: whatever comes in, Verify never panics and accepts only a
// solution of a challenge this key signed, in the exact salt form, with the
// right number and an expiry within now + TTL; the same payload never twice.
func FuzzVerify(f *testing.F) {
	now := time.Unix(1_800_000_000, 0)
	v, _ := New(key, 50, time.Minute)
	v.now = func() time.Time { return now }
	for range 4 {
		c, _ := v.Challenge()
		for n := int64(0); n <= c.MaxNumber; n++ {
			if hashHex(c.Salt, n) == c.Challenge {
				good := solution{Algorithm: Algorithm, Challenge: c.Challenge, Number: n, Salt: c.Salt, Signature: c.Signature}
				f.Add(encode(good))
				// The spliced form of the review (digits moved into the salt).
				s := strconv.FormatInt(n, 10)
				f.Add(encode(solution{Algorithm: Algorithm, Challenge: c.Challenge, Number: n % 10, Salt: c.Salt + s[:len(s)-1], Signature: c.Signature}))
				break
			}
		}
	}
	f.Add("")
	f.Add("e30=") // {}
	f.Fuzz(func(t *testing.T, payload string) {
		fresh, _ := New(key, 50, time.Minute)
		fresh.now = v.now
		if fresh.Verify(payload) != nil {
			return
		}
		raw, err := base64.StdEncoding.DecodeString(payload)
		var s solution
		if err != nil || json.Unmarshal(raw, &s) != nil {
			t.Fatalf("accepted an undecodable payload %q", payload)
		}
		m := saltRe.FindStringSubmatch(s.Salt)
		if m == nil || s.Signature != fresh.sign(s.Challenge) || hashHex(s.Salt, s.Number) != s.Challenge {
			t.Fatalf("accepted a solution that is not ours: %+v", s)
		}
		sec, _ := strconv.ParseInt(m[1], 10, 64)
		if exp := time.Unix(sec, 0); !now.Before(exp) || exp.After(now.Add(time.Minute)) {
			t.Fatalf("accepted an expiry outside (now, now + TTL]: %v", exp)
		}
		if fresh.Verify(payload) == nil {
			t.Fatalf("accepted the same payload twice")
		}
	})
}
