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
| Suites de TLS 1.0, 1.1 y 1.2 | En cada versión, ofrecer todas, quitar la elegida y repetir hasta que el servidor rechace | Suites aceptadas (forward secrecy, AEAD, inseguras). Las débiles a veces solo sobreviven en TLS 1.0/1.1 (ver docs/validation.md) |
| Certificado | Se captura la cadena en cualquier handshake (incluso si falla después) y se verifica aparte | Clave, firma, cadena, caducidad, nombre, emisor |

Cada sonda termina en `supported`, `not_supported` (alerta del servidor, conexión cerrada o parámetros que no ofrecimos) o `error` (timeout o fallo de red). **Un `error` nunca se interpreta como "no soportado".**

Límites: 5 s por handshake, 60 s por escaneo, 4 handshakes en paralelo por destino y 16 en total por proceso. Todo se cancela con `context`. Las sondas no reutilizan sesiones (sin tickets): cada una es un handshake completo.

### Intensidad: `basic` y `full` (hito 2.7.3)

- **`basic`**: exactamente lo anterior, solo `crypto/tls` (un test con un fichero de referencia generado por el escáner anterior lo garantiza).
- **`full`**: además, **sondas de ClientHello en crudo** (`internal/scanner/rawtls`), después de las de `crypto/tls` y usando sus resultados. Mismas protecciones: la IP ya validada, el dialer con la política anti-SSRF, el mismo semáforo (4 por destino, 16 por proceso), 5 s por sonda y, además, **un máximo de 48 sondas en crudo por escaneo** (`RawProbeBudget`). Ninguna completa un handshake: se envía el ClientHello, se lee la primera respuesta y se cierra.

| Sonda en crudo | Cómo | Conexiones |
|---|---|---|
| SSLv2 | CLIENT-HELLO SSLv2 con sus 7 cifrados | 1 |
| SSLv3 | ClientHello 3.0 sin extensiones con todas las suites; si responde, se enumeran las suites que solo acepta en SSLv3 | 1 (+ suites, solo si habla SSLv3) |
| Suites de TLS 1.2, 1.1 y 1.0 que `crypto/tls` no encontró | Todas las suites de IANA negociables sin credenciales (DHE, CCM, Camellia, ARIA, SEED, IDEA, NULL, EXPORT, anónimas…): ofrecer, quitar la elegida, repetir. En 1.1 y 1.0, solo las no vistas en una versión superior. Si `crypto/tls` no vio una versión, la primera sonda ofrece todo para saber si está activa | 1 por versión + suites nuevas |
| Tamaño del grupo DH | Del ServerKeyExchange de cada suite DHE aceptada (sin responder nunca) | 0 |
| Lista completa de TLS 1.3 | Todas las suites de TLS 1.3 (también AEGIS, Ascon, SM4 e integrity-only); un HelloRetryRequest también cuenta | 1 + suites |
| Grupos de TLS 1.3 que `crypto/tls` no sondea | ffdhe, brainpool, x448, ML-KEM puro, híbridos en borrador, Kyber pre-estándar, GOST, SM2: se ofrecen con `key_share` **vacío** y el servidor nombra el que soporta en un HelloRetryRequest | 1 + grupos |
| Curvas de TLS 1.2 que `crypto/tls` no sondea | secp224r1, secp192r1, brainpool, binarias…: suites ECDHE con solo esas curvas; la elegida se lee en el ServerKeyExchange | 1 + curvas (solo si hay ECDHE) |

Un servidor moderno usa unas 10–12 sondas en crudo; el peor caso del laboratorio, 38 (`docs/validation.md`). Si se agota el presupuesto, lo que falte queda como **no evaluado**. El resultado está en `Result.Raw` (y `Result.Intensity`, `Result.Connections`); todos son campos opcionales, así que los resultados guardados antes conservan su significado. El worker toma la intensidad de `SCAN_INTENSITY` (por defecto `full`); en la Fase 3, `full` será solo para dominios verificados. `cmd/qrscan -intensity basic|full`.

