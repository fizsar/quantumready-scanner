# Motor de escaneo / Scanning engine

**ES** · [EN](#english)

`internal/scanner` es una librería pura (sin HTTP ni base de datos) que analiza un endpoint TLS y devuelve **hechos**. Juzgarlos (nota, hallazgos, remediación) es trabajo de `internal/scoring`.

## Qué hace un escaneo

1. **Valida el destino**: nombre DNS en ASCII (punycode) o IP literal; puerto 1–65535 (443 por defecto).
2. **Resuelve una sola vez** y aplica la política anti-SSRF (abajo) a **todas** las direcciones. Escanea la primera IPv4 (o la primera IPv6 si no hay IPv4).
3. **Conecta siempre a esa IP ya validada** (nunca al nombre), con el nombre en el SNI. Un segundo DNS que apunte a otra parte (*DNS rebinding*) no tiene efecto. El dialer vuelve a comprobar la política sobre el socket justo antes de conectar.
4. Lanza entre 15 y 40 handshakes completos (según cuántas suites acepte el servidor) con `crypto/tls`:

| Sonda | Cómo | Para qué |
|---|---|---|
| Oferta tipo navegador | TLS 1.2–1.3, grupos `X25519MLKEM768, x25519, secp256r1, secp384r1` (orden de Chrome/Firefox) | Qué grupo **elige** el servidor de verdad |
| Cada grupo híbrido por separado | Solo TLS 1.3 y un único grupo: `X25519MLKEM768`, `SecP256r1MLKEM768`, `SecP384r1MLKEM1024` | Si lo **soporta**, aunque no lo prefiera |
| Cada grupo clásico por separado | `x25519`, `secp256r1`, `secp384r1`, `secp521r1` (solo suites ECDHE, para que el grupo se use de verdad) | Inventario |
| Cada versión por separado | TLS 1.0, 1.1, 1.2 y 1.3, con todas las suites y grupos de Go | Versiones activas |
| Suites de TLS 1.2 | Ofrecer todas, quitar la elegida y repetir hasta que el servidor rechace | Suites aceptadas (forward secrecy, AEAD, inseguras) |
| Certificado | Se captura la cadena en cualquier handshake (incluso si falla después) y se verifica aparte | Clave, firma, cadena, caducidad, nombre, emisor |

Cada sonda termina en `supported`, `not_supported` (alerta del servidor, conexión cerrada o parámetros que no ofrecimos) o `error` (timeout o fallo de red). **Un `error` nunca se interpreta como "no soportado".**

Límites: 5 s por handshake, 60 s por escaneo, 4 handshakes en paralelo por destino y 16 en total por proceso. Todo se cancela con `context`. Las sondas no reutilizan sesiones (sin tickets): cada una es un handshake completo.

## Política anti-SSRF

Solo se conecta a direcciones **unicast públicas**. Se rechaza el host entero si **alguna** de sus direcciones no lo es (un sitio público no tiene motivo para resolver a una dirección interna). Bloqueado, entre otros:

- IPv4: `0.0.0.0/8`, `10.0.0.0/8`, `100.64.0.0/10` (CGNAT), `127.0.0.0/8`, `169.254.0.0/16` (incluye metadatos de la nube), **`172.16.0.0/12` (incluye las redes de Docker: `172.17.0.0/16`, `172.18.0.0/16`…)**, `192.168.0.0/16`, rangos de documentación y benchmarking, multicast, reservados y broadcast.
- IPv6: todo lo que no sea `2000::/3` (loopback, link-local, ULA, multicast…) y, dentro de él, NAT64, 6to4, Teredo y documentación, porque pueden encapsular una IPv4 interna. Las IPv4 mapeadas (`::ffff:10.0.0.1`) se evalúan como IPv4.

Los tests (`netguard_test.go`) cubren cada rango, incluidas las IP de las redes Docker del VPS (`172.17.0.1`, `172.18.0.2`).

### Dominio que resuelve a la IP pública del propio VPS

`fizsar.com` y `quantumready.fizsar.com` resuelven a `149.56.13.98`, que es pública: **se permite**, y es lo que queremos (escanear nuestro propio sitio público). Desde el worker, esa conexión llega al host por su IP pública, así que solo alcanza lo que el host expone en esa IP y deja pasar el cortafuegos (ufw: 22, 80 y 443), es decir, **lo mismo que ve cualquiera desde Internet**. Lo que está ligado a `127.0.0.1` (la API en 8080 y la web en 8081) y las redes internas de Docker no son alcanzables por esa vía. Además, la API (hito 2.4) solo admite los dominios de `SCAN_ALLOWLIST` y limitará los puertos.

## Lo que `crypto/tls` no puede evaluar

Se informa en `Result.NotEvaluated` y la interfaz lo muestra como **"no evaluado"**, nunca como "no soportado":

| Código | Qué falta | ¿Merece la pena cubrirlo? |
|---|---|---|
| `ssl2_ssl3` | SSLv2 y SSLv3 (Go no los implementa) | Sí, para el riesgo actual: sonda de ClientHello en crudo (solo leer el ServerHello) |
| `tls12_dhe_suites` | Suites DHE (Diffie-Hellman de campo finito), habituales en servidores antiguos | Sí: misma sonda en crudo |
| `tls12_ccm_camellia_aria_null_export_suites` | CCM, Camellia, ARIA, NULL, EXPORT, anónimas, ECDHE-CBC-SHA384, RSA-AES256-SHA256 | Parcial: las peligrosas (NULL, EXPORT, anónimas) sí |
| `tls13_suite_list` | Lista completa de suites TLS 1.3 (Go no deja elegirlas; solo vemos la negociada) | Bajo interés: las cinco de TLS 1.3 son AEAD |
| `tls12_server_cipher_preference` | Orden de preferencia del servidor (Go reordena las suites que ofrece) | Bajo interés |
| `legacy_groups` | Grupos ffdhe, brainpool, secp224r1… | Bajo interés |
| `non_standard_pqc_groups` | Variantes PQC no estándar (Kyber draft `0x6399`, ML-KEM "puro") | Informativo |
| `implementation_vulnerabilities` | Heartbleed, ROBOT, renegociación, compresión… | Fuera de alcance: es otra clase de escáner |

Otras limitaciones: no se reproduce byte a byte el ClientHello de un navegador (GREASE, orden de extensiones), solo su preferencia de grupos; algún servidor o middlebox podría comportarse distinto. Claves RSA de más de 8192 bits: `crypto/tls` aborta el handshake (el certificado sí se captura).

**Cómo cubrirlo**: sondas de *ClientHello en crudo* con `golang.org/x/crypto/cryptobyte`: se construye el ClientHello a mano, se envía y se lee **solo** el ServerHello (o la alerta), sin completar el handshake. Es la técnica de testssl.sh para comprobar soporte. Unas 300–500 líneas con tests. Se decidirá en el hito 2.7 a partir de la comparación con testssl.sh (2.6). Se descarta empaquetar OpenSSL o testssl.sh en el worker (shell en la imagen y mucha más superficie de ataque).

## Ajustes de GODEBUG

`cmd/qrscan` y `cmd/worker` declaran `//go:debug tlssha1=1` y `//go:debug rsa1024min=0` para que `crypto/tls` complete handshakes con servidores muy antiguos (firmas SHA-1 en TLS 1.2, RSA de menos de 1024 bits) y se puedan analizar. El escáner solo observa esas conexiones y nunca confía en ellas. Los tests de la librería se ejecutan sin esos ajustes.

---

## English

`internal/scanner` is a pure library (no HTTP, no database) that probes a TLS endpoint and returns **facts**. Judging them (grade, findings, remediation) is `internal/scoring`'s job.

**A scan** validates the target, resolves it **once**, applies the anti-SSRF policy to **every** address and always connects to the validated IP (never the name; SNI carries the name), so DNS rebinding has no effect; the dialer re-checks the policy on the socket. It then runs 15 to 40 full `crypto/tls` handshakes: a browser-like offer (which group the server **picks**), each hybrid ML-KEM group alone (whether it is **supported**), each classical group, each protocol version, an enumeration of the TLS 1.2 suites, and certificate capture (even from failed handshakes) with independent verification. Each probe ends as `supported`, `not_supported` or `error`; an `error` (timeout, network) is never read as "not supported". Limits: 5 s per handshake, 60 s per scan, 4 parallel handshakes per target, 16 per process; no session resumption.

**Anti-SSRF**: only public unicast addresses; the whole host is refused if any address is not. Blocked ranges include `10/8`, `100.64/10`, `127/8`, `169.254/16` (cloud metadata), **`172.16/12` (Docker networks)**, `192.168/16`, documentation, multicast, reserved; IPv6 outside `2000::/3` plus NAT64, 6to4, Teredo and documentation; IPv4-mapped addresses are checked as IPv4. A domain resolving to the VPS's own public IP (`fizsar.com` → `149.56.13.98`) is allowed: it only reaches what the host exposes publicly through the firewall (22, 80, 443), the same as anyone on the Internet; loopback-bound services (API 8080, web 8081) and Docker networks are not reachable that way.

**Not evaluated by crypto/tls** (reported in `Result.NotEvaluated`, shown as "not evaluated", never "not supported"): SSLv2/v3, DHE suites, CCM/Camellia/ARIA/NULL/EXPORT/anonymous suites, the full TLS 1.3 suite list, server cipher preference, legacy groups, non-standard PQC groups, implementation vulnerabilities (Heartbleed, ROBOT…). The browser ClientHello is not reproduced byte for byte. Coverage option: raw ClientHello probes (build the ClientHello, read only the ServerHello), as testssl.sh does; to be decided in milestone 2.7 after the testssl.sh comparison. Bundling OpenSSL/testssl.sh in the worker is ruled out.

**GODEBUG**: `cmd/qrscan` and `cmd/worker` set `tlssha1=1` and `rsa1024min=0` so very old servers can be analyzed; the scanner only observes those connections and never trusts them.
