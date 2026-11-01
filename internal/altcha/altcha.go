// Package altcha is a self-hosted proof of work in the ALTCHA format
// (altcha.org): the server hands out a signed challenge, the browser finds
// the number n such that SHA-256(salt + n) equals it (on average maxnumber/2
// hashes, about a second), and the server checks the answer: the signature
// (HMAC-SHA-256 with a key that never leaves the server), the expiry inside
// the salt, and that the same challenge is not answered twice. No third
// party is involved and nothing is stored but recently used challenges.
package altcha

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Algorithm is the only one accepted.
const Algorithm = "SHA-256"

// Errors of Verify.
var (
	ErrInvalid = errors.New("altcha: invalid solution")
	ErrExpired = errors.New("altcha: challenge expired")
	ErrReplay  = errors.New("altcha: challenge already used")
)

// Challenge is sent to the browser.
type Challenge struct {
	Algorithm string `json:"algorithm"`
	Challenge string `json:"challenge"`
	MaxNumber int64  `json:"maxnumber"`
	Salt      string `json:"salt"`
	Signature string `json:"signature"`
}

// solution is what the browser sends back, base64-encoded JSON.
type solution struct {
	Algorithm string `json:"algorithm"`
	Challenge string `json:"challenge"`
	Number    int64  `json:"number"`
	Salt      string `json:"salt"`
	Signature string `json:"signature"`
}

// Verifier issues and checks challenges.
type Verifier struct {
	key       []byte
	maxNumber int64
	ttl       time.Duration
	now       func() time.Time

	mu    sync.Mutex
	used  map[string]time.Time // challenge → expiry
	swept time.Time
}

// New returns a verifier. key must be at least 32 bytes; maxNumber sets the
// difficulty (0: 50 000; about 1 s on average in a browser) and ttl how long a challenge can be answered (0:
// 10 minutes).
func New(key []byte, maxNumber int64, ttl time.Duration) (*Verifier, error) {
	if len(key) < 32 {
		return nil, errors.New("altcha: the HMAC key must have at least 32 bytes")
	}
	if maxNumber <= 0 {
		maxNumber = 50_000
	}
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	return &Verifier{key: key, maxNumber: maxNumber, ttl: ttl, now: time.Now, used: map[string]time.Time{}}, nil
}

func hashHex(salt string, n int64) string {
	h := sha256.Sum256([]byte(salt + strconv.FormatInt(n, 10)))
	return hex.EncodeToString(h[:])
}

func (v *Verifier) sign(challenge string) string {
	m := hmac.New(sha256.New, v.key)
	m.Write([]byte(challenge))
	return hex.EncodeToString(m.Sum(nil))
}

// Challenge returns a new challenge.
func (v *Verifier) Challenge() (Challenge, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return Challenge{}, err
	}
	n, err := rand.Int(rand.Reader, big.NewInt(v.maxNumber+1))
	if err != nil {
		return Challenge{}, err
	}
	// The final & ends the salt: the digits of n, hashed right after it,
	// can never be read as part of ?expires= (see saltRe).
	salt := hex.EncodeToString(b) + "?expires=" + strconv.FormatInt(v.now().Add(v.ttl).Unix(), 10) + "&"
	c := hashHex(salt, n.Int64())
	return Challenge{Algorithm: Algorithm, Challenge: c, MaxNumber: v.maxNumber, Salt: salt, Signature: v.sign(c)}, nil
}

// Verify checks a solution (the base64 payload of the browser). Each
// challenge is accepted once.
func (v *Verifier) Verify(payload string) error {
	v.mu.Lock()
	v.sweep(v.now()) // on every call, valid or not: used never holds expired entries for long
	v.mu.Unlock()
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(payload))
	if err != nil || len(raw) > 1024 {
		return ErrInvalid
	}
	var s solution
	if err := json.Unmarshal(raw, &s); err != nil || s.Algorithm != Algorithm || s.Number < 0 || s.Number > v.maxNumber {
		return ErrInvalid
	}
	// Signature first: only challenges we issued are worth hashing.
	if !hmac.Equal([]byte(v.sign(s.Challenge)), []byte(s.Signature)) {
		return ErrInvalid
	}
	// Only the challenge is signed, and it is SHA-256(salt + n): without one
	// exact salt form, digits of n could be moved into ?expires= (same hash,
	// later expiry).
	m := saltRe.FindStringSubmatch(s.Salt)
	if m == nil {
		return ErrInvalid
	}
	if subtle.ConstantTimeCompare([]byte(hashHex(s.Salt, s.Number)), []byte(s.Challenge)) != 1 {
		return ErrInvalid
	}
	sec, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil {
		return ErrInvalid
	}
	expires := time.Unix(sec, 0)
	now := v.now()
	if expires.After(now.Add(v.ttl)) { // never issued by us: no proof outlives the TTL
		return ErrInvalid
	}
	if !now.Before(expires) {
		return ErrExpired
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if _, seen := v.used[s.Challenge]; seen {
		return ErrReplay
	}
	v.used[s.Challenge] = expires
	return nil
}

// saltRe is the only salt Challenge issues: 12 random bytes in hex, the
// ?expires= Unix time (10 digits) and a final &. Nothing can be appended
// to it, so the expiry hashed into the signed challenge is the one read.
var saltRe = regexp.MustCompile(`^[0-9a-f]{24}\?expires=([0-9]{10})&$`)

// sweep forgets expired challenges (they fail on expiry anyway), at most
// once a minute; memory stays bounded by the challenges of the last TTL,
// since no accepted expiry is later than now + TTL.
func (v *Verifier) sweep(now time.Time) {
	if d := now.Sub(v.swept); d >= 0 && d < time.Minute { // a clock that went back sweeps at once
		return
	}
	v.swept = now
	for c, exp := range v.used {
		if !now.Before(exp) {
			delete(v.used, c)
		}
	}
}