**Cómo leer `raw`** (desde el S6d): `suites`, `tls13_groups` y `tls12_curves` listan **solo lo que `crypto/tls` no encontró o no sondea**; lo que sí sondea está en `key_exchange.groups` y en las listas `tls1x_cipher_suites`. Por eso un servidor moderno tiene esas listas vacías: no le falta nada, es que no acepta nada más. El campo `lists_exclude_crypto_tls: true` lo dice en el propio JSON, y las listas son siempre `[]`, nunca `null`. `*_complete: true` significa que la enumeración terminó (el servidor rechazó lo que quedaba), sea cual sea lo encontrado; `false`, que se cortó por un error o por el presupuesto. `raw.versions` va de la versión más antigua a la más nueva. Los resultados guardados antes pueden tener `null` en esas listas, sin el campo y con las versiones en orden inverso: significan lo mismo, puntúan igual (lo comprueba un test) y `RawResult.Normalize` los lleva a la forma actual. No cambia `schema_version`: el campo es opcional y nuevo, y `null` y `[]` se leen igual.

**Laboratorio** (`test/tlslab`): `cmd/qrscan -lab` cambia la política anti-SSRF por una que acepta **solo** loopback y RFC 1918 (nunca una IP pública); existe solo en `cmd/qrscan`, y un test compila los binarios de producción y comprueba que ese código no está en ellos.

### Esquema 2: registro por sonda y códigos de error (hito 2.8.4)

`schema_version` 2 añade campos, todos opcionales: un resultado de la versión 1 se lee igual (sin ellos) y la puntuación no lee ninguno (tests `TestSchema2SameJudgement` y `TestSchema2FieldsAreNotScored` de internal/scoring, con los 10 servidores del laboratorio y fizsar.com).

- **`probe_log`**: cada conexión del escaneo, en orden de inicio (`start_ms`, `duration_ms`). Cada conexión pasa por una copia de lo escrito y lo leído (hasta 64 KiB por sentido), tanto en las de `crypto/tls` como en las sondas en crudo:
  - **Lo ofrecido** (`offers`, a las que cada sonda apunta con `offer`) se lee de los **bytes del ClientHello enviado** (`rawtls.ParseClientHello`), no de la configuración: versiones, suites, grupos, key shares, `signature_algorithms`, `signature_algorithms_cert`, ALPN y las extensiones en orden, con GREASE y SCSV tal como salieron. Las ofertas iguales se guardan una vez, y las de una enumeración como diferencia con la anterior (`like` + `suites_removed`/`groups_removed`; `ProbeLog.Offer(i)` y `expandOffer` en la web las desarrollan).
  - **La respuesta** (`response`) se lee de los **bytes del servidor hasta donde empieza el cifrado** (`rawtls.ParseServerFlight`): `type` (`server_hello`, `hello_retry_request`, `alert`, `closed`, `timeout`, `not_tls`, `sslv2_server_hello`, `sslv2_error`, `no_connection`, `not_sent`), versión, suite y grupo elegidos (en TLS 1.3 el del key share; en TLS 1.0–1.2 la curva del ServerKeyExchange, o los bits del DH), el grupo que pide un HelloRetryRequest y la alerta con nivel, código y nombre IANA.
  - `purpose` y `subject` dicen para qué era la sonda (`default`, `group`, `version`, `suites`, `sslv2`, `sslv3`, `tls13_suites`, `tls13_groups`, `tls12_curves`); `status` es su veredicto, igual que en el resto del resultado.
- **`error_code`** junto a cada `error` (y `trust_error_code` junto a `trust_error`): un código estable (ADR-0001), lista completa en `internal/scanner/errcode.go`: `alert:<n>`, `timeout`, `closed`, `reset`, `connection_refused`, `unoffered_<qué>`, `other_group`, `other_version`, `malformed`, `not_tls`, `budget`… y para el certificado `unknown_authority`, `expired` (también «aún no válido»: Go les da el mismo motivo), `name_constraints`, `incompatible_usage`… El texto de la biblioteca sigue al lado, solo como evidencia.
- **OID de cada certificado de la cadena** (`signature_oid`, `public_key_oid`), leídos del certificado tal como llegó, también los que `crypto/x509` no conoce.

