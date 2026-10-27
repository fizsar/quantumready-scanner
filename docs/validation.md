# Validación frente a testssl.sh / Validation against testssl.sh

**ES** · [EN](#english)

**Fecha**: 13/10/2026, 18:23–18:31 UTC. **Herramientas**: testssl.sh **3.2.4** (OpenSSL 3.5.8; imagen `ghcr.io/testssl/testssl.sh:3.2@sha256:51918b1e954df0acee8c08d3ad5ce306710e4fefe49ed0e0f92728ecf9e7ff34`, opciones `-4 --ip one -p -E -f -S -P`, sin pruebas de vulnerabilidades) y nuestro escáner (`cmd/qrscan`, commit `947ec08`). Una sola pasada de cada herramienta por dominio (cloudflare.com con OK expreso de Fiz). Misma IP en ambas herramientas.

Clasificación de cada diferencia: **L** = limitación de `crypto/tls` (documentada en docs/scanner.md), **C** = diferencia de criterio, **F** = fallo nuestro (corregido).

## fizsar.com (149.56.13.98)

| Aspecto | testssl.sh | QuantumReady | |
|---|---|---|---|
| Versiones | TLS 1.2, 1.3 (sin SSLv2/v3, TLS 1.0/1.1) | TLS 1.2, 1.3 | = |
| Suites TLS 1.2 | 3 (ECDHE-ECDSA AES128/256-GCM, ChaCha20) | las mismas 3 | = |
| Suites TLS 1.3 | 3 | solo la negociada (AES_128_GCM) | L (`tls13_suite_list`) |
| KEM / PQC | X25519MLKEM768 | X25519MLKEM768 soportado **y elegido** con oferta de navegador | = (+ nosotros distinguimos soportado/preferido) |
| Curvas clásicas | prime256v1, X25519 | x25519, secp256r1 | = |
| Certificado | ECDSA P-256, SHA384, cadena OK, nombre OK, YE1 (Let's Encrypt), caduca 06/01/2027, CT sí | lo mismo (2 SCT incrustados) | = |
| Orden del servidor | determinado por el cliente en TLS 1.3 | no evaluado | L (`tls12_server_cipher_preference`) |
| **Nota** | — (no puntúa PQC) | **95 · A** | |

## quantumready.fizsar.com (149.56.13.98)

Idéntico a fizsar.com (mismo Caddy) salvo el certificado: YE2, caduca 10/01/2027. Todo coincide; **95 · A**.

## cloudflare.com (104.16.133.229)

| Aspecto | testssl.sh | QuantumReady | |
|---|---|---|---|
| Versiones | TLS 1.0, 1.1, 1.2, 1.3 | TLS 1.0, 1.1, 1.2, 1.3 | = |
| Suites TLS 1.0 | 5, **incl. TLS_RSA_WITH_3DES_EDE_CBC_SHA** | **no enumeradas** | **F** (corregido) |
| Suites TLS 1.1 | 4 | no enumeradas | **F** (corregido) |
| Suites TLS 1.2 | 20 | 17 | faltan ECDHE-ECDSA/RSA-AES256-CBC-SHA384 y RSA-AES256-CBC-SHA256: Go no las implementa → L (`tls12_ccm_camellia_aria_null_export_suites`). No cambian la nota: ya hay CBC (tope B). |
| Suites TLS 1.3 | 3 | la negociada | L |
| KEM / PQC | X25519MLKEM768 | X25519MLKEM768 soportado y elegido | = |
| Curvas | prime256v1, secp384r1, secp521r1, X25519 | las mismas 4 | = |
| Certificado | **dos**: RSA-2048/SHA256 (WR1) y ECDSA P-256/SHA256 (WE1), Google Trust Services, caducan 04/12/2026 | el ECDSA (el que recibe un navegador) | C: puntuamos el certificado que obtiene un cliente actual; el RSA es para clientes antiguos |
| Orden del servidor (TLS 1.2) | sí | no evaluado | L |
| **Nota** | — | **85 · B** en el escaneo; **C** con la corrección (3DES aceptado → tope C) | |

### El fallo y su corrección

El escáner solo enumeraba las suites de TLS 1.2. Cloudflare acepta 3DES **solo en TLS 1.0**, así que no lo veíamos y la nota no aplicaba el tope C que el propio modelo exige para 3DES. `crypto/tls` sí puede negociar esas suites: no era una limitación, era un fallo.

- Escáner: enumera también TLS 1.0 y 1.1 (`tls10_cipher_suites`, `tls11_cipher_suites`, `legacy_cipher_suites_complete`). Test con un servidor que acepta 3DES solo cuando el cliente pide TLS 1.0.
- Scoring: RC4, 3DES y CBC se buscan en todas las versiones; las suites que solo existen en TLS 1.0/1.1 se muestran con su versión ("TLS_RSA_WITH_3DES_EDE_CBC_SHA (TLS 1.0)"). Caso fijo "3DES only in TLS 1.0 caps at C". El modelo sigue siendo 2026.1: las reglas no cambian (3DES aceptado → C); el escáner ahora ve más. No hay notas guardadas en producción.
- cloudflare.com **no se ha vuelto a escanear** (acordamos una sola vez). Con los datos de testssl, el resultado corregido sería 85 puntos con tope **C**.

## Conclusiones

- En lo que importa para la nota post-cuántica (versiones, grupos y PQC, certificado), las dos herramientas coinciden en los tres dominios.
- Las diferencias en suites son las limitaciones ya documentadas de Go, sin efecto en la nota de estos dominios, más el fallo de TLS 1.0/1.1 (corregido).
- Ninguno de los tres dominios acepta SSLv2/SSLv3 ni suites DHE, así que la comparación no permite medir cuánto importan esas limitaciones en servidores reales antiguos.

## Recomendación sobre el hito 2.7 (sondas de ClientHello en crudo)

**Más adelante, no ahora.** Hoy solo se escanean dominios de una lista cerrada, bien configurados, y la comparación no ha mostrado ninguna diferencia de nota atribuible a lo que Go no puede ver. Sí merecerá la pena **antes de abrir el escáner a dominios de clientes** (Fase 3, dominios verificados): en servidores corporativos antiguos son habituales SSLv3, DHE y suites CBC-SHA384, y entonces "no evaluado" dejaría de ser marginal. Propuesta: hito 2.7 al final de la Fase 2 o al inicio de la Fase 3, limitado a SSLv3, DHE y la lista completa de TLS 1.3, con una nueva comparación con testssl.sh contra un servidor de pruebas deliberadamente antiguo (no contra terceros).

## Hito 2.7.3: sondas en crudo frente a testssl.sh (laboratorio)

**Fecha**: 14/10/2026. **Dónde**: solo el laboratorio aislado `test/tlslab` (9 servidores deliberadamente vulnerables y, desde el 18/10/2026, `modern`, bien configurado y sin PQC; red Docker interna, sin puertos publicados; ver su README). **Nunca contra terceros.** **Herramientas**: testssl.sh 3.2.4 (misma imagen fijada que arriba, `-p -E -f -S -P`; referencia en `test/tlslab/reference/`) y el escáner con intensidad `full` (`make tlslab-test`). La comparación es automática: el test de integración lee el JSON de testssl.sh, compara protocolos, suites, tamaños de DH y grupos (curvas, KEMs y ffdhe) y falla si testssl.sh ve algo que nosotros no, salvo las diferencias explicadas abajo.

| Servidor | Protocolos | Suites | DH | Grupos | Resultado |
|---|---|---|---|---|---|
| `ssl2` (OpenSSL 1.0.2u) | SSLv2 | 4 cifrados SSLv2 | — | — | igual |
| `ssl3` | SSLv3 | 24 | 1024 | testssl.sh ve secp256r1 en SSLv3; nosotros no | **vemos menos** (abajo) |
| `export` | TLS 1.0, 1.1, 1.2 | 29 (EXPORT, DES, NULL, anónimas) | 512 | — | igual |
| `dhe1024` | TLS 1.2 | 10 | 1024 | — | igual |
| `tls10` | TLS 1.0 | 9 (RC4, 3DES, IDEA, SEED, Camellia) | — | — | igual |
| `secp224` | TLS 1.2 | 8 | — | secp224r1 | igual |
| `tls12` (OpenSSL 3.5.9) | TLS 1.2 | 26 (Camellia, ARIA, CCM, CCM_8; sin anónimas desde el 15/10/2026) | 2048 | brainpoolP256r1, secp256r1 | igual |
| `tls13` | TLS 1.3 | las 5, CCM_8 incluida | — | 8: X25519MLKEM768, MLKEM768, MLKEM1024, x25519, secp256r1, ffdhe2048, ffdhe3072, brainpoolP256r1tls13 | igual |
| `modern` (OpenSSL 3.5.9) | TLS 1.2, 1.3 | 3 ECDHE con AEAD; TLS 1.3: 3 | — | x25519, secp256r1 (ningún grupo PQC) | igual (18/10/2026) |
| `kyber` (Go 1.23.12) | TLS 1.3 | 3 | — | testssl.sh: ninguno; nosotros: **X25519Kyber768Draft00** y x25519 | **vemos más** |

Detalle por aspecto: `test/tlslab/results/COMPARISON.md` (se regenera con `make tlslab-test`).

### Donde vemos más que testssl.sh

- **El borrador de Kyber (`X25519Kyber768Draft00`, 0x6399)**. testssl.sh 3.2.4 informa "No KEMs offered" en el servidor Go 1.23, y ni siquiera lista x25519 (en servidores solo TLS 1.3 cambia a la OpenSSL del sistema y no sondea esos grupos). Nosotros lo encontramos con un ClientHello con `key_share` vacío: el servidor responde con un HelloRetryRequest que nombra el grupo. Es justo el caso "PQC experimental, no estándar" que la Fase 3 debe distinguir del ML-KEM estándar.
- Por diseño, también detectamos los grupos de TLS 1.3 sin generar claves (ffdhe, ML-KEM puro, brainpool), lo que permite sondear grupos que ninguna librería cliente implementa.

### Donde vemos menos (o distinto)

- **Curvas ECDHE que solo existen en SSLv3** (`ssl3`: secp256r1). Un cliente SSLv3 no envía `supported_groups`, así que no las sondeamos; el resultado lo marca como **no evaluado** (`legacy_groups`), no como "no soportado". Sin efecto en la nota: SSLv3 ya limita a C.
- **Suites por versión por debajo de TLS 1.2**: para ahorrar conexiones, en TLS 1.1, 1.0 y SSLv3 solo buscamos las suites que no aparecieron en una versión superior (sí detectamos qué versiones están activas). El conjunto total de suites coincide con el de testssl.sh en todos los servidores; lo que no listamos es la repetición de la misma suite en varias versiones.
- **Orden de preferencia del servidor**: sigue "no evaluado" (decisión del plan del 2.7).
- **Certificado de los servidores con los que `crypto/tls` no completa ningún handshake** (`ssl2`, `ssl3`, `export`, `dhe1024`, `secp224`): testssl.sh lo lee y nosotros no. El certificado sale de los handshakes de `crypto/tls`; las sondas en crudo lo ven en claro (TLS 1.0–1.2) pero no lo leen. Queda como **no evaluado** (`CERT_MISSING`, y desde el 2.8.5 `pq_certificate`). Leerlo de esas sondas es una mejora posible.
- **Vulnerabilidades de implementación** (Heartbleed, ROBOT…): fuera del alcance; tampoco se pidieron a testssl.sh.

### Hito 2.8.5: certificados post-cuánticos (ML-DSA)

Dos servidores más en el laboratorio, con OpenSSL 3.5.9 y certificados ML-DSA-65 (FIPS 204) generados al construir la imagen:

| Servidor | Versiones | Certificados | Ofreciendo ML-DSA (como Chrome) | Sin ML-DSA (como Firefox) | testssl.sh 3.2 |
|---|---|---|---|---|---|
| `mldsa-dual` | TLS 1.2, 1.3 | ML-DSA-65 y ECDSA P-256, elegidos según `signature_algorithms` | ML-DSA-65 (firma del handshake `mldsa65`) | ECDSA (firma `ecdsa_secp256r1_sha256`) | ve los 2 certificados (ECDSA y ML-DSA-65) y ML-DSA-65 entre las firmas de TLS 1.3: igual |
| `mldsa-only` | solo TLS 1.3 | solo ML-DSA-65 | ML-DSA-65 | el servidor rechaza la conexión (alerta 40, `handshake_failure`) | ve 1 certificado ML-DSA-65: igual |

Protocolos, suites y grupos también coinciden con testssl.sh en los dos. El certificado que ve un cliente sin ML-DSA lo lee una sonda en crudo de TLS 1.3 que descifra la respuesta del servidor (deriva las claves del handshake de su propio key share y se detiene tras el CertificateVerify, sin enviar nunca Finished): `crypto/tls` de Go 1.27 ofrece ML-DSA siempre y no deja quitarlo. Las mismas comprobaciones, con servidores de Go 1.27 (que también sirve certificados ML-DSA), están en los tests de `internal/scanner` y `internal/scanner/rawtls`, y con certificados SLH-DSA generados por OpenSSL en `internal/scanner/testdata/pqcerts`.

Nota con el modelo 2026.2 (`internal/scoring`, `TestLabPostQuantumCertificates`): tal como se escanean, 85 · F los dos (el certificado autofirmado limita a F, como en todo el laboratorio); con el certificado hecho válido, `mldsa-dual` 95 · B (las suites CBC de TLS 1.2 de OpenSSL) y `mldsa-only` 95 · A, exactamente la misma nota que con un certificado ECDSA: el certificado post-cuántico no suma (`CERT_PQ_SIGNATURE`, informativo). `mldsa-only` tiene además `CERT_PQ_NO_CLASSICAL_FALLBACK` (gravedad media, sin tope).

### Hito 2.8.6: notas con el modelo 2026.3

Las mismas sondas, el modelo nuevo: dos ejes (`docs/scoring.md`). Laboratorio con el certificado hecho válido, como en los tests (`internal/scoring`: `TestLabServers`, `TestLabPostQuantumCertificates`); fizsar.com, su escaneo real.

| Servidor | 2026.2 | 2026.3: seguridad hoy | 2026.3: preparación cuántica |
|---|---|---|---|
| `fizsar.com` | A · 95 | A · 100 | 75 · Preparado |
| `tls13` | A · 100 | A · 100 | 85 · Preparado |
| `modern` | D · 45 | A · 100 | 25 · No preparado |
| `kyber` | D · 45 | A · 100 | 10 · No preparado |
| `tls12` | D · 40 | D · 45 (tope B) | 15 · No preparado |
| `dhe1024` | D · 40 | C · 70 (tope C) | 15 · No preparado |
| `secp224` | D · 40 | C · 70 (tope C) | 15 · No preparado |
| `tls10` | D · 40 | D · 45 (tope C) | 15 · No preparado |
| `ssl2` | F · 20 | F · 45 (tope F) | 15 · No preparado |
| `ssl3` | F · 20 | F · 45 (tope F) | 15 · No preparado |
| `export` | F · 20 | F · 45 (tope F) | 15 · No preparado |
| `mldsa-dual` | B · 95 | B · 100 (tope B) | 90 · Preparado |
| `mldsa-only` | A · 95 | A · 100 | 90 · Preparado |

`kyber` queda por debajo de `modern` en preparación (10 frente a 25): aceptar el borrador obsoleto de Kyber resta 15. `mldsa-only` no pierde puntos por no tener certificado clásico de respaldo: es un hallazgo medio visible (`CERT_PQ_NO_CLASSICAL_FALLBACK`). En /examples, con el certificado simulado sin renovación automatizada: fizsar.com A · 100 / 75 Preparado, `modern` A · 90 / 10, `kyber` A · 90 / 0 y `export` F · 35 / 0 (`TestLabFixtures`).

### Mediciones y tiempo máximo por escaneo

Cada servidor, con intensidad `full` y `basic`, en local y con una latencia simulada de 100 ms de RTT (el dialer espera 2 RTT por conexión: TCP y el primer intercambio; solo en tests).

| Servidor | full: conexiones | full: sondas en crudo | full: local | full: RTT 100 ms | basic: conexiones | basic: local | basic: RTT 100 ms |
|---|---|---|---|---|---|---|---|
| `ssl2` | 21 | 6 | 18 ms | 1,6 s | 15 | 4 ms | 1,0 s |
| `ssl3` | 45 | 30 | 110 ms | 6,6 s | 15 | 32 ms | 1,0 s |
| `export` | 53 | 38 | 45 ms | 8,1 s | 15 | 4 ms | 1,0 s |
| `dhe1024` | 31 | 16 | 62 ms | 3,7 s | 15 | 4 ms | 1,0 s |
| `tls10` | 30 | 11 | 47 ms | 3,4 s | 19 | 26 ms | 1,8 s |
| `secp224` | 31 | 16 | 44 ms | 3,7 s | 15 | 4 ms | 1,0 s |
| `tls12` | 49 | 33 | 29 ms | 7,1 s | 16 | 6 ms | 1,0 s |
| `tls13` | 32 | 17 | 17 ms | 3,4 s | 15 | 8 ms | 1,0 s |
| `modern` | 29 | 11 | 28 ms | 2,4 s | 18 | 16 ms | 1,4 s |
| `kyber` | 26 | 11 | 9 ms | 2,2 s | 15 | 5 ms | 1,0 s |

**Decisión: se mantienen 60 s por escaneo.** El peor caso del laboratorio (`export`, 38 sondas en crudo, 53 conexiones) tarda 8,1 s con 100 ms de RTT; la duración crece de forma lineal con el RTT (la cadena más larga es secuencial: cada suite encontrada exige una conexión nueva), así que incluso con 300 ms de RTT (otro continente) serían unos 25 s, y el presupuesto de 48 sondas en crudo acota el peor caso de cualquier servidor. 90 s solo protegerían frente a servidores que tardan segundos en cada handshake, y para eso ya está el límite de 5 s por conexión: lo que no termine se marca como error y "no evaluado", nunca como "no soportado". Ningún servidor agotó el presupuesto. Un servidor moderno real (Go/Caddy) usa 11 sondas en crudo: medido el 15/10/2026 con `qrscan` desde WSL contra fizsar.com y quantumready.fizsar.com (29 conexiones, 3,2–3,3 s, ambos 95 · A con el modelo 2026.2).

**Intensidad `basic` idéntica a la anterior**: `internal/scanner/testdata/basic_golden.json` se generó con el escáner anterior al 2.7.3 (commit `5631b85`) y un test comprueba que `basic` da exactamente el mismo JSON, salvo los campos nuevos opcionales (`intensity`, `connections`).

---

## English

**Date**: 2026-10-13, 18:23–18:31 UTC. **Tools**: testssl.sh 3.2.4 (OpenSSL 3.5.8, image pinned by digest, `-4 --ip one -p -E -f -S -P`, no vulnerability checks) and our scanner (`cmd/qrscan`, commit `947ec08`), one run each per domain (cloudflare.com with Fiz's explicit OK).

**fizsar.com** and **quantumready.fizsar.com**: identical results for versions (TLS 1.2/1.3), TLS 1.2 suites (3), PQC (X25519MLKEM768, and we also show it is the group a browser gets), classical curves and certificate; only differences are crypto/tls limitations (full TLS 1.3 suite list, server cipher order). Both **95 · A**.

**cloudflare.com**: versions, PQC, curves match. TLS 1.2: testssl finds 20 suites, we find 17; the 3 missing (CBC-SHA384/SHA256 variants) are not implemented by Go (documented limitation, no grade impact since CBC already caps at B). Certificates: Cloudflare serves an RSA and an ECDSA certificate; we score the one a current client receives (difference of criterion). **Bug found and fixed**: 3DES is accepted only in TLS 1.0, and we enumerated suites in TLS 1.2 only, so the 3DES cap (C) was missed: the scan said 85 · B, the fixed model gives C. The scanner now enumerates TLS 1.0/1.1 too (tested with a server offering 3DES only to TLS 1.0 clients), scoring checks RC4/3DES/CBC across all versions and labels legacy-only suites with their version. Model stays 2026.1 (rules unchanged; no stored production scores). cloudflare.com was not re-scanned (single run agreed).

**Milestone 2.7 (raw ClientHello probes)**: later, not now. No grade difference came from what Go cannot see, and none of the three domains offers SSLv3 or DHE. Do it before opening the scanner to customer domains (phase 3), scoped to SSLv3, DHE and the full TLS 1.3 list, validated against a deliberately old test server rather than third parties.

**Milestone 2.7.3 (raw ClientHello probes vs testssl.sh, lab only)**: nine deliberately weak servers (plus, since 2026-10-18, `modern`: TLS 1.2 and 1.3, ECDHE with AEAD only, x25519 and secp256r1, no PQC; identical to testssl.sh) in the isolated `test/tlslab` (internal Docker network, no published ports; never third parties). `make tlslab-test` scans each one with intensity full and compares protocols, suites, DH sizes and groups with the testssl.sh 3.2.4 reference automatically, failing if testssl.sh sees anything we do not (except the explained cases). Everything matches, including SSLv2 (4 ciphers), SSLv3 (24 suites, DH 1024), EXPORT/DES/NULL/anonymous suites and DH 512, DHE 1024 and 2048, Camellia/ARIA/CCM/CCM_8, secp224r1, brainpool, the five TLS 1.3 suites and ML-KEM (pure and hybrid), ffdhe and brainpool TLS 1.3 groups. **We see more**: the pre-standard Kyber draft (and x25519) on the Go 1.23 server, which testssl.sh reports as "No KEMs offered"; we find it with an empty key_share and the server's HelloRetryRequest. **We see less**: ECDHE curves that exist only in SSLv3 (an SSLv3 client sends no supported_groups), reported as "not evaluated" with no grade effect (SSLv3 already caps at C); below TLS 1.2 we only list suites not seen in a higher version (the total set matches; versions are detected). Server cipher order and implementation vulnerabilities stay out of scope. **Scan timeout stays at 60 s**: the worst lab case takes 8.1 s at a simulated 100 ms RTT (53 connections, 38 raw probes), growing linearly with RTT (about 25 s at 300 ms) and bounded by the 48-probe budget; slow handshakes are already capped at 5 s each. **Intensity basic is unchanged**: a golden file written by the pre-2.7.3 scanner (commit `5631b85`) pins its JSON, except for the new optional fields.
