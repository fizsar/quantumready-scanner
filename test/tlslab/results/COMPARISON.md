## Mediciones (intensidad full y basic)

| Servidor | full: conexiones | full: sondas en crudo | full: duración (local) | full: duración (RTT 100 ms) | basic: conexiones | basic: duración (local) | basic: duración (RTT 100 ms) |
|---|---|---|---|---|---|---|---|
| `ssl2` | 22 | 7 | 30 ms | 1619 ms | 15 | 6 ms | 1012 ms |
| `ssl3` | 46 | 31 | 153 ms | 6673 ms | 15 | 33 ms | 1048 ms |
| `export` | 54 | 39 | 58 ms | 8148 ms | 15 | 5 ms | 1012 ms |
| `dhe1024` | 32 | 17 | 68 ms | 3762 ms | 15 | 6 ms | 1015 ms |
| `tls10` | 31 | 12 | 34 ms | 3473 ms | 19 | 27 ms | 1847 ms |
| `secp224` | 32 | 17 | 51 ms | 3700 ms | 15 | 5 ms | 1015 ms |
| `tls12` | 50 | 34 | 42 ms | 7114 ms | 16 | 9 ms | 1024 ms |
| `tls13` | 36 | 21 | 24 ms | 3459 ms | 15 | 11 ms | 1019 ms |
| `modern` | 33 | 15 | 26 ms | 2659 ms | 18 | 15 ms | 1434 ms |
| `kyber` | 30 | 15 | 15 ms | 2230 ms | 15 | 6 ms | 1013 ms |
| `mldsa-dual` | 40 | 19 | 49 ms | 3686 ms | 21 | 37 ms | 2065 ms |
| `mldsa-only` | 32 | 17 | 32 ms | 2645 ms | 15 | 30 ms | 1034 ms |

## Comparación con testssl.sh

| Servidor | Aspecto | testssl.sh | QuantumReady | |
|---|---|---|---|---|
| `ssl2` | protocolos | 1 | 1 | = |
| `ssl2` | cifrados SSLv2 | 4 | 4 | = |
| `ssl2` | certificados (clave) | 1 | 1 | = |
| `ssl2` | certificado (detalle) | 4 | 4 | = |
| `ssl3` | protocolos | 1 | 1 | = |
| `ssl3` | suites SSLv3 (no vistas en TLS) | 24 | 24 | = |
| `ssl3` | DH (bits) | 1 | 1 | = |
| `ssl3` | grupos | 1 | 0 | **vemos menos**: [secp256r1] |
| `ssl3` | certificados (clave) | 1 | 1 | = |
| `ssl3` | certificado (detalle) | 4 | 4 | = |
| `export` | protocolos | 3 | 3 | = |
| `export` | suites TLS 1.0–1.2 | 29 | 29 | = |
| `export` | DH (bits) | 1 | 1 | = |
| `export` | certificados (clave) | 1 | 1 | = |
| `export` | certificado (detalle) | 4 | 4 | = |
| `dhe1024` | protocolos | 1 | 1 | = |
| `dhe1024` | suites TLS 1.0–1.2 | 10 | 10 | = |
| `dhe1024` | DH (bits) | 1 | 1 | = |
| `dhe1024` | certificados (clave) | 1 | 1 | = |
| `dhe1024` | certificado (detalle) | 4 | 4 | = |
| `tls10` | protocolos | 1 | 1 | = |
| `tls10` | suites TLS 1.0–1.2 | 9 | 9 | = |
| `tls10` | certificados (clave) | 1 | 1 | = |
| `tls10` | certificado (detalle) | 4 | 4 | = |
| `secp224` | protocolos | 1 | 1 | = |
| `secp224` | suites TLS 1.0–1.2 | 8 | 8 | = |
| `secp224` | grupos | 1 | 1 | = |
| `secp224` | certificados (clave) | 1 | 1 | = |
| `secp224` | certificado (detalle) | 4 | 4 | = |
| `tls12` | protocolos | 1 | 1 | = |
| `tls12` | suites TLS 1.0–1.2 | 26 | 26 | = |
| `tls12` | DH (bits) | 1 | 1 | = |
| `tls12` | grupos | 2 | 2 | = |
| `tls12` | certificados (clave) | 1 | 1 | = |
| `tls12` | certificado (detalle) | 4 | 4 | = |
| `tls13` | protocolos | 1 | 1 | = |
| `tls13` | suites TLS 1.3 | 5 | 5 | = |
| `tls13` | grupos | 8 | 8 | = |
| `tls13` | certificados (clave) | 1 | 1 | = |
| `tls13` | certificado (detalle) | 4 | 4 | = |
| `tls13` | orden de suites TLS 1.3 | 1 | 1 | = |
| `modern` | protocolos | 2 | 2 | = |
| `modern` | suites TLS 1.0–1.2 | 3 | 3 | = |
| `modern` | suites TLS 1.3 | 3 | 3 | = |
| `modern` | grupos | 2 | 2 | = |
| `modern` | certificados (clave) | 1 | 1 | = |
| `modern` | certificado (detalle) | 4 | 4 | = |
| `modern` | orden de suites TLS 1.3 | 1 | 1 | = |
| `kyber` | protocolos | 1 | 1 | = |
| `kyber` | suites TLS 1.3 | 3 | 3 | = |
| `kyber` | grupos | 0 | 2 | **vemos más**: [X25519Kyber768Draft00 x25519] |
| `kyber` | certificados (clave) | 1 | 1 | = |
| `kyber` | certificado (detalle) | 4 | 4 | distinto: solo testssl [caduca 2027-10-14]; solo nosotros [caduca 2027-10-28] |
| `kyber` | orden de suites TLS 1.3 | 1 | 1 | = |
| `mldsa-dual` | protocolos | 2 | 2 | = |
| `mldsa-dual` | suites TLS 1.0–1.2 | 7 | 7 | = |
| `mldsa-dual` | suites TLS 1.3 | 3 | 3 | = |
| `mldsa-dual` | grupos | 8 | 8 | = |
| `mldsa-dual` | certificados (clave) | 2 | 2 | = |
| `mldsa-dual` | certificado (detalle) | 0 | 4 | **vemos más**: [cadena autofirmado caduca 2036-10-24 clave ML-DSA-65 firma ML-DSA-65] |
| `mldsa-dual` | orden de suites TLS 1.3 | 1 | 1 | = |
| `mldsa-only` | protocolos | 1 | 1 | = |
| `mldsa-only` | suites TLS 1.3 | 3 | 3 | = |
| `mldsa-only` | grupos | 8 | 8 | = |
| `mldsa-only` | certificados (clave) | 1 | 1 | = |
| `mldsa-only` | certificado (detalle) | 3 | 4 | **vemos más**: [firma ML-DSA-65] |
| `mldsa-only` | orden de suites TLS 1.3 | 1 | 1 | = |