**Lo que los bytes no pueden mostrar**:
- Una **alerta que llega cifrada** (en TLS 1.3, después del ServerHello; en TLS 1.0–1.2, después del ChangeCipherSpec): su código sale del error de `crypto/tls`, que lleva el valor exacto de la alerta, pero **no su nivel** (`encrypted: true`, sin `level`). En las sondas en crudo no ocurre: leen solo hasta el ServerHello o el ServerKeyExchange.
- Tras un **HelloRetryRequest**, la oferta registrada es el **primer** ClientHello; el segundo (con el key share pedido) no se guarda aparte.
- Sin conexión TCP no hay oferta (`no_connection`); si el presupuesto de sondas en crudo impide una, queda `not_sent` con `budget`.

**Tamaño** (JSON compacto, medido el 27/10/2026): fizsar.com pasa de 4,0 a 21,2 KB; el laboratorio, de 2–7 KB a 14–30 KB (export 29,7 KB, tls12 28,8 KB); comprimido con gzip, 2–3,5 KB.

## Política anti-SSRF

Solo se conecta a direcciones **unicast públicas**. Se rechaza el host entero si **alguna** de sus direcciones no lo es (un sitio público no tiene motivo para resolver a una dirección interna). Bloqueado, entre otros:

- IPv4: `0.0.0.0/8`, `10.0.0.0/8`, `100.64.0.0/10` (CGNAT), `127.0.0.0/8`, `169.254.0.0/16` (incluye metadatos de la nube), **`172.16.0.0/12` (incluye las redes de Docker: `172.17.0.0/16`, `172.18.0.0/16`…)**, `192.168.0.0/16`, rangos de documentación y benchmarking, multicast, reservados y broadcast.
- IPv6: todo lo que no sea `2000::/3` (loopback, link-local, ULA, multicast…) y, dentro de él, NAT64, 6to4, Teredo y documentación, porque pueden encapsular una IPv4 interna. Las IPv4 mapeadas (`::ffff:10.0.0.1`) se evalúan como IPv4.

Los tests (`netguard_test.go`) cubren cada rango, incluidas las IP de las redes Docker del VPS (`172.17.0.1`, `172.18.0.2`).

### Dominio que resuelve a la IP pública del propio VPS

`fizsar.com` y `quantumready.fizsar.com` resuelven a `149.56.13.98`, que es pública: **se permite**, y es lo que queremos (escanear nuestro propio sitio público). Desde el worker, esa conexión llega al host por su IP pública, así que solo alcanza lo que el host expone en esa IP y deja pasar el cortafuegos (ufw: 22, 80 y 443), es decir, **lo mismo que ve cualquiera desde Internet**. Lo que está ligado a `127.0.0.1` (la API en 8080 y la web en 8081) y las redes internas de Docker no son alcanzables por esa vía. Además, la API (hito 2.4) solo admite los dominios de `SCAN_ALLOWLIST` y limitará los puertos.

## Lo que no se evalúa

Se informa en `Result.NotEvaluated` y la interfaz lo muestra como **"no evaluado"**, nunca como "no soportado". Con intensidad `basic` es la lista completa de abajo; con `full`, las sondas en crudo cubren las seis primeras, que **solo vuelven a aparecer si su sonda no terminó** (error o presupuesto agotado):

