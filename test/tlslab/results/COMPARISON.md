## Mediciones (intensidad full y basic)

| Servidor | full: conexiones | full: sondas en crudo | full: duración (local) | full: duración (RTT 100 ms) | basic: conexiones | basic: duración (local) | basic: duración (RTT 100 ms) |
|---|---|---|---|---|---|---|---|
| `ssl2` | 21 | 6 | 17 ms | 1614 ms | 15 | 4 ms | 1006 ms |
| `ssl3` | 45 | 30 | 120 ms | 6521 ms | 15 | 31 ms | 1027 ms |
| `export` | 53 | 38 | 50 ms | 8105 ms | 15 | 3 ms | 1008 ms |
| `dhe1024` | 31 | 16 | 61 ms | 3688 ms | 15 | 4 ms | 1009 ms |
| `tls10` | 30 | 11 | 75 ms | 3448 ms | 19 | 26 ms | 1837 ms |
| `secp224` | 31 | 16 | 43 ms | 3662 ms | 15 | 4 ms | 1010 ms |
| `tls12` | 49 | 33 | 29 ms | 7084 ms | 16 | 6 ms | 1017 ms |
| `tls13` | 32 | 17 | 17 ms | 3430 ms | 15 | 8 ms | 1010 ms |
| `kyber` | 26 | 11 | 8 ms | 2214 ms | 15 | 4 ms | 1007 ms |

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
| `kyber` | protocolos | 1 | 1 | = |
| `kyber` | suites TLS 1.3 | 3 | 3 | = |
| `kyber` | grupos | 0 | 2 | **vemos más**: [X25519Kyber768Draft00 x25519] |
