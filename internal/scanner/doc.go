// Package scanner is the TLS/PQC scanning engine. It must not depend on HTTP
// handlers or the database.
//
// A scan resolves the target once, refuses non-public addresses (anti-SSRF),
// pins the validated IP for every connection (DNS rebinding) and runs a set of
// handshakes with crypto/tls: a browser-like offer, each hybrid ML-KEM and
// classical group alone, each protocol version alone and an enumeration of the
// TLS 1.2 cipher suites. The certificate chain is captured and verified
// separately. The result holds facts only; internal/scoring judges them.
//
// What crypto/tls cannot negotiate is reported as "not evaluated", never as
// "not supported": see Result.NotEvaluated and docs/scanner.md.
package scanner