| Código | Qué falta | Con intensidad `full` |
|---|---|---|
| `ssl2_ssl3` | SSLv2 y SSLv3 (Go no los implementa) | Evaluado (sondas SSLv2 y SSLv3) |
| `tls12_dhe_suites` | Suites DHE (Diffie-Hellman de campo finito), habituales en servidores antiguos | Evaluado, con el tamaño del grupo DH |
| `tls12_ccm_camellia_aria_null_export_suites` | CCM, Camellia, ARIA, NULL, EXPORT, anónimas, ECDHE-CBC-SHA384, RSA-AES256-SHA256 | Evaluado |
| `tls13_suite_list` | Lista completa de suites TLS 1.3 (Go no deja elegirlas; solo vemos la negociada) | Evaluado |
| `legacy_groups` | Grupos ffdhe, brainpool, secp224r1… | Evaluado (salvo curvas que solo existen en SSLv3) |
| `non_standard_pqc_groups` | Variantes PQC no estándar (Kyber draft `0x6399`, ML-KEM "puro") | Evaluado |
| `tls12_server_cipher_preference` | Orden de preferencia del servidor (Go reordena las suites que ofrece) | Sigue sin evaluar (bajo interés) |
| `credential_suites` | Suites PSK, SRP, Kerberos y GOST: necesitan credenciales o algoritmos que un escáner no tiene | Solo con `full`: no se pueden evaluar |
| `implementation_vulnerabilities` | Heartbleed, ROBOT, renegociación, compresión… | Fuera de alcance: es otra clase de escáner (atacar, no auditar) |

Otras limitaciones: no se reproduce byte a byte el ClientHello de un navegador (GREASE, orden de extensiones), solo su preferencia de grupos; algún servidor o middlebox podría comportarse distinto. Claves RSA de más de 8192 bits: `crypto/tls` aborta el handshake (el certificado sí se captura).

**Cómo cubrirlo**: sondas de *ClientHello en crudo* con `golang.org/x/crypto/cryptobyte`: se construye el ClientHello a mano, se envía y se lee **solo** el ServerHello (o la alerta), sin completar el handshake. Es la técnica de testssl.sh para comprobar soporte. Unas 300–500 líneas con tests. Se decidirá en el hito 2.7 a partir de la comparación con testssl.sh (2.6). Se descarta empaquetar OpenSSL o testssl.sh en el worker (shell en la imagen y mucha más superficie de ataque).

**Estado (hito 2.7)**: librería `internal/scanner/rawtls` (2.7.1), integrada en el escáner con la intensidad `full` (2.7.3, arriba). Construye ClientHello SSLv2, SSLv3 y TLS 1.0–1.3 con `cryptobyte` a partir de los registros de IANA incrustados (suites y grupos, `rawtls/iana/`) y lee como mucho 64 KiB por sonda: ServerHello, HelloRetryRequest o alerta y, si se pide, el resto del primer vuelo hasta el ServerKeyExchange (tamaño del primo DH, curva ECDHE). Nunca envía nada después del ClientHello. Lo que no se puede interpretar es un error tipado (`ErrMalformed`, `ErrUnoffered`, `ErrLimit`), nunca "no soportado". Tests contra un servidor `crypto/tls` real (sobre `net.Pipe`), casos malformados escritos a mano y fuzzing nativo de Go (`make fuzz`; 10 s por parser en la CI).

## Ajustes de GODEBUG

`cmd/qrscan` y `cmd/worker` declaran `//go:debug tlssha1=1` y `//go:debug rsa1024min=0` para que `crypto/tls` complete handshakes con servidores muy antiguos (firmas SHA-1 en TLS 1.2, RSA de menos de 1024 bits) y se puedan analizar. El escáner solo observa esas conexiones y nunca confía en ellas. Los tests de la librería se ejecutan sin esos ajustes.

---

## English

`internal/scanner` is a pure library (no HTTP, no database) that probes a TLS endpoint and returns **facts**. Judging them (grade, findings, remediation) is `internal/scoring`'s job.

**A scan** validates the target, resolves it **once**, applies the anti-SSRF policy to **every** address and always connects to the validated IP (never the name; SNI carries the name), so DNS rebinding has no effect; the dialer re-checks the policy on the socket. It then runs 15 to 40 full `crypto/tls` handshakes: a browser-like offer (which group the server **picks**), each hybrid ML-KEM group alone (whether it is **supported**), each classical group, each protocol version, an enumeration of the TLS 1.0, 1.1 and 1.2 suites, and certificate capture (even from failed handshakes) with independent verification. Each probe ends as `supported`, `not_supported` or `error`; an `error` (timeout, network) is never read as "not supported". Limits: 5 s per handshake, 60 s per scan, 4 parallel handshakes per target, 16 per process; no session resumption.

