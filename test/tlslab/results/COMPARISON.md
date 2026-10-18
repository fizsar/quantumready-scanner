## Mediciones (intensidad full y basic)

| Servidor | full: conexiones | full: sondas en crudo | full: duración (local) | full: duración (RTT 100 ms) | basic: conexiones | basic: duración (local) | basic: duración (RTT 100 ms) |
|---|---|---|---|---|---|---|---|
| `ssl2` | 21 | 6 | 17 ms | 1618 ms | 15 | 3 ms | 1009 ms |
| `ssl3` | 45 | 30 | 145 ms | 6615 ms | 15 | 49 ms | 1045 ms |
| `export` | 53 | 38 | 55 ms | 8102 ms | 15 | 4 ms | 1012 ms |
| `dhe1024` | 31 | 16 | 97 ms | 3697 ms | 15 | 17 ms | 1009 ms |
| `tls10` | 30 | 11 | 137 ms | 3461 ms | 19 | 43 ms | 1830 ms |
| `secp224` | 31 | 16 | 57 ms | 3666 ms | 15 | 5 ms | 1008 ms |
| `tls12` | 49 | 33 | 36 ms | 7074 ms | 16 | 8 ms | 1015 ms |
| `tls13` | 32 | 17 | 21 ms | 3432 ms | 15 | 9 ms | 1013 ms |
| `modern` | 29 | 11 | 28 ms | 2433 ms | 18 | 16 ms | 1430 ms |
| `kyber` | 26 | 11 | 9 ms | 2218 ms | 15 | 8 ms | 1009 ms |

## Comparación con testssl.sh

| Servidor | Aspecto | testssl.sh | QuantumReady | |
|---|---|---|---|---|
| `ssl2` | protocolos | 1 | 1 | = |
| `ssl2` | cifrados SSLv2 | 4 | 4 | = |
| `ssl3` | protocolos | 1 | 1 | = |
| `ssl3` | suites SSLv3 (no vistas en TLS) | 24 | 24 | = |
| `ssl3` | DH (bits) | 1 | 1 | = |
| `ssl3` | grupos | 1 | 0 | **vemos menos**: [secp256r1] |
| `export` | protocolos | 3 | 3 | = |
| `export` | suites TLS 1.0–1.2 | 29 | 29 | = |
| `export` | DH (bits) | 1 | 1 | = |
| `dhe1024` | protocolos | 1 | 1 | = |
| `dhe1024` | suites TLS 1.0–1.2 | 10 | 10 | = |
| `dhe1024` | DH (bits) | 1 | 1 | = |
| `tls10` | protocolos | 1 | 1 | = |
| `tls10` | suites TLS 1.0–1.2 | 9 | 9 | = |
| `secp224` | protocolos | 1 | 1 | = |
| `secp224` | suites TLS 1.0–1.2 | 8 | 8 | = |
| `secp224` | grupos | 1 | 1 | = |
| `tls12` | protocolos | 1 | 1 | = |
| `tls12` | suites TLS 1.0–1.2 | 26 | 26 | = |
| `tls12` | DH (bits) | 1 | 1 | = |
| `tls12` | grupos | 2 | 2 | = |
| `tls13` | protocolos | 1 | 1 | = |
| `tls13` | suites TLS 1.3 | 5 | 5 | = |
| `tls13` | grupos | 8 | 8 | = |
| `modern` | protocolos | 2 | 2 | = |
| `modern` | suites TLS 1.0–1.2 | 3 | 3 | = |
| `modern` | suites TLS 1.3 | 3 | 3 | = |
| `modern` | grupos | 2 | 2 | = |
| `kyber` | protocolos | 1 | 1 | = |
| `kyber` | suites TLS 1.3 | 3 | 3 | = |
| `kyber` | grupos | 0 | 2 | **vemos más**: [X25519Kyber768Draft00 x25519] |
