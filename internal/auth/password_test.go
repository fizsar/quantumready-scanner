package auth

import (
	"errors"
	"strings"
	"testing"
)

func TestHashAndVerify(t *testing.T) {
	h, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Errorf("hash = %s", h)
	}
	if ok, rehash, err := VerifyPassword("correct horse battery staple", h); !ok || rehash || err != nil {
		t.Errorf("right password: ok=%v rehash=%v err=%v", ok, rehash, err)
	}
	if ok, _, _ := VerifyPassword("correct horse battery stapl", h); ok {
		t.Error("wrong password accepted")
	}
	h2, _ := HashPassword("correct horse battery staple")
	if h == h2 {
		t.Error("same hash twice: no salt")
	}
	// Old parameters still verify, and ask for a rehash.
	old := strings.Replace(h, "m=19456,t=2", "m=19456,t=3", 1)
	if _, rehash, _ := VerifyPassword("x", old); !rehash {
		t.Error("other parameters do not ask for a rehash")
	}
	for _, bad := range []string{"", "$2a$10$abc", "$argon2id$v=19$m=1,t=1,p=0$c2FsdA$aGFzaA", "$argon2id$v=19$m=1,t=1,p=1$***$aGFzaA"} {
		if ok, _, err := VerifyPassword("x", bad); ok || err == nil {
			t.Errorf("VerifyPassword(%q) = %v, %v", bad, ok, err)
		}
	}
	if VerifyDummy("anything") {
		t.Error("VerifyDummy returned true")
	}
}

func TestCheckPassword(t *testing.T) {
	for _, tc := range []struct {
		pw, email string
		want      error
	}{
		{"correct horse battery staple", "ana@example.com", nil},
		{"short", "ana@example.com", ErrPasswordTooShort},
		{"ñandú-ñandú", "ana@example.com", ErrPasswordTooShort}, // 11 characters, 13 bytes
		{strings.Repeat("a", 129), "ana@example.com", ErrPasswordTooLong},
		{"123456789012", "ana@example.com", ErrPasswordCommon},
		{"QWERTYUIOPASDFGHJKL", "ana@example.com", ErrPasswordCommon}, // case-insensitive
		{"usuario-2026!", "usuario@example.com", ErrPasswordEmail},
		{"ana-is-a-good-name", "ana@example.com", nil}, // local parts under 4 characters are not checked
	} {
		if err := CheckPassword(tc.pw, tc.email); !errors.Is(err, tc.want) {
			t.Errorf("CheckPassword(%q) = %v, want %v", tc.pw, err, tc.want)
		}
	}
	if len(commonPasswords) < 1000 {
		t.Errorf("only %d common passwords loaded", len(commonPasswords))
	}
}

func TestNewToken(t *testing.T) {
	a, ha, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	b, _, _ := NewToken()
	if a == b || len(a) != 43 || len(ha) != 32 || string(HashToken(a)) != string(ha) {
		t.Errorf("tokens %q %q, hash %x", a, b, ha)
	}
}
