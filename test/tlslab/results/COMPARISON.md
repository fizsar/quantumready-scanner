## Mediciones (intensidad full y basic)

| Servidor | full: conexiones | full: sondas en crudo | full: duración (local) | full: duración (RTT 100 ms) | basic: conexiones | basic: duración (local) | basic: duración (RTT 100 ms) |
|---|---|---|---|---|---|---|---|
| `ssl2` | 21 | 6 | 25 ms | 1612 ms | 15 | 5 ms | 1007 ms |
| `ssl3` | 45 | 30 | 147 ms | 6538 ms | 15 | 46 ms | 1029 ms |
| `export` | 53 | 38 | 54 ms | 8088 ms | 15 | 5 ms | 1009 ms |
| `dhe1024` | 31 | 16 | 73 ms | 3683 ms | 15 | 5 ms | 1008 ms |
| `tls10` | 30 | 11 | 41 ms | 3442 ms | 19 | 23 ms | 1827 ms |
| `secp224` | 31 | 16 | 42 ms | 3657 ms | 15 | 3 ms | 1007 ms |
| `tls12` | 49 | 33 | 29 ms | 7059 ms | 16 | 7 ms | 1011 ms |
| `tls13` | 33 | 18 | 16 ms | 3430 ms | 15 | 7 ms | 1010 ms |
| `modern` | 30 | 12 | 15 ms | 2425 ms | 18 | 9 ms | 1415 ms |
| `kyber` | 27 | 12 | 8 ms | 2216 ms | 15 | 5 ms | 1008 ms |
| `mldsa-dual` | 37 | 16 | 33 ms | 3646 ms | 21 | 31 ms | 2033 ms |
| `mldsa-only` | 29 | 14 | 30 ms | 2628 ms | 15 | 21 ms | 1016 ms |

## Comparación con testssl.sh

| Servidor | Aspecto | testssl.sh | QuantumReady | |
|---|---|---|---|---|
| `ssl2` | protocolos | 1 | 1 | = |
| `ssl2` | cifrados SSLv2 | 4 | 4 | = |
| `ssl2` | certificados (clave) | 1 | 0 | **vemos menos**: [RSA] |
| `ssl3` | protocolos | 1 | 1 | = |
| `ssl3` | suites SSLv3 (no vistas en TLS) | 24 | 24 | = |
| `ssl3` | DH (bits) | 1 | 1 | = |
| `ssl3` | grupos | 1 | 0 | **vemos menos**: [secp256r1] |
| `ssl3` | certificados (clave) | 1 | 0 | **vemos menos**: [RSA] |
| `export` | protocolos | 3 | 3 | = |
| `export` | suites TLS 1.0–1.2 | 29 | 29 | = |
| `export` | DH (bits) | 1 | 1 | = |
| `export` | certificados (clave) | 1 | 0 | **vemos menos**: [RSA] |
| `dhe1024` | protocolos | 1 | 1 | = |
| `dhe1024` | suites TLS 1.0–1.2 | 10 | 10 | = |
| `dhe1024` | DH (bits) | 1 | 1 | = |
| `dhe1024` | certificados (clave) | 1 | 0 | **vemos menos**: [RSA] |
| `tls10` | protocolos | 1 | 1 | = |
| `tls10` | suites TLS 1.0–1.2 | 9 | 9 | = |
| `tls10` | certificados (clave) | 1 | 1 | = |
| `secp224` | protocolos | 1 | 1 | = |
| `secp224` | suites TLS 1.0–1.2 | 8 | 8 | = |
| `secp224` | grupos | 1 | 1 | = |
| `secp224` | certificados (clave) | 1 | 0 | **vemos menos**: [RSA] |
| `tls12` | protocolos | 1 | 1 | = |
| `tls12` | suites TLS 1.0–1.2 | 26 | 26 | = |
| `tls12` | DH (bits) | 1 | 1 | = |
| `tls12` | grupos | 2 | 2 | = |
| `tls12` | certificados (clave) | 1 | 1 | = |
| `tls13` | protocolos | 1 | 1 | = |
| `tls13` | suites TLS 1.3 | 5 | 5 | = |
| `tls13` | grupos | 8 | 8 | = |
| `tls13` | certificados (clave) | 1 | 1 | = |
| `modern` | protocolos | 2 | 2 | = |
| `modern` | suites TLS 1.0–1.2 | 3 | 3 | = |
| `modern` | suites TLS 1.3 | 3 | 3 | = |
| `modern` | grupos | 2 | 2 | = |
| `modern` | certificados (clave) | 1 | 1 | = |
| `kyber` | protocolos | 1 | 1 | = |
| `kyber` | suites TLS 1.3 | 3 | 3 | = |
| `kyber` | grupos | 0 | 2 | **vemos más**: [X25519Kyber768Draft00 x25519] |
| `kyber` | certificados (clave) | 1 | 1 | = |
| `mldsa-dual` | protocolos | 2 | 2 | = |
| `mldsa-dual` | suites TLS 1.0–1.2 | 7 | 7 | = |
| `mldsa-dual` | suites TLS 1.3 | 3 | 3 | = |
| `mldsa-dual` | grupos | 8 | 8 | = |
| `mldsa-dual` | certificados (clave) | 2 | 2 | = |
| `mldsa-only` | protocolos | 1 | 1 | = |
| `mldsa-only` | suites TLS 1.3 | 3 | 3 | = |
| `mldsa-only` | grupos | 8 | 8 | = |
| `mldsa-only` | certificados (clave) | 1 | 1 | = |
