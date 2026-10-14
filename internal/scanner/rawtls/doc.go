// Package rawtls builds TLS and SSLv2 ClientHello messages by hand and reads
// only the server's first answer (ServerHello, HelloRetryRequest, alert and,
// on request, the rest of the first flight up to ServerKeyExchange). It never
// sends anything after the ClientHello, so no handshake is ever completed.
//
// It covers what crypto/tls cannot offer: SSLv2 and SSLv3, every IANA cipher
// suite (DHE, CCM, Camellia, ARIA, NULL, EXPORT, anonymous…), the full TLS 1.3
// suite list and every IANA group (ffdhe, brainpool, binary curves, pure
// ML-KEM, pre-standard Kyber). It is a pure protocol library: dialing,
// anti-SSRF, timeouts and concurrency belong to the caller (internal/scanner).
//
// Server answers are untrusted input: every read is bounded (MaxResponseBytes,
// record and message sizes) and every length is checked before use. Anything
// that does not parse is an error wrapping ErrMalformed; a parameter we did not
// offer is an error wrapping ErrUnoffered. Neither must be read as "not
// supported" by the caller.
package rawtls
