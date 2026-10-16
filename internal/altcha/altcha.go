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
	"fmt"
	"math/big"
	"net/url"
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
// difficulty (0: 100 000) and ttl how long a challenge can be answered (0:
// 10 minutes).
func New(key []byte, maxNumber int64, ttl time.Duration) (*Verifier, error) {
	if len(key) < 32 {
		return nil, errors.New("altcha: the HMAC key must have at least 32 bytes")
	}
	if maxNumber <= 0 {
		maxNumber = 100_000
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
	salt := hex.EncodeToString(b) + "?expires=" + strconv.FormatInt(v.now().Add(v.ttl).Unix(), 10)
	c := hashHex(salt, n.Int64())
	return Challenge{Algorithm: Algorithm, Challenge: c, MaxNumber: v.maxNumber, Salt: salt, Signature: v.sign(c)}, nil
}

// Verify checks a solution (the base64 payload of the browser). Each
// challenge is accepted once.
func (v *Verifier) Verify(payload string) error {
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
	if subtle.ConstantTimeCompare([]byte(hashHex(s.Salt, s.Number)), []byte(s.Challenge)) != 1 {
		return ErrInvalid
	}
	expires, err := expiry(s.Salt)
	if err != nil {
		return ErrInvalid
	}
	now := v.now()
	if !now.Before(expires) {
		return ErrExpired
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	v.sweep(now)
	if _, seen := v.used[s.Challenge]; seen {
		return ErrReplay
	}
	v.used[s.Challenge] = expires
	return nil
}

// expiry reads the ?expires= Unix time that the salt carries (it is part
// of the hashed and signed challenge, so it cannot be changed).
func expiry(salt string) (time.Time, error) {
	i := strings.IndexByte(salt, '?')
	if i < 0 {
		return time.Time{}, errors.New("no parameters")
	}
	q, err := url.ParseQuery(salt[i+1:])
	if err != nil {
		return time.Time{}, err
	}
	sec, err := strconv.ParseInt(q.Get("expires"), 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("expires: %w", err)
	}
	return time.Unix(sec, 0), nil
}

// sweep forgets expired challenges (they fail on expiry anyway), at most
// once a minute; memory stays bounded by the challenges of the last TTL.
func (v *Verifier) sweep(now time.Time) {
	if now.Sub(v.swept) < time.Minute {
		return
	}
	v.swept = now
	for c, exp := range v.used {
		if !now.Before(exp) {
			delete(v.used, c)
		}
	}
}
