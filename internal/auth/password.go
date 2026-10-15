package auth

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/subtle"
	_ "embed"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

// argon2id parameters: OWASP Password Storage Cheat Sheet (19 MiB, 2
// passes, 1 thread). Stored hashes carry their parameters (PHC format), so
// changing these only affects new hashes; Verify reports old ones for rehash.
const (
	argonMemoryKiB = 19 * 1024
	argonTime      = 2
	argonThreads   = 1
	argonKeyLen    = 32
	argonSaltLen   = 16
)

// Password length limits (characters). NIST SP 800-63B: at least 8, and
// allow long passphrases; we ask for 12.
const (
	MinPasswordLen = 12
	MaxPasswordLen = 128
)

// Password policy errors (stable codes for the API).
var (
	ErrPasswordTooShort = errors.New("password_too_short")
	ErrPasswordTooLong  = errors.New("password_too_long")
	ErrPasswordCommon   = errors.New("password_common")
	ErrPasswordEmail    = errors.New("password_contains_email")
)

// commonPasswords: the NCSC list of the 100,000 most used passwords
// (SecLists, MIT license, Passwords/Common-Credentials/
// 100k-most-used-passwords-NCSC.txt at commit 1a7bb912, source SHA-256
// c2e56968…c576e0), lowercased, keeping only entries of 12+ characters:
// shorter ones fail the length rule anyway. Checked offline: no password, or
// part of one, leaves the server.
//
//go:embed common-passwords.txt
var commonPasswordsTxt []byte

var commonPasswords = func() map[string]bool {
	m := map[string]bool{}
	s := bufio.NewScanner(bytes.NewReader(commonPasswordsTxt))
	for s.Scan() {
		if line := strings.TrimSpace(s.Text()); line != "" {
			m[line] = true
		}
	}
	return m
}()

// CheckPassword applies the password policy: length, not a common password,
// not containing the local part of the email.
func CheckPassword(password, email string) error {
	n := utf8.RuneCountInString(password)
	switch {
	case n < MinPasswordLen:
		return ErrPasswordTooShort
	case n > MaxPasswordLen:
		return ErrPasswordTooLong
	case commonPasswords[strings.ToLower(password)]:
		return ErrPasswordCommon
	}
	if local, _, ok := strings.Cut(strings.ToLower(email), "@"); ok && len(local) >= 4 &&
		strings.Contains(strings.ToLower(password), local) {
		return ErrPasswordEmail
	}
	return nil
}

// HashPassword returns an argon2id hash in PHC string format.
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemoryKiB, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, argonMemoryKiB, argonTime, argonThreads,
		b64.EncodeToString(salt), b64.EncodeToString(key)), nil
}

var b64 = base64.RawStdEncoding

// VerifyPassword checks password against an encoded hash in constant time.
// needsRehash reports a hash made with other parameters (rehash on login).
func VerifyPassword(password, encoded string) (ok, needsRehash bool, err error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, false, errors.New("auth: not an argon2id hash")
	}
	var version int
	var mem, iters uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false, false, errors.New("auth: unsupported argon2 version")
	}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &mem, &iters, &threads); err != nil || threads == 0 {
		return false, false, errors.New("auth: bad argon2 parameters")
	}
	salt, err1 := b64.DecodeString(parts[4])
	want, err2 := b64.DecodeString(parts[5])
	if err1 != nil || err2 != nil || len(want) == 0 || len(want) > 128 {
		return false, false, errors.New("auth: bad argon2 hash encoding")
	}
	got := argon2.IDKey([]byte(password), salt, iters, mem, threads, uint32(len(want))) //nolint:gosec // len ≤ 128
	ok = subtle.ConstantTimeCompare(got, want) == 1
	needsRehash = mem != argonMemoryKiB || iters != argonTime || threads != argonThreads || len(want) != argonKeyLen
	return ok, needsRehash, nil
}

// dummyHash is verified against when the account does not exist, so a
// login takes the same time whether or not the email is registered.
var dummyHash = func() string {
	h, err := HashPassword("dummy password, never matches anything real")
	if err != nil {
		panic(err)
	}
	return h
}()

// VerifyDummy burns the same time as a real verification and returns false.
func VerifyDummy(password string) bool {
	_, _, _ = VerifyPassword(password, dummyHash)
	return false
}
