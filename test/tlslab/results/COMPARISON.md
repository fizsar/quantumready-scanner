## Mediciones (intensidad full y basic)

| Servidor | full: conexiones | full: sondas en crudo | full: duración (local) | full: duración (RTT 100 ms) | basic: conexiones | basic: duración (local) | basic: duración (RTT 100 ms) |
|---|---|---|---|---|---|---|---|
| `ssl2` | 21 | 6 | 16 ms | 1618 ms | 15 | 4 ms | 1013 ms |
| `ssl3` | 45 | 30 | 145 ms | 6640 ms | 15 | 40 ms | 1042 ms |
| `export` | 53 | 38 | 52 ms | 8137 ms | 15 | 4 ms | 1011 ms |
| `dhe1024` | 31 | 16 | 72 ms | 3734 ms | 15 | 5 ms | 1013 ms |
| `tls10` | 30 | 11 | 81 ms | 3461 ms | 19 | 28 ms | 1852 ms |
| `secp224` | 31 | 16 | 45 ms | 3709 ms | 15 | 5 ms | 1010 ms |
| `tls12` | 53 | 37 | 44 ms | 7922 ms | 16 | 9 ms | 1022 ms |
| `tls13` | 32 | 17 | 19 ms | 3448 ms | 15 | 9 ms | 1019 ms |
| `kyber` | 26 | 11 | 15 ms | 2226 ms | 15 | 5 ms | 1012 ms |

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
| `tls12` | suites TLS 1.0–1.2 | 30 | 30 | = |
| `tls12` | DH (bits) | 3 | 3 | = |
| `tls12` | grupos | 2 | 2 | = |
| `tls13` | protocolos | 1 | 1 | = |
| `tls13` | suites TLS 1.3 | 5 | 5 | = |
| `tls13` | grupos | 8 | 8 | = |
| `kyber` | protocolos | 1 | 1 | = |
| `kyber` | suites TLS 1.3 | 3 | 3 | = |
| `kyber` | grupos | 0 | 2 | **vemos más**: [X25519Kyber768Draft00 x25519] |
