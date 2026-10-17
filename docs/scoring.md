# Modelo de puntuación / Scoring model

**ES** · [EN](#english)

Versión del modelo: **2026.2** (`scoring.Version`; historial al final). `internal/scoring` convierte un resultado del escáner en una **nota de preparación post-cuántica** (0–100 y letra A–F) y una lista de **hallazgos**. Es determinista: el mismo escaneo da siempre el mismo resultado.

## Dos ejes de riesgo

- **Riesgo cuántico**: el intercambio de claves sin PQC permite *cosechar ahora, descifrar después* (el tráfico de hoy se podrá descifrar cuando exista un ordenador cuántico): es lo urgente. Las firmas clásicas del certificado son un riesgo **futuro**: una firma solo tiene que resistir durante el handshake, así que el peligro empieza el día que exista ese ordenador, no antes.
- **Riesgo actual**: debilidades explotables hoy (certificado no válido, versiones o suites obsoletas…).

Cada eje muestra su severidad más alta (`quantum_risk`, `current_risk`). Los hallazgos `pass` (bien hecho) e `info` no cuentan como riesgo.

## Nota (0–100)

| Bloque | Puntos | Criterio |
|---|---|---|
| Intercambio de claves | **50** / 30 / 0 | 50 si con la oferta de un navegador actual el servidor **elige** un híbrido ML-KEM; 30 si lo **soporta** pero no lo elige (el tráfico real sigue siendo clásico); 0 si no lo soporta |
| TLS 1.3 | 15 | Los grupos híbridos solo existen en TLS 1.3 |
| Forward secrecy | 10 | Todas las suites aceptadas, en cualquier versión activa (TLS 1.0–1.2 y SSLv3), con intercambio efímero (ECDHE o DHE), o solo TLS 1.3. Hasta 2026.1 solo se miraba TLS 1.2 |
| Variedad de híbridos | 5 | Más de un grupo híbrido (p. ej. `SecP256r1MLKEM768` para clientes FIPS) |
| Certificado y agilidad | 20 | 5 válido (cadena de confianza, nombre, fechas) · 5 clave (RSA ≥ 2048, ECDSA ≥ 256, Ed25519) · 5 firma (SHA-256 o superior) · 5 renovación automatizada (ver abajo) |

Letra por nota: **A** ≥ 90 · **B** ≥ 75 · **C** ≥ 60 · **D** ≥ 40 · **F** < 40.

### Por qué estos pesos

- La idea acordada: **un TLS clásico bien configurado saca D** y la **F queda para lo roto**. Un servidor sin PQC con TLS 1.3, forward secrecy y un certificado comercial RSA-2048 de un año suma 15 + 10 + 15 = **40 (D)**; con un certificado automatizado (Let's Encrypt), **45 (D)**. Con la propuesta inicial (55/35/0 y certificado de 15) ese mismo servidor comercial quedaba en 35 (F), así que el intercambio de claves baja a 50/30 y el certificado sube a 20, con el criterio "válido" añadido.
- Soportar ML-KEM sin preferirlo vale 30 y no 50: lo que protege a los usuarios es lo que negocia su navegador. Con todo lo demás bien, eso da **75 (B)**.
- Un servidor **solo TLS 1.2** bien configurado suma 30 (o 25 con certificado comercial) y llega a **40 (D)** por el suelo (abajo). Además recibe el hallazgo alto del eje cuántico *"Sin TLS 1.3 no es posible adoptar intercambio de claves híbrido ML-KEM"* (`TLS13_MISSING`).

### Suelo: la F es para lo roto

Si **no hay ningún tope F** (nada roto), la nota mínima es **40**, el fondo de la D. El desglose lo muestra como un ajuste explícito (`floor_points`, "Suelo: nada roto"). **Solo un tope F anula el suelo** (desde 2026.2; en 2026.1 también lo anulaba el tope C): con un tope C (débil: RC4, 3DES, sin forward secrecy, clave o firma débiles, SSLv3, DH de 1024 bits) la nota se queda en 40 o más y el tope deja la letra en **D o C**; con un tope B (mejorable: TLS 1.0/1.1, CBC), entre D y B. Así se respeta "F = roto, C = débil": TLS clásico sin nada roto nunca es F, sea cual sea su certificado o si le falta TLS 1.3.

Por qué un suelo y no más puntos para TLS 1.2: con los pesos aprobados, dar puntos a TLS 1.2 dejaba un servidor solo TLS 1.2 con certificado comercial en 35 (F), y subirlo más exigía igualar TLS 1.2 con TLS 1.3. El suelo cumple la regla "F = roto" sin tocar los pesos.

### Renovación automatizada (5 puntos)

El cambio a certificados post-cuánticos (ML-DSA o híbridos) exigirá reemitir todos los certificados: quien lo tenga automatizado lo hará sin esfuerzo. Se considera automatizado si:

1. la **validez es de 47 días o menos**, o
2. lo emite una **autoridad que funciona con ACME por defecto**: Let's Encrypt, Google Trust Services o ZeroSSL (por organización del emisor).

Por qué no "100 días o menos": el CA/Browser Forum reduce la validez máxima a **200 días (15/03/2026), 100 (15/03/2027) y 47 (15/03/2029)**, así que pronto *todos* los certificados tendrán 100 días o menos y el criterio dejaría de distinguir. 47 días sigue distinguiendo hasta 2029 (hoy solo se llega ahí automatizando), y el emisor ACME reconoce la automatización aunque el certificado dure 90 días. **En 2029 el criterio 1 dejará de distinguir**: habrá que revisarlo (y subir la versión del modelo). Limitación: una CA comercial con ACME pero certificados de un año no puntúa; no se puede saber desde fuera cómo se renueva.

## Topes por riesgo actual (regla del peor caso)

La letra final es la **peor** entre la de la nota y el tope más severo:

| Tope | Cuándo |
|---|---|
| **F** | Certificado caducado o aún no válido, cadena no válida o autofirmada, nombre que no coincide · *desde 2026.2*: SSLv2, suites NULL o solo de integridad, EXPORT, anónimas o DES simple, DH de menos de 1024 bits |
| **C** | Clave RSA < 2048 (o ECDSA < 256), firma SHA-1/MD5, RC4 o 3DES aceptados, ninguna suite con forward secrecy · *desde 2026.2*: SSLv3, DH de 1024 a 2047 bits |
| **B** | TLS 1.0 o 1.1 activos, suites CBC |

Ejemplo: un sitio con PQC y todo perfecto pero con el certificado caducado tiene nota 95 y letra **F**. El tope F anula además el suelo de 40; el C no (ver "Suelo").

## Modelo 2026.2: lo que ven las sondas en crudo

Con intensidad `full` (`docs/scanner.md`), el escáner ve lo que `crypto/tls` no podía: SSLv2/SSLv3, todas las suites de IANA, el tamaño del grupo DH, la lista completa de TLS 1.3 y los grupos no habituales. El modelo 2026.2 los juzga con la regla de siempre: **F = roto** (se rompe o se suplanta hoy con medios al alcance de cualquiera), **C = débil** (atacable con recursos serios o en condiciones concretas), **B = mejorable**. Los atributos de cada suite (cifrado, bits, modo, EXPORT, anónima) salen del registro de IANA incrustado en `rawtls`.

| Hallazgo | Eje · severidad | Efecto | Por qué (referencia) |
|---|---|---|---|
| `PROTO_SSL2`: SSLv2 activo | actual · crítico | **tope F** | Roto por diseño y prohibido desde 2011 (**RFC 6176**). Tenerlo activo permite **DROWN** (CVE-2016-0800): con una clave compartida, descifra también conexiones TLS modernas. |
| `PROTO_SSL3`: SSLv3 activo | actual · alto | **tope C** | Prohibido desde 2015 (**RFC 7568**) por **POODLE** (CVE-2014-3566). Es C y no F porque exige forzar la versión y muchas peticiones por byte recuperado, y ningún navegador actual lo negocia. |
| `SUITE_NULL`: suites sin cifrado | actual · crítico | **tope F** | El tráfico viaja en claro (RFC 5246, apéndice C). |
| `SUITE_INTEGRITY_ONLY`: TLS 1.3 solo de integridad | actual · crítico | **tope F** | Ver abajo (**RFC 9150**). |
| `SUITE_EXPORT`: suites EXPORT | actual · crítico | **tope F** | Claves de 40–56 bits; **FREAK** (CVE-2015-0204) y **Logjam** (CVE-2015-4000) permiten degradar la conexión a ellas y romperla en minutos. |
| `SUITE_ANON`: suites anónimas (DH_anon, ECDH_anon) | actual · crítico | **tope F** | Sin certificado no hay autenticación del servidor: cualquier intermediario puede suplantarlo (RFC 5246, apéndice F.1.1.1). |
| `SUITE_DES`: DES simple (56 bits) | actual · crítico | **tope F** | Roto por fuerza bruta desde 1998 (EFF DES Cracker); retirado por NIST (FIPS 46-3, 2005) y de TLS (RFC 5469). |
| `DH_WEAK`: grupo DH < 1024 bits | actual · crítico | **tope F** | **Logjam** (Adrian et al., 2015): los grupos de 512 bits se rompen en tiempo real tras un precálculo de días. |
| `DH_SMALL`: grupo DH de 1024 a 2047 bits | actual · alto | **tope C** | El mismo estudio estimó que un actor estatal puede romper los grupos de 1024 bits más usados; NIST (SP 800-131A) exige 2048 bits o más desde 2014. C y no F: hace falta un presupuesto de nivel estatal. *Aclaración*: 1024 es el caso aprobado en el plan; aplico lo mismo a los poco habituales 1025–2047 (p. ej. 1536), que tampoco llegan a los 2048. |
| `DH_OK`: grupos DH ≥ 2048 | actual · correcto | — | Las suites DHE con grupos así cuentan como forward secrecy. |
| Suites RSA, CBC, RC4 o 3DES encontradas por las sondas en crudo (Camellia, ARIA, SEED, IDEA…) | — | reglas de 2026.1 | Mismas reglas que antes, ahora sobre todas las suites: forward secrecy (TLS 1.2), RC4/3DES → C, CBC → B. |
| `SUITE_CCM8`: etiqueta de 64 bits (TLS 1.2 o 1.3) | actual · bajo | sin tope | No está roto: es AEAD con menos margen, pensado para dispositivos limitados (RFC 6655, RFC 8446). |
| `GROUP_WEAK_CURVES`: secp224r1, secp192r1, curvas binarias, explícitas | actual · bajo | sin tope | Desaconsejadas (RFC 8422, RFC 9847), pero los navegadores no las ofrecen: solo afectan a clientes antiguos. |
| `GROUPS_EXTRA`: ffdhe, brainpool, x448… | actual · informativo | — | Válidos, poco habituales. |
| `SUITE_MODERN_AEAD`: AEGIS, Ascon | actual · informativo | — | AEAD modernos (Ascon es el estándar ligero del NIST, SP 800-232): ni suman ni restan, y no cuentan como débiles. |
| `TLS13_SUITES_AEAD`: lista de TLS 1.3 sin CCM_8 ni solo-integridad | actual · correcto | — | |
| `KEX_PURE_MLKEM`: ML-KEM sin componente clásico | cuántico · informativo | +5 en **variedad** si hay más de un grupo PQC estándar (`KEX_MULTIPLE_PQC`) | Estándar (FIPS 203) y preferido por CNSA 2.0, pero ningún navegador lo ofrece: no protege el tráfico real, así que **no** cuenta en el bloque de 50/30 del intercambio de claves. |
| `KEX_EXPERIMENTAL_PQC_ONLY`: solo Kyber pre-estándar o híbridos en borrador | cuántico · medio | 0 puntos (como sin PQC); se mantiene `KEX_NO_HYBRID` | Kyber (`X25519Kyber768Draft00`, 0x6399) es anterior a FIPS 203 e IANA lo marca como obsoleto (RFC 10024); Chrome lo retiró a finales de 2024, así que hoy no protege a nadie. Medio y no crítico porque indica una migración empezada: el remedio es actualizar, no empezar de cero. |
| `KEX_EXPERIMENTAL_PQC_EXTRA`: lo mismo junto al estándar | cuántico · informativo | — | Conviene retirarlo, pero no es un riesgo. |

### Suites "solo de integridad" de TLS 1.3 (RFC 9150)

`TLS_SHA256_SHA256` y `TLS_SHA384_SHA384` autentican y protegen la integridad de los datos, pero **no los cifran**. Existen para entornos industriales y de control (IoT, SCADA, redes de operación) donde la información no es secreta, la latencia y el consumo importan, y la normativa o la operación exigen poder inspeccionar el tráfico. En una **web pública** no tienen justificación: todo lo que el usuario envía (contraseñas, cookies, formularios) viajaría en claro para cualquiera en el camino, igual que con las suites NULL. Por eso tienen tope **F**. El RFC 9150 las limita a usos en los que la confidencialidad no hace falta, e IANA las marca como desaconsejadas (D, RFC 9847).

### Qué cambia respecto a 2026.1

- **Dos cambios afectan a cualquier escaneo**, también a los `basic`:
  - **Suelo**: solo lo anula un tope F. Un servidor débil (tope C) que antes caía por debajo de 40 (letra F) ahora queda en **40 · D**. Ninguna nota baja por esto.
  - **Forward secrecy en todas las versiones**: una suite sin intercambio efímero aceptada en TLS 1.0 o 1.1 (no solo en 1.2) cuesta ahora los 10 puntos (`FS_PARTIAL`), y un servidor solo TLS 1.0 con RSA recibe `FS_NONE` (tope C). Las suites anónimas cuentan como efímeras: su problema (sin autenticación) tiene su propio tope F.
- **Los hallazgos nuevos** solo aparecen con sondas en crudo (intensidad `full`): sin ellas no se emite ninguno (lo comprueba `TestBasicScansScoreAsBefore`).
- Con sondas en crudo, una nota **puede bajar** (nuevos topes, suites sin forward secrecy que `crypto/tls` no veía, versiones antiguas activas que solo ven las sondas en crudo) o **subir 5 puntos** (ML-KEM puro en la variedad).
- Las notas guardadas no se recalculan: cada escaneo conserva su `scoring_version`. Los 6 escaneos de producción siguen en 2026.1; los nuevos llevarán 2026.2.
- Nuestros dominios (Caddy, TLS de Go) no tienen nada de lo nuevo: siguen en **A · 95** con intensidad `full` y 2026.2 (comprobado con `qrscan`, `docs/validation.md`).

Pruebas: `internal/scoring/lab_test.go` puntúa el resultado real de los 9 servidores del laboratorio (con un certificado válido, para juzgar solo la configuración TLS) y casos sintéticos para lo que ningún servidor del laboratorio tiene (solo integridad, AEGIS, Kyber junto a ML-KEM).

## Lo que no puntúa

- **Lo "no evaluado"** (lo que `crypto/tls` no puede comprobar: SSLv3, suites DHE…, ver `docs/scanner.md`) **nunca suma ni resta**: se muestra aparte. Un test lo verifica (`TestNotEvaluatedNeverAffectsTheScore`).
- **Una sonda con error** (timeout, red) no cuenta como soportado ni como no soportado: no da puntos y genera `SCAN_PARTIAL` (y `KEX_HYBRID_UNKNOWN` si afecta al intercambio de claves).
- **Informativos sin efecto en la nota**: firma clásica del certificado (`CERT_CLASSICAL_SIGNATURE`), AES-128 y Grover (`SYM_AES128`: NIST sigue considerando AES-128 adecuado), transparencia de certificados (`CERT_CT`, distingue SCT incrustados y entregados en el handshake).

## Hallazgos

Cada hallazgo tiene `code` estable, `axis` (`quantum`/`current`), `severity` (`critical`, `high`, `medium`, `low`, `info`, `pass`), `category` y `params`. En base de datos solo se guardan código y parámetros; los textos ES/EN (título, explicación, remediación) viven en `internal/scoring/catalog.go`, así que se pueden corregir sin migrar datos. Un test comprueba que todos los códigos tienen texto en ambos idiomas y que ningún marcador queda sin rellenar.

### Referencias por hallazgo

La fuente de cada regla. `TestEveryFindingHasAReference` (`internal/scoring/references_test.go`) comprueba que cada código del catálogo tiene aquí una fila con al menos un RFC, CVE, borrador del IETF, publicación del NIST/FIPS o votación del CA/Browser Forum, y que no sobra ninguna fila. Quedan fuera tres códigos que no juzgan al servidor, sino que dicen que el escaneo no pudo comprobar algo (no puntúan, ver «Lo que no puntúa»): `SCAN_PARTIAL`, `KEX_HYBRID_UNKNOWN` y `CERT_MISSING`.

| Código | Referencia |
|---|---|
| `KEX_HYBRID_PREFERRED`, `KEX_HYBRID_NOT_PREFERRED`, `KEX_SINGLE_HYBRID`, `KEX_MULTIPLE_HYBRIDS` | draft-ietf-tls-ecdhe-mlkem (X25519MLKEM768, SecP256r1MLKEM768, SecP384r1MLKEM1024); FIPS 203 (ML-KEM) |
| `KEX_NO_HYBRID` | draft-ietf-tls-ecdhe-mlkem; FIPS 203 |
| `KEX_PURE_MLKEM`, `KEX_MULTIPLE_PQC` | FIPS 203; draft-ietf-tls-mlkem |
| `KEX_EXPERIMENTAL_PQC_ONLY`, `KEX_EXPERIMENTAL_PQC_EXTRA` | draft-tls-westerbaan-xyber768d00 (borrador previo, sustituido por draft-ietf-tls-ecdhe-mlkem) |
| `TLS13_SUPPORTED`, `TLS13_MISSING` | RFC 8446; draft-ietf-tls-ecdhe-mlkem (los grupos híbridos solo existen en TLS 1.3) |
| `TLS13_SUITES_AEAD` | RFC 8446, sección 9.1 y apéndice B.4 |
| `SUITE_INTEGRITY_ONLY` | RFC 9150 |
| `SUITE_CCM8` | RFC 6655; RFC 8446 |
| `SUITE_MODERN_AEAD` | NIST SP 800-232 (Ascon) |
| `SUITE_NULL` | RFC 5246, apéndice C |
| `SUITE_EXPORT` | CVE-2015-0204 (FREAK); CVE-2015-4000 (Logjam) |
| `SUITE_ANON` | RFC 5246, apéndice F.1.1.1 |
| `SUITE_DES` | RFC 5469; FIPS 46-3 (retirado) |
| `SUITE_RC4` | RFC 7465 |
| `SUITE_3DES` | CVE-2016-2183 (Sweet32) |
| `SUITE_CBC` | CVE-2013-0169 (Lucky Thirteen); RFC 7457 |
| `SYM_AES128` | NIST SP 800-57 Part 1 Rev. 5 (AES-128 da 128 bits de seguridad) |
| `PROTO_SSL2` | RFC 6176; CVE-2016-0800 (DROWN) |
| `PROTO_SSL3` | RFC 7568; CVE-2014-3566 (POODLE) |
| `PROTO_LEGACY_TLS` | RFC 8996 |
| `FS_ALL`, `FS_PARTIAL`, `FS_NONE` | RFC 9325 (BCP 195) |
| `DH_WEAK` | CVE-2015-4000 (Logjam) |
| `DH_SMALL`, `DH_OK` | NIST SP 800-131A Rev. 2; RFC 7919 |
| `GROUP_WEAK_CURVES` | RFC 8422; RFC 9847 |
| `GROUPS_EXTRA` | RFC 7919 (ffdhe); RFC 8734 (brainpool en TLS 1.3); RFC 7748 (x448) |
| `CERT_VALID`, `CERT_UNTRUSTED`, `CERT_SELF_SIGNED` | RFC 5280, sección 6 (validación de la cadena) |
| `CERT_EXPIRED`, `CERT_NOT_YET_VALID`, `CERT_EXPIRES_SOON` | RFC 5280, sección 4.1.2.5 (validez) |
| `CERT_HOSTNAME_MISMATCH` | RFC 9525 |
| `CERT_WEAK_KEY` | NIST SP 800-131A Rev. 2 |
| `CERT_WEAK_SIGNATURE` | RFC 9155; NIST SP 800-131A Rev. 2 |
| `CERT_CLASSICAL_SIGNATURE` | FIPS 204 (ML-DSA); FIPS 205 (SLH-DSA) |
| `CERT_CT`, `CERT_CT_MISSING` | RFC 6962 |
| `CERT_AUTOMATED_ACME`, `CERT_NOT_AUTOMATED` | RFC 8555 (ACME) |
| `CERT_AUTOMATED_SHORT` | CA/Browser Forum, votación SC-081 (validez máxima de 47 días en 2029) |

## Versionado del modelo

Cada resultado lleva `scoring_version`. **Cambiar pesos, umbrales, topes o las reglas que emiten hallazgos obliga a subir `scoring.Version`** (formato `AAAA.N`: `2026.1`, `2026.2`…), para que una nota guardada se interprete siempre con el modelo con que se calculó. Corregir la redacción de un texto no cambia la versión. La columna para guardarla llega con la tabla `scans` (hito 2.3).

Historial: **2026.1** (13/10/2026, primera versión en producción) · **2026.2** (15/10/2026, hito 2.7.4: hallazgos y topes de las sondas en crudo; ver arriba).

---

## English

Model version **2026.1**. `internal/scoring` turns a scan into a **post-quantum readiness score** (0–100, grade A–F) and **findings**, deterministically.

**Two risk axes**: *quantum* (key exchange without PQC = harvest now, decrypt later, the urgent part; classical certificate signatures = **future** risk, since a signature only has to hold during the handshake) and *current* (weaknesses exploitable today). Each axis reports its worst severity; `pass` and `info` are not risks.

**Score**: key exchange **50** if a browser-like offer gets a hybrid ML-KEM group, 30 if it is supported but not preferred, 0 otherwise · TLS 1.3 **15** · forward secrecy on every accepted suite in every active version up to TLS 1.2 (or TLS 1.3 only) **10** · more than one hybrid group **5** · certificate **20** (5 valid, 5 key RSA ≥ 2048 / ECDSA ≥ 256 / Ed25519, 5 signature SHA-256+, 5 automated renewal). Grades: A ≥ 90, B ≥ 75, C ≥ 60, D ≥ 40, F < 40.

**Why**: a well-configured classical server (TLS 1.3, forward secrecy, commercial RSA-2048 certificate) scores 40 (D), or 45 with an automated certificate; F is for broken setups. The initial 55/35 + 15 proposal left that server at 35 (F), hence 50/30 and a 20-point certificate block. Supported-but-not-preferred ML-KEM scores 75 (B).

**Floor**: with no F cap (nothing broken), the score is at least **40** (bottom of D), shown as `floor_points`. Only an F cap cancels it (since 2026.2; in 2026.1 a C cap did too): a weak server (C cap) stays at 40 or more and ends in D or C; a B cap (TLS 1.0/1.1, CBC) keeps the floor. So a well-configured TLS 1.2-only server gets 40 (D) whatever its certificate, plus the high quantum-axis finding "Without TLS 1.3, hybrid ML-KEM key exchange cannot be adopted" (`TLS13_MISSING`). A floor rather than points for TLS 1.2, because with the approved weights extra TLS 1.2 points still left a commercial-certificate TLS 1.2 server at 35 (F).

**Automated renewal**: validity ≤ 47 days, or issued by an ACME-first CA (Let's Encrypt, Google Trust Services, ZeroSSL). "≤ 100 days" would stop differentiating as the CA/B Forum caps validity at 200 days (March 2026), 100 (2027) and 47 (2029); the 47-day rule must be revisited in 2029.

**Caps** (worst case): F for an expired, not-yet-valid, untrusted or self-signed certificate or a name mismatch; C for RSA < 2048, SHA-1/MD5 signatures, RC4/3DES or no forward secrecy at all; B for TLS 1.0/1.1 or CBC suites.

**Never scored**: what crypto/tls cannot evaluate (shown separately, tested), probe errors (`SCAN_PARTIAL`, `KEX_HYBRID_UNKNOWN`), and informational findings (classical signature, AES-128/Grover, Certificate Transparency with embedded vs handshake SCTs).

**Findings** store only `code` and `params`; ES/EN texts live in `catalog.go` and are tested for completeness. **References**: the source of every finding (RFC, CVE, IETF draft, NIST/FIPS or CA/Browser Forum ballot) is in the "Referencias por hallazgo" table above; `TestEveryFindingHasAReference` checks every catalog code has one, except the three that only say a check could not be made (`SCAN_PARTIAL`, `KEX_HYBRID_UNKNOWN`, `CERT_MISSING`).

**Versioning**: every result carries `scoring_version`. Changing weights, thresholds, caps or finding rules **requires bumping `scoring.Version`** (`YYYY.N`); rewording a text does not. The column to store it arrives with the `scans` table (milestone 2.3).

**Model 2026.2 (milestone 2.7.4)**: with intensity full the scanner also sees SSLv2/SSLv3, every IANA suite, DH group sizes, the full TLS 1.3 list and uncommon groups, judged with the same rule (F broken, C weak, B improvable). New caps, each with its reference: **F** for SSLv2 (RFC 6176, DROWN CVE-2016-0800), NULL suites, TLS 1.3 integrity-only suites (RFC 9150: meant for industrial/IoT networks where data is not secret; on a public website traffic would travel in clear), EXPORT (FREAK CVE-2015-0204, Logjam CVE-2015-4000), anonymous suites (no server authentication), single DES (brute-forced since 1998, RFC 5469) and DH groups under 1024 bits (Logjam); **C** for SSLv3 (RFC 7568, POODLE CVE-2014-3566) and DH groups of 1024–2047 bits (nation-state reach per the Logjam study; NIST SP 800-131A requires 2048+). Low, no cap: CCM_8 (64-bit tag) and weak or binary curves (browsers never offer them). Informational: AEGIS and Ascon (modern AEADs, never a penalty), ffdhe/brainpool/x448, sound TLS 1.3 suites. Pure ML-KEM (FIPS 203, CNSA 2.0) counts towards **variety** (+5 with more than one standard PQC group) but not towards the key exchange block, since browsers do not offer it. Pre-standard Kyber or draft hybrids alone (`KEX_EXPERIMENTAL_PQC_ONLY`, medium) earn no points: browsers dropped them, so they protect nobody. Two changes apply to every scan, basic included: only an F cap cancels the floor (a weak, C-capped server under 40 now gets 40 · D instead of F), and forward secrecy is judged over every active version (TLS 1.0–1.2 and SSLv3), so a TLS 1.0-only RSA server gets `FS_NONE`; anonymous suites count as ephemeral (their lack of authentication has its own F cap). The new findings need raw results (tested). Stored scores are never recomputed. Our own domains stay at A · 95.
