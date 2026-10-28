## Mediciones (intensidad full y basic)

| Servidor | full: conexiones | full: sondas en crudo | full: duración (local) | full: duración (RTT 100 ms) | basic: conexiones | basic: duración (local) | basic: duración (RTT 100 ms) |
|---|---|---|---|---|---|---|---|
| `ssl2` | 21 | 6 | 47 ms | 1616 ms | 15 | 7 ms | 1007 ms |
| `ssl3` | 45 | 30 | 167 ms | 6581 ms | 15 | 56 ms | 1036 ms |
| `export` | 53 | 38 | 69 ms | 8100 ms | 15 | 5 ms | 1009 ms |
| `dhe1024` | 31 | 16 | 91 ms | 3716 ms | 15 | 5 ms | 1007 ms |
| `tls10` | 30 | 11 | 50 ms | 3453 ms | 19 | 48 ms | 1842 ms |
| `secp224` | 31 | 16 | 64 ms | 3675 ms | 15 | 6 ms | 1009 ms |
| `tls12` | 49 | 33 | 36 ms | 7071 ms | 16 | 8 ms | 1014 ms |
| `tls13` | 33 | 18 | 21 ms | 3434 ms | 15 | 10 ms | 1015 ms |
| `modern` | 30 | 12 | 25 ms | 2429 ms | 18 | 22 ms | 1422 ms |
| `kyber` | 27 | 12 | 11 ms | 2219 ms | 15 | 8 ms | 1011 ms |
| `mldsa-dual` | 37 | 16 | 53 ms | 3653 ms | 21 | 41 ms | 2044 ms |
| `mldsa-only` | 29 | 14 | 36 ms | 2631 ms | 15 | 43 ms | 1023 ms |

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
| `modern` | protocolos | 2 | 2 | = |
| `modern` | suites TLS 1.0–1.2 | 3 | 3 | = |
| `modern` | suites TLS 1.3 | 3 | 3 | = |
| `modern` | grupos | 2 | 2 | = |
| `modern` | certificados (clave) | 1 | 1 | = |
| `modern` | certificado (detalle) | 4 | 4 | = |
| `kyber` | protocolos | 1 | 1 | = |
| `kyber` | suites TLS 1.3 | 3 | 3 | = |
| `kyber` | grupos | 0 | 2 | **vemos más**: [X25519Kyber768Draft00 x25519] |
| `kyber` | certificados (clave) | 1 | 1 | = |
| `kyber` | certificado (detalle) | 4 | 4 | distinto: solo testssl [caduca 2027-10-14]; solo nosotros [caduca 2027-10-28] |
| `mldsa-dual` | protocolos | 2 | 2 | = |
| `mldsa-dual` | suites TLS 1.0–1.2 | 7 | 7 | = |
| `mldsa-dual` | suites TLS 1.3 | 3 | 3 | = |
| `mldsa-dual` | grupos | 8 | 8 | = |
| `mldsa-dual` | certificados (clave) | 2 | 2 | = |
| `mldsa-dual` | certificado (detalle) | 0 | 4 | **vemos más**: [cadena autofirmado caduca 2036-10-24 clave ML-DSA-65 firma ML-DSA-65] |
| `mldsa-only` | protocolos | 1 | 1 | = |
| `mldsa-only` | suites TLS 1.3 | 3 | 3 | = |
| `mldsa-only` | grupos | 8 | 8 | = |
| `mldsa-only` | certificados (clave) | 1 | 1 | = |
| `mldsa-only` | certificado (detalle) | 3 | 4 | **vemos más**: [firma ML-DSA-65] |
