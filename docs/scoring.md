# Modelo de puntuación / Scoring model

**ES** · [EN](#english)

Versión del modelo: **2026.1** (`scoring.Version`). `internal/scoring` convierte un resultado del escáner en una **nota de preparación post-cuántica** (0–100 y letra A–F) y una lista de **hallazgos**. Es determinista: el mismo escaneo da siempre el mismo resultado.

## Dos ejes de riesgo

- **Riesgo cuántico**: el intercambio de claves sin PQC permite *cosechar ahora, descifrar después* (el tráfico de hoy se podrá descifrar cuando exista un ordenador cuántico): es lo urgente. Las firmas clásicas del certificado son un riesgo **futuro**: una firma solo tiene que resistir durante el handshake, así que el peligro empieza el día que exista ese ordenador, no antes.
- **Riesgo actual**: debilidades explotables hoy (certificado no válido, versiones o suites obsoletas…).

Cada eje muestra su severidad más alta (`quantum_risk`, `current_risk`). Los hallazgos `pass` (bien hecho) e `info` no cuentan como riesgo.

## Nota (0–100)

| Bloque | Puntos | Criterio |
|---|---|---|
| Intercambio de claves | **50** / 30 / 0 | 50 si con la oferta de un navegador actual el servidor **elige** un híbrido ML-KEM; 30 si lo **soporta** pero no lo elige (el tráfico real sigue siendo clásico); 0 si no lo soporta |
| TLS 1.3 | 15 | Los grupos híbridos solo existen en TLS 1.3 |
| Forward secrecy | 10 | Todas las suites de TLS 1.2 con intercambio efímero (ECDHE), o solo TLS 1.3 |
| Variedad de híbridos | 5 | Más de un grupo híbrido (p. ej. `SecP256r1MLKEM768` para clientes FIPS) |
| Certificado y agilidad | 20 | 5 válido (cadena de confianza, nombre, fechas) · 5 clave (RSA ≥ 2048, ECDSA ≥ 256, Ed25519) · 5 firma (SHA-256 o superior) · 5 renovación automatizada (ver abajo) |

Letra por nota: **A** ≥ 90 · **B** ≥ 75 · **C** ≥ 60 · **D** ≥ 40 · **F** < 40.

### Por qué estos pesos

- La idea acordada: **un TLS clásico bien configurado saca D** y la **F queda para lo roto**. Un servidor sin PQC con TLS 1.3, forward secrecy y un certificado comercial RSA-2048 de un año suma 15 + 10 + 15 = **40 (D)**; con un certificado automatizado (Let's Encrypt), **45 (D)**. Con la propuesta inicial (55/35/0 y certificado de 15) ese mismo servidor comercial quedaba en 35 (F), así que el intercambio de claves baja a 50/30 y el certificado sube a 20, con el criterio "válido" añadido.
- Soportar ML-KEM sin preferirlo vale 30 y no 50: lo que protege a los usuarios es lo que negocia su navegador. Con todo lo demás bien, eso da **75 (B)**.
- Un servidor **solo TLS 1.2** bien configurado suma 30 (o 25 con certificado comercial) y llega a **40 (D)** por el suelo (abajo). Además recibe el hallazgo alto del eje cuántico *"Sin TLS 1.3 no es posible adoptar intercambio de claves híbrido ML-KEM"* (`TLS13_MISSING`).

### Suelo: la F es para lo roto

Si **no hay ningún tope F ni C** (nada roto ni débil: certificado válido, sin RC4/3DES, con forward secrecy, clave y firma correctas), la nota mínima es **40**, el fondo de la D. El desglose lo muestra como un ajuste explícito (`floor_points`, "Suelo: nada roto ni débil"). Un tope B (TLS 1.0/1.1, CBC) no quita el suelo: es mejorable, no roto. Así, TLS clásico bien hecho nunca es F, sea cual sea su certificado o si le falta TLS 1.3.

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
| **F** | Certificado caducado o aún no válido, cadena no válida o autofirmada, nombre que no coincide |
| **C** | Clave RSA < 2048 (o ECDSA < 256), firma SHA-1/MD5, RC4 o 3DES aceptados, ninguna suite con forward secrecy |
| **B** | TLS 1.0 o 1.1 activos, suites CBC |

Ejemplo: un sitio con PQC y todo perfecto pero con el certificado caducado tiene nota 95 y letra **F**. Los topes F y C anulan además el suelo de 40.

## Lo que no puntúa

- **Lo "no evaluado"** (lo que `crypto/tls` no puede comprobar: SSLv3, suites DHE…, ver `docs/scanner.md`) **nunca suma ni resta**: se muestra aparte. Un test lo verifica (`TestNotEvaluatedNeverAffectsTheScore`).
- **Una sonda con error** (timeout, red) no cuenta como soportado ni como no soportado: no da puntos y genera `SCAN_PARTIAL` (y `KEX_HYBRID_UNKNOWN` si afecta al intercambio de claves).
- **Informativos sin efecto en la nota**: firma clásica del certificado (`CERT_CLASSICAL_SIGNATURE`), AES-128 y Grover (`SYM_AES128`: NIST sigue considerando AES-128 adecuado), transparencia de certificados (`CERT_CT`, distingue SCT incrustados y entregados en el handshake).

## Hallazgos

Cada hallazgo tiene `code` estable, `axis` (`quantum`/`current`), `severity` (`critical`, `high`, `medium`, `low`, `info`, `pass`), `category` y `params`. En base de datos solo se guardan código y parámetros; los textos ES/EN (título, explicación, remediación) viven en `internal/scoring/catalog.go`, así que se pueden corregir sin migrar datos. Un test comprueba que todos los códigos tienen texto en ambos idiomas y que ningún marcador queda sin rellenar.

## Versionado del modelo

Cada resultado lleva `scoring_version`. **Cambiar pesos, umbrales, topes o las reglas que emiten hallazgos obliga a subir `scoring.Version`** (formato `AAAA.N`: `2026.1`, `2026.2`…), para que una nota guardada se interprete siempre con el modelo con que se calculó. Corregir la redacción de un texto no cambia la versión. La columna para guardarla llega con la tabla `scans` (hito 2.3).

---

## English

Model version **2026.1**. `internal/scoring` turns a scan into a **post-quantum readiness score** (0–100, grade A–F) and **findings**, deterministically.

**Two risk axes**: *quantum* (key exchange without PQC = harvest now, decrypt later, the urgent part; classical certificate signatures = **future** risk, since a signature only has to hold during the handshake) and *current* (weaknesses exploitable today). Each axis reports its worst severity; `pass` and `info` are not risks.

**Score**: key exchange **50** if a browser-like offer gets a hybrid ML-KEM group, 30 if it is supported but not preferred, 0 otherwise · TLS 1.3 **15** · forward secrecy on every TLS 1.2 suite (or TLS 1.3 only) **10** · more than one hybrid group **5** · certificate **20** (5 valid, 5 key RSA ≥ 2048 / ECDSA ≥ 256 / Ed25519, 5 signature SHA-256+, 5 automated renewal). Grades: A ≥ 90, B ≥ 75, C ≥ 60, D ≥ 40, F < 40.

**Why**: a well-configured classical server (TLS 1.3, forward secrecy, commercial RSA-2048 certificate) scores 40 (D), or 45 with an automated certificate; F is for broken setups. The initial 55/35 + 15 proposal left that server at 35 (F), hence 50/30 and a 20-point certificate block. Supported-but-not-preferred ML-KEM scores 75 (B).

**Floor**: with no F or C cap (nothing broken or weak), the score is at least **40** (bottom of D), shown as `floor_points`. A B cap (TLS 1.0/1.1, CBC) keeps the floor. So a well-configured TLS 1.2-only server gets 40 (D) whatever its certificate, plus the high quantum-axis finding "Without TLS 1.3, hybrid ML-KEM key exchange cannot be adopted" (`TLS13_MISSING`). A floor rather than points for TLS 1.2, because with the approved weights extra TLS 1.2 points still left a commercial-certificate TLS 1.2 server at 35 (F).

**Automated renewal**: validity ≤ 47 days, or issued by an ACME-first CA (Let's Encrypt, Google Trust Services, ZeroSSL). "≤ 100 days" would stop differentiating as the CA/B Forum caps validity at 200 days (March 2026), 100 (2027) and 47 (2029); the 47-day rule must be revisited in 2029.

**Caps** (worst case): F for an expired, not-yet-valid, untrusted or self-signed certificate or a name mismatch; C for RSA < 2048, SHA-1/MD5 signatures, RC4/3DES or no forward secrecy at all; B for TLS 1.0/1.1 or CBC suites.

**Never scored**: what crypto/tls cannot evaluate (shown separately, tested), probe errors (`SCAN_PARTIAL`, `KEX_HYBRID_UNKNOWN`), and informational findings (classical signature, AES-128/Grover, Certificate Transparency with embedded vs handshake SCTs).

**Findings** store only `code` and `params`; ES/EN texts live in `catalog.go` and are tested for completeness.

**Versioning**: every result carries `scoring_version`. Changing weights, thresholds, caps or finding rules **requires bumping `scoring.Version`** (`YYYY.N`); rewording a text does not. The column to store it arrives with the `scans` table (milestone 2.3).
