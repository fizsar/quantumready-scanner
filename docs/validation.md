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

---

## English

**Date**: 2026-10-13, 18:23–18:31 UTC. **Tools**: testssl.sh 3.2.4 (OpenSSL 3.5.8, image pinned by digest, `-4 --ip one -p -E -f -S -P`, no vulnerability checks) and our scanner (`cmd/qrscan`, commit `947ec08`), one run each per domain (cloudflare.com with Fiz's explicit OK).

**fizsar.com** and **quantumready.fizsar.com**: identical results for versions (TLS 1.2/1.3), TLS 1.2 suites (3), PQC (X25519MLKEM768, and we also show it is the group a browser gets), classical curves and certificate; only differences are crypto/tls limitations (full TLS 1.3 suite list, server cipher order). Both **95 · A**.

**cloudflare.com**: versions, PQC, curves match. TLS 1.2: testssl finds 20 suites, we find 17; the 3 missing (CBC-SHA384/SHA256 variants) are not implemented by Go (documented limitation, no grade impact since CBC already caps at B). Certificates: Cloudflare serves an RSA and an ECDSA certificate; we score the one a current client receives (difference of criterion). **Bug found and fixed**: 3DES is accepted only in TLS 1.0, and we enumerated suites in TLS 1.2 only, so the 3DES cap (C) was missed: the scan said 85 · B, the fixed model gives C. The scanner now enumerates TLS 1.0/1.1 too (tested with a server offering 3DES only to TLS 1.0 clients), scoring checks RC4/3DES/CBC across all versions and labels legacy-only suites with their version. Model stays 2026.1 (rules unchanged; no stored production scores). cloudflare.com was not re-scanned (single run agreed).

**Milestone 2.7 (raw ClientHello probes)**: later, not now. No grade difference came from what Go cannot see, and none of the three domains offers SSLv3 or DHE. Do it before opening the scanner to customer domains (phase 3), scoped to SSLv3, DHE and the full TLS 1.3 list, validated against a deliberately old test server rather than third parties.