**Anti-SSRF**: only public unicast addresses; the whole host is refused if any address is not. Blocked ranges include `10/8`, `100.64/10`, `127/8`, `169.254/16` (cloud metadata), **`172.16/12` (Docker networks)**, `192.168/16`, documentation, multicast, reserved; IPv6 outside `2000::/3` plus NAT64, 6to4, Teredo and documentation; IPv4-mapped addresses are checked as IPv4. A domain resolving to the VPS's own public IP (`fizsar.com` → `149.56.13.98`) is allowed: it only reaches what the host exposes publicly through the firewall (22, 80, 443), the same as anyone on the Internet; loopback-bound services (API 8080, web 8081) and Docker networks are not reachable that way.

**Not evaluated by crypto/tls** (reported in `Result.NotEvaluated`, shown as "not evaluated", never "not supported"): SSLv2/v3, DHE suites, CCM/Camellia/ARIA/NULL/EXPORT/anonymous suites, the full TLS 1.3 suite list, server cipher preference, legacy groups, non-standard PQC groups, implementation vulnerabilities (Heartbleed, ROBOT…). The browser ClientHello is not reproduced byte for byte. Coverage option: raw ClientHello probes (build the ClientHello, read only the ServerHello), as testssl.sh does; to be decided in milestone 2.7 after the testssl.sh comparison. Bundling OpenSSL/testssl.sh in the worker is ruled out.

**GODEBUG**: `cmd/qrscan` and `cmd/worker` set `tlssha1=1` and `rsa1024min=0` so very old servers can be analyzed; the scanner only observes those connections and never trusts them.

**Intensity (milestone 2.7.3)**: `basic` is exactly the crypto/tls scan above (pinned by a golden file written by the previous scanner); `full` adds raw ClientHello probes (`internal/scanner/rawtls`) after the crypto/tls ones, with the same pinned IP, anti-SSRF dialer, semaphore and 5 s per probe, plus a budget of **48 raw probes per scan**. They never complete a handshake. They cover SSLv2, SSLv3 (and suites accepted only there), every IANA suite negotiable without credentials in TLS 1.2/1.1/1.0 that crypto/tls did not find (DHE with the DH group size from ServerKeyExchange, CCM, Camellia, ARIA, NULL, EXPORT, anonymous…; versions crypto/tls cannot see are detected too), the full TLS 1.3 suite list, TLS 1.3 groups crypto/tls does not probe (ffdhe, brainpool, x448, pure ML-KEM, draft hybrids, pre-standard Kyber) through an empty key_share and the server's HelloRetryRequest, and extra TLS 1.2 curves through ServerKeyExchange. A modern server costs about 10–12 raw probes; the worst lab case 38. Whatever does not complete stays "not evaluated". Results in `Result.Raw`, `Result.Intensity`, `Result.Connections` (optional fields; stored results keep their meaning). The worker reads `SCAN_INTENSITY` (default `full`); in phase 3, `full` only for verified domains. With `full`, "not evaluated" keeps only server cipher preference, credential suites (PSK, SRP, Kerberos, GOST) and implementation vulnerabilities, plus any raw probe that did not finish. `cmd/qrscan -lab` (lab only) accepts only loopback and RFC 1918 addresses; a test builds the production binaries and checks that this code is not in them.

**Schema 2 (milestone 2.8.4)** adds optional fields only (scoring reads none; a v1 result reads the same). `probe_log` records every connection: the **offer is parsed from the ClientHello bytes actually sent** and the **response from the server's bytes up to where encryption starts** (type, chosen version, suite and group, HelloRetryRequest group, alert with level, code and IANA name), plus purpose, status, `error_code`, start and duration. Identical offers are stored once and an enumeration's offers as differences (`like`). Every `error` gets a stable `error_code` (`alert:<n>`, `timeout`, `closed`, `unoffered_<what>`… in `internal/scanner/errcode.go`), the certificate a `trust_error_code`, and each chain certificate its signature and public key OIDs. An alert that arrives encrypted keeps its exact code (from crypto/tls's error) but not its level. Size: fizsar.com goes from 4.0 to 21.2 KB of compact JSON (2–3.5 KB gzipped).
