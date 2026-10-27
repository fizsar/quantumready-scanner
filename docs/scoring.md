# Modelo de puntuación / Scoring model

**ES** · [EN](#english)

Versión del modelo: **2026.3** (`scoring.Version`; historial al final). `internal/scoring` convierte un resultado del escáner en **dos ejes**, «Seguridad hoy» (0–100 y letra A–F) y «Preparación cuántica» (0–100 en tres niveles), y una lista de **hallazgos**. Es determinista: el mismo escaneo da siempre el mismo resultado. Los escaneos analizados con un modelo anterior conservan su juicio (ver «Escaneos de modelos anteriores»).

## Dos ejes

- **Seguridad hoy**: lo que protege la conexión frente a un atacante actual. Nota de 0 a 100 y letra de A a F, con los topes del peor caso (abajo).
- **Preparación cuántica**: lo que protege frente a un ordenador cuántico. El intercambio de claves sin PQC permite *cosechar ahora, descifrar después* (el tráfico de hoy se podrá descifrar cuando exista ese ordenador): es lo urgente. Las firmas clásicas del certificado son un riesgo **futuro**: una firma solo tiene que resistir durante el handshake, así que el peligro empieza el día que exista ese ordenador, no antes. Nota de 0 a 100 en tres niveles, sin letra.

Los hallazgos siguen teniendo dos ejes de **riesgo**, cuántico y actual, y cada uno muestra su severidad más alta (`quantum_risk`, `current_risk`); los `pass` (bien hecho) e `info` no cuentan como riesgo. Dentro del riesgo cuántico, el informe separa **Shor** (intercambio de claves y firmas) de **Grover** (cifrado simétrico, solo informativo: `SYM_AES128`).

## Seguridad hoy (0–100, A–F)

| Bloque | Puntos | Criterio |
|---|---|---|
| TLS 1.3 | 30 | La versión que hoy da la mejor protección |
| Forward secrecy | 25 | Todas las suites aceptadas, en cualquier versión activa (TLS 1.0–1.2 y SSLv3), con intercambio efímero (ECDHE o DHE), o solo TLS 1.3. **Parcial: 0** |
| Certificado | 45 | 15 válido (cadena de confianza, nombre, fechas) · 10 clave (RSA ≥ 2048, ECDSA ≥ 256, Ed25519, ML-DSA o SLH-DSA) · 10 firma (SHA-256 o superior) · 10 renovación automatizada (ver abajo) |

Letra por nota: **A** ≥ 90 · **B** ≥ 75 · **C** ≥ 60 · **D** ≥ 40 · **F** < 40, y la letra final es la peor entre esa y el tope más severo. **Sin suelo** desde 2026.3: el suelo de 40 de 2026.2 compensaba que la falta de PQC restara la mitad de la nota única; con dos ejes ya no resta aquí. Un certificado que no se ve puntúa 0 en su bloque (`CERT_MISSING`, informativo, sin tope).

## Preparación cuántica (0–100, en niveles)

| Bloque | Puntos | Criterio |
|---|---|---|
| Intercambio de claves | **50** / 30 / 0 | 50 si con la oferta de un navegador actual el servidor **elige** un híbrido ML-KEM; 30 si solo lo acepta cuando se le ofrece solo (el tráfico real sigue siendo clásico); 0 si no lo soporta |
| TLS 1.3 | 10 | Los grupos híbridos solo existen en TLS 1.3 |
| Variedad | 10 | Más de un grupo post-cuántico estándar (p. ej. `SecP256r1MLKEM768` para clientes FIPS; ML-KEM puro cuenta aquí) |
| Renovación automatizada | 15 | Agilidad: pasar a certificados post-cuánticos exigirá reemitirlos todos |
| Certificado post-cuántico | 15 | El servidor presenta un certificado ML-DSA o SLH-DSA cuando el cliente lo ofrece (`CERT_PQ_SIGNATURE`, hito 2.8.5) |
| Kyber obsoleto | **−15** | Aceptar el codepoint del borrador de Kyber (`X25519Kyber768Draft00`). La nota no baja de 0 |

Niveles: **Preparado** ≥ 70 · **En camino** 35–69 · **No preparado** < 35. **Mientras no haya certificados post-cuánticos en la web pública, lo habitual es como mucho 85.** `CERT_PQ_NO_CLASSICAL_FALLBACK` (sin certificado clásico de respaldo) es un hallazgo medio visible en el informe y **no resta puntos en ningún eje**: los del certificado post-cuántico se mantienen. Grover solo se informa.

### Por qué estos pesos

- **Dos ejes en vez de una nota**: con una sola nota, un TLS clásico impecable sacaba una D (2026.2), que mezclaba «inseguro hoy» con «no preparado para lo cuántico». Ahora es **A hoy** y **No preparado**.
- **50 por el híbrido preferido, 30 si solo se acepta**: lo que protege a los usuarios es lo que negocia su navegador.
- **Kyber −15**: lo deja por debajo de no tener PQC (en el laboratorio, `kyber` 10 frente a `modern` 25). Es una migración que empezó y se quedó a medias con un codepoint que los navegadores ya no ofrecen.
- **La renovación automatizada cuenta en los dos ejes**: es higiene del certificado hoy (10) y la agilidad que permitirá pasar a certificados post-cuánticos (15).
- **Forward secrecy parcial suma 0**: una sola suite sin intercambio efímero deja abierto ese camino.

### Renovación automatizada (10 puntos de seguridad y 15 de preparación)

El cambio a certificados post-cuánticos (ML-DSA o híbridos) exigirá reemitir todos los certificados: quien lo tenga automatizado lo hará sin esfuerzo. Se considera automatizado si:

1. la **validez es de 47 días o menos**, o
2. lo emite una **autoridad que funciona con ACME por defecto**: Let's Encrypt, Google Trust Services o ZeroSSL (por organización del emisor).

Por qué no "100 días o menos": el CA/Browser Forum reduce la validez máxima a **200 días (15/03/2026), 100 (15/03/2027) y 47 (15/03/2029)**, así que pronto *todos* los certificados tendrán 100 días o menos y el criterio dejaría de distinguir. 47 días sigue distinguiendo hasta 2029 (hoy solo se llega ahí automatizando), y el emisor ACME reconoce la automatización aunque el certificado dure 90 días. **En 2029 el criterio 1 dejará de distinguir**: habrá que revisarlo (y subir la versión del modelo). Limitación: una CA comercial con ACME pero certificados de un año no puntúa; no se puede saber desde fuera cómo se renueva.

## Escaneos de modelos anteriores

Cada resultado guarda su `scoring_version`, y la nota no se recalcula: un escaneo analizado con 2026.2 se muestra con su modelo, **una sola nota** (y su suelo, si lo tuvo), el aviso **«Analizado con el modelo 2026.2»** y un botón **«Repetir el análisis»**, que lanza un escaneo nuevo del mismo dominio (`outdated_model` en la API). La reutilización de escaneos recientes (la espera de 2 minutos por destino) solo devuelve uno del modelo actual o aún sin analizar.

Modelo **2026.2**, para leer esos informes: intercambio de claves 50 / 30 / 0, TLS 1.3 15, forward secrecy 10, variedad 5 y certificado 20 (5 cada criterio); suelo de 40 si no había tope F; mismas letras y topes.

## Topes por riesgo actual (regla del peor caso)

La letra final es la **peor** entre la de la nota y el tope más severo:

| Tope | Cuándo |
|---|---|
| **F** | Certificado caducado o aún no válido, cadena no válida o autofirmada, nombre que no coincide · *desde 2026.2*: SSLv2, suites NULL o solo de integridad, EXPORT, anónimas o DES simple, DH de menos de 1024 bits |
| **C** | Clave RSA < 2048 (o ECDSA < 256), firma SHA-1/MD5, RC4 o 3DES aceptados, ninguna suite con forward secrecy · *desde 2026.2*: SSLv3, DH de 1024 a 2047 bits |
| **B** | TLS 1.0 o 1.1 activos, suites CBC |

Ejemplo: un sitio con PQC y todo perfecto pero con el certificado caducado tiene 85 de seguridad (pierde los 15 de «válido») y letra **F**. Los topes solo afectan a «Seguridad hoy».

## Reglas de las sondas en crudo (desde 2026.2)

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
| `KEX_PURE_MLKEM`: ML-KEM sin componente clásico | cuántico · informativo | +10 de preparación en **variedad** si hay más de un grupo PQC estándar (`KEX_MULTIPLE_PQC`; +5 en 2026.2) | Estándar (FIPS 203) y preferido por CNSA 2.0, pero ningún navegador lo ofrece: no protege el tráfico real, así que **no** cuenta en el bloque de 50/30 del intercambio de claves. |
| `KEX_EXPERIMENTAL_PQC_ONLY`: solo Kyber pre-estándar o híbridos en borrador | cuántico · medio | 0 puntos de intercambio (como sin PQC); se mantiene `KEX_NO_HYBRID`; desde 2026.3, **−15** de preparación si es el borrador de Kyber | Kyber (`X25519Kyber768Draft00`, 0x6399) es anterior a FIPS 203 e IANA lo marca como obsoleto (RFC 10024); Chrome lo retiró a finales de 2024, así que hoy no protege a nadie. Medio y no crítico porque indica una migración empezada: el remedio es actualizar, no empezar de cero. |
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

Pruebas: `internal/scoring/lab_test.go` puntúa el resultado real de los 10 servidores del laboratorio (con un certificado válido, para juzgar solo la configuración TLS) y casos sintéticos para lo que ningún servidor del laboratorio tiene (solo integridad, AEGIS, Kyber junto a ML-KEM).

## Lo que no puntúa

- **Lo "no evaluado"** (lo que `crypto/tls` no puede comprobar: SSLv3, suites DHE…, ver `docs/scanner.md`) **nunca suma ni resta**: se muestra aparte. Un test lo verifica (`TestNotEvaluatedNeverAffectsTheScore`).
- **Una sonda con error** (timeout, red) no cuenta como soportado ni como no soportado: no da puntos y genera `SCAN_PARTIAL` (y `KEX_HYBRID_UNKNOWN` si afecta al intercambio de claves).
- **Certificados post-cuánticos (hito 2.8.5)**: el escáner pide el certificado ofreciendo firmas ML-DSA (como Chrome) y sin ellas (como Firefox o un cliente antiguo; ver docs/scanner.md). `CERT_PQ_SIGNATURE` (eje cuántico, informativo) dice que el servidor presenta un certificado ML-DSA o SLH-DSA cuando se le ofrece; en 2026.2 era informativo; desde 2026.3 suma 15 de preparación cuántica. En «Seguridad hoy», una clave ML-DSA o SLH-DSA cuenta en el criterio «clave» igual que una ECDSA, ni más ni menos. `CERT_PQ_NO_CLASSICAL_FALLBACK` (eje actual, **gravedad media**, sin tope y sin restar puntos en ningún eje) dice que, sin ML-DSA en la oferta, el servidor rechaza la conexión o vuelve a presentar el certificado post-cuántico: los clientes sin ML-DSA no pueden conectarse. Es media y no alta porque no debilita el cifrado ni la autenticación de quien sí conecta: es un problema de disponibilidad, pero deja fuera a una parte grande de los visitantes (en octubre de 2026, Firefox no ofrece ML-DSA). Si no se puede saber (intensidad básica, error de la sonda), queda como no evaluado (`pq_certificate_fallback`); si no se vio ningún certificado, `pq_certificate`.
- **Informativos sin efecto en la nota**: firma clásica del certificado (`CERT_CLASSICAL_SIGNATURE`; desde el 2.8.5 dice si el servidor la eligió aunque se le ofreció ML-DSA), AES-128 y Grover (`SYM_AES128`: NIST sigue considerando AES-128 adecuado), transparencia de certificados (`CERT_CT`, distingue SCT incrustados y entregados en el handshake).

## Hallazgos

Cada hallazgo tiene `code` estable, `axis` (`quantum`/`current`), `severity` (`critical`, `high`, `medium`, `low`, `info`, `pass`), `category` y `params`. En base de datos solo se guardan código y parámetros; los textos ES/EN (título, explicación, remediación) viven en `internal/catalog/catalog.go` (un paquete aparte de la evaluación, ADR-0002), así que se pueden corregir sin migrar datos. Un test comprueba que todos los códigos tienen texto en ambos idiomas y que ningún marcador queda sin rellenar.

### Referencias por hallazgo

La fuente de cada regla. `TestEveryFindingHasAReference` (`internal/catalog/references_test.go`) comprueba que cada código del catálogo tiene aquí una fila con al menos una fuente (RFC, CVE, borrador del IETF, publicación del NIST/FIPS o votación del CA/Browser Forum), que cada fuente es un enlace a su sitio oficial (rfc-editor.org, nvd.nist.gov, csrc.nist.gov, datatracker.ietf.org o cabforum.org) con su título en «Fuentes», y que no sobra ninguna fila. Quedan fuera tres códigos que no juzgan al servidor, sino que dicen que el escaneo no pudo comprobar algo (no puntúan, ver «Lo que no puntúa»): `SCAN_PARTIAL`, `KEX_HYBRID_UNKNOWN` y `CERT_MISSING`.

| Código | Referencia |
|---|---|
| `KEX_HYBRID_PREFERRED`, `KEX_HYBRID_NOT_PREFERRED`, `KEX_SINGLE_HYBRID`, `KEX_MULTIPLE_HYBRIDS` | [RFC 10024](https://www.rfc-editor.org/rfc/rfc10024) (X25519MLKEM768, SecP256r1MLKEM768, SecP384r1MLKEM1024); [FIPS 203](https://csrc.nist.gov/pubs/fips/203/final) (ML-KEM) |
| `KEX_NO_HYBRID` | [RFC 10024](https://www.rfc-editor.org/rfc/rfc10024); [FIPS 203](https://csrc.nist.gov/pubs/fips/203/final) |
| `KEX_PURE_MLKEM`, `KEX_MULTIPLE_PQC` | [FIPS 203](https://csrc.nist.gov/pubs/fips/203/final); [draft-ietf-tls-mlkem](https://datatracker.ietf.org/doc/draft-ietf-tls-mlkem/) |
| `KEX_EXPERIMENTAL_PQC_ONLY`, `KEX_EXPERIMENTAL_PQC_EXTRA` | [draft-tls-westerbaan-xyber768d00](https://datatracker.ietf.org/doc/draft-tls-westerbaan-xyber768d00/) (borrador caducado, anterior al estándar) |
| `TLS13_SUPPORTED`, `TLS13_MISSING` | [RFC 8446](https://www.rfc-editor.org/rfc/rfc8446); [RFC 10024](https://www.rfc-editor.org/rfc/rfc10024) (los grupos híbridos son de TLS 1.3) |
| `TLS13_SUITES_AEAD` | [RFC 8446](https://www.rfc-editor.org/rfc/rfc8446), sección 9.1 y apéndice B.4 |
| `SUITE_INTEGRITY_ONLY` | [RFC 9150](https://www.rfc-editor.org/rfc/rfc9150) |
| `SUITE_CCM8` | [RFC 6655](https://www.rfc-editor.org/rfc/rfc6655); [RFC 8446](https://www.rfc-editor.org/rfc/rfc8446) |
| `SUITE_MODERN_AEAD` | [NIST SP 800-232](https://csrc.nist.gov/pubs/sp/800/232/final) (Ascon) |
| `SUITE_NULL` | [RFC 5246](https://www.rfc-editor.org/rfc/rfc5246), apéndice C |
| `SUITE_EXPORT` | [CVE-2015-0204](https://nvd.nist.gov/vuln/detail/CVE-2015-0204) (FREAK); [CVE-2015-4000](https://nvd.nist.gov/vuln/detail/CVE-2015-4000) (Logjam) |
| `SUITE_ANON` | [RFC 5246](https://www.rfc-editor.org/rfc/rfc5246), apéndice F.1.1.1 |
| `SUITE_DES` | [RFC 5469](https://www.rfc-editor.org/rfc/rfc5469); [FIPS 46-3](https://csrc.nist.gov/pubs/fips/46-3/final) (retirado) |
| `SUITE_RC4` | [RFC 7465](https://www.rfc-editor.org/rfc/rfc7465) |
| `SUITE_3DES` | [CVE-2016-2183](https://nvd.nist.gov/vuln/detail/CVE-2016-2183) (Sweet32) |
| `SUITE_CBC` | [CVE-2013-0169](https://nvd.nist.gov/vuln/detail/CVE-2013-0169) (Lucky Thirteen); [RFC 7457](https://www.rfc-editor.org/rfc/rfc7457) |
| `SYM_AES128` | [NIST SP 800-57 Part 1 Rev. 5](https://csrc.nist.gov/pubs/sp/800/57/pt1/r5/final) (AES-128 da 128 bits de seguridad) |
| `PROTO_SSL2` | [RFC 6176](https://www.rfc-editor.org/rfc/rfc6176); [CVE-2016-0800](https://nvd.nist.gov/vuln/detail/CVE-2016-0800) (DROWN) |
| `PROTO_SSL3` | [RFC 7568](https://www.rfc-editor.org/rfc/rfc7568); [CVE-2014-3566](https://nvd.nist.gov/vuln/detail/CVE-2014-3566) (POODLE) |
| `PROTO_LEGACY_TLS` | [RFC 8996](https://www.rfc-editor.org/rfc/rfc8996) |
| `FS_ALL`, `FS_PARTIAL`, `FS_NONE` | [RFC 9325](https://www.rfc-editor.org/rfc/rfc9325) (BCP 195) |
| `DH_WEAK` | [CVE-2015-4000](https://nvd.nist.gov/vuln/detail/CVE-2015-4000) (Logjam) |
| `DH_SMALL`, `DH_OK` | [NIST SP 800-131A Rev. 2](https://csrc.nist.gov/pubs/sp/800/131/a/r2/final); [RFC 7919](https://www.rfc-editor.org/rfc/rfc7919) |
| `GROUP_WEAK_CURVES` | [RFC 8422](https://www.rfc-editor.org/rfc/rfc8422); [RFC 9847](https://www.rfc-editor.org/rfc/rfc9847) (marca secp224r1, secp192r1 y las curvas binarias como desaconsejadas, «D») |
| `GROUPS_EXTRA` | [RFC 7919](https://www.rfc-editor.org/rfc/rfc7919) (ffdhe); [RFC 8734](https://www.rfc-editor.org/rfc/rfc8734) (brainpool en TLS 1.3); [RFC 7748](https://www.rfc-editor.org/rfc/rfc7748) (x448) |
| `CERT_VALID`, `CERT_UNTRUSTED`, `CERT_SELF_SIGNED` | [RFC 5280](https://www.rfc-editor.org/rfc/rfc5280), sección 6 (validación de la cadena) |
| `CERT_EXPIRED`, `CERT_NOT_YET_VALID`, `CERT_EXPIRES_SOON` | [RFC 5280](https://www.rfc-editor.org/rfc/rfc5280), sección 4.1.2.5 (validez) |
| `CERT_HOSTNAME_MISMATCH` | [RFC 9525](https://www.rfc-editor.org/rfc/rfc9525) |
| `CERT_WEAK_KEY` | [NIST SP 800-131A Rev. 2](https://csrc.nist.gov/pubs/sp/800/131/a/r2/final) |
| `CERT_WEAK_SIGNATURE` | [RFC 9155](https://www.rfc-editor.org/rfc/rfc9155); [NIST SP 800-131A Rev. 2](https://csrc.nist.gov/pubs/sp/800/131/a/r2/final) |
| `CERT_CLASSICAL_SIGNATURE` | [FIPS 204](https://csrc.nist.gov/pubs/fips/204/final) (ML-DSA); [FIPS 205](https://csrc.nist.gov/pubs/fips/205/final) (SLH-DSA) |
| `CERT_PQ_SIGNATURE` | [FIPS 204](https://csrc.nist.gov/pubs/fips/204/final) (ML-DSA); [FIPS 205](https://csrc.nist.gov/pubs/fips/205/final) (SLH-DSA); [draft-ietf-tls-mldsa](https://datatracker.ietf.org/doc/draft-ietf-tls-mldsa/) (ML-DSA en TLS 1.3); [draft-ietf-lamps-dilithium-certificates](https://datatracker.ietf.org/doc/draft-ietf-lamps-dilithium-certificates/) (ML-DSA en X.509) |
| `CERT_PQ_NO_CLASSICAL_FALLBACK` | [RFC 8446](https://www.rfc-editor.org/rfc/rfc8446) (4.4.2.2: el servidor elige el certificado según `signature_algorithms`); [draft-ietf-tls-mldsa](https://datatracker.ietf.org/doc/draft-ietf-tls-mldsa/) |
| `CERT_CT`, `CERT_CT_MISSING` | [RFC 6962](https://www.rfc-editor.org/rfc/rfc6962) |
| `CERT_AUTOMATED_ACME`, `CERT_NOT_AUTOMATED` | [RFC 8555](https://www.rfc-editor.org/rfc/rfc8555) (ACME) |
| `CERT_AUTOMATED_SHORT` | [CA/Browser Forum, SC081v3](https://cabforum.org/2025/04/11/ballot-sc081v3-introduce-schedule-of-reducing-validity-and-data-reuse-periods/) (validez máxima de 47 días en 2029) |

#### Fuentes

| Fuente | Título oficial | URL |
|---|---|---|
| RFC 5246 | The Transport Layer Security (TLS) Protocol Version 1.2 | https://www.rfc-editor.org/rfc/rfc5246 |
| RFC 5280 | Internet X.509 Public Key Infrastructure Certificate and Certificate Revocation List (CRL) Profile | https://www.rfc-editor.org/rfc/rfc5280 |
| RFC 5469 | DES and IDEA Cipher Suites for Transport Layer Security (TLS) | https://www.rfc-editor.org/rfc/rfc5469 |
| RFC 6176 | Prohibiting Secure Sockets Layer (SSL) Version 2.0 | https://www.rfc-editor.org/rfc/rfc6176 |
| RFC 6655 | AES-CCM Cipher Suites for Transport Layer Security (TLS) | https://www.rfc-editor.org/rfc/rfc6655 |
| RFC 6962 | Certificate Transparency | https://www.rfc-editor.org/rfc/rfc6962 |
| RFC 7457 | Summarizing Known Attacks on Transport Layer Security (TLS) and Datagram TLS (DTLS) | https://www.rfc-editor.org/rfc/rfc7457 |
| RFC 7465 | Prohibiting RC4 Cipher Suites | https://www.rfc-editor.org/rfc/rfc7465 |
| RFC 7568 | Deprecating Secure Sockets Layer Version 3.0 | https://www.rfc-editor.org/rfc/rfc7568 |
| RFC 7748 | Elliptic Curves for Security | https://www.rfc-editor.org/rfc/rfc7748 |
| RFC 7919 | Negotiated Finite Field Diffie-Hellman Ephemeral Parameters for Transport Layer Security (TLS) | https://www.rfc-editor.org/rfc/rfc7919 |
| RFC 8422 | Elliptic Curve Cryptography (ECC) Cipher Suites for Transport Layer Security (TLS) Versions 1.2 and Earlier | https://www.rfc-editor.org/rfc/rfc8422 |
| RFC 8446 | The Transport Layer Security (TLS) Protocol Version 1.3 | https://www.rfc-editor.org/rfc/rfc8446 |
| RFC 8555 | Automatic Certificate Management Environment (ACME) | https://www.rfc-editor.org/rfc/rfc8555 |
| RFC 8734 | Elliptic Curve Cryptography (ECC) Brainpool Curves for Transport Layer Security (TLS) Version 1.3 | https://www.rfc-editor.org/rfc/rfc8734 |
| RFC 8996 | Deprecating TLS 1.0 and TLS 1.1 | https://www.rfc-editor.org/rfc/rfc8996 |
| RFC 9150 | TLS 1.3 Authentication and Integrity-Only Cipher Suites | https://www.rfc-editor.org/rfc/rfc9150 |
| RFC 9155 | Deprecating MD5 and SHA-1 Signature Hashes in TLS 1.2 and DTLS 1.2 | https://www.rfc-editor.org/rfc/rfc9155 |
| RFC 9325 | Recommendations for Secure Use of Transport Layer Security (TLS) and Datagram Transport Layer Security (DTLS) | https://www.rfc-editor.org/rfc/rfc9325 |
| RFC 9525 | Service Identity in TLS | https://www.rfc-editor.org/rfc/rfc9525 |
| RFC 9847 | IANA Registry Updates for TLS and DTLS | https://www.rfc-editor.org/rfc/rfc9847 |
| RFC 10024 | Post-Quantum Traditional (PQ/T) Hybrid Key Agreement Mechanisms for TLS 1.3 | https://www.rfc-editor.org/rfc/rfc10024 |
| draft-ietf-tls-mlkem | ML-KEM Post-Quantum Key Agreement for TLS 1.3 | https://datatracker.ietf.org/doc/draft-ietf-tls-mlkem/ |
| draft-tls-westerbaan-xyber768d00 | X25519Kyber768Draft00 hybrid post-quantum key agreement | https://datatracker.ietf.org/doc/draft-tls-westerbaan-xyber768d00/ |
| draft-ietf-tls-mldsa | Use of ML-DSA in TLS 1.3 | https://datatracker.ietf.org/doc/draft-ietf-tls-mldsa/ |
| draft-ietf-lamps-dilithium-certificates | Internet X.509 Public Key Infrastructure -- Algorithm Identifiers for the Module-Lattice-Based Digital Signature Algorithm (ML-DSA) | https://datatracker.ietf.org/doc/draft-ietf-lamps-dilithium-certificates/ |
| CVE-2013-0169 | Lucky Thirteen: CBC padding timing attack in TLS/DTLS | https://nvd.nist.gov/vuln/detail/CVE-2013-0169 |
| CVE-2014-3566 | POODLE: SSL 3.0 CBC padding oracle | https://nvd.nist.gov/vuln/detail/CVE-2014-3566 |
| CVE-2015-0204 | FREAK: RSA EXPORT key downgrade | https://nvd.nist.gov/vuln/detail/CVE-2015-0204 |
| CVE-2015-4000 | Logjam: DHE EXPORT downgrade and weak Diffie-Hellman groups | https://nvd.nist.gov/vuln/detail/CVE-2015-4000 |
| CVE-2016-0800 | DROWN: decrypting TLS with SSLv2 | https://nvd.nist.gov/vuln/detail/CVE-2016-0800 |
| CVE-2016-2183 | Sweet32: birthday attacks on 64-bit block ciphers (3DES) | https://nvd.nist.gov/vuln/detail/CVE-2016-2183 |
| FIPS 46-3 | Data Encryption Standard (DES) (withdrawn) | https://csrc.nist.gov/pubs/fips/46-3/final |
| FIPS 203 | Module-Lattice-Based Key-Encapsulation Mechanism Standard | https://csrc.nist.gov/pubs/fips/203/final |
| FIPS 204 | Module-Lattice-Based Digital Signature Standard | https://csrc.nist.gov/pubs/fips/204/final |
| FIPS 205 | Stateless Hash-Based Digital Signature Standard | https://csrc.nist.gov/pubs/fips/205/final |
| NIST SP 800-57 Part 1 Rev. 5 | Recommendation for Key Management: Part 1 – General | https://csrc.nist.gov/pubs/sp/800/57/pt1/r5/final |
| NIST SP 800-131A Rev. 2 | Transitioning the Use of Cryptographic Algorithms and Key Lengths | https://csrc.nist.gov/pubs/sp/800/131/a/r2/final |
| NIST SP 800-232 | Ascon-Based Lightweight Cryptography Standards for Constrained Devices: Authenticated Encryption, Hash, and Extendable Output Functions | https://csrc.nist.gov/pubs/sp/800/232/final |
| CA/Browser Forum SC081v3 | Ballot SC081v3: Introduce Schedule of Reducing Validity and Data Reuse Periods | https://cabforum.org/2025/04/11/ballot-sc081v3-introduce-schedule-of-reducing-validity-and-data-reuse-periods/ |

Los CVE no tienen título oficial en el NVD: se indica el nombre con que se conoce el ataque.

## Versionado del modelo

Cada resultado lleva `scoring_version`. **Cambiar pesos, umbrales, topes o las reglas que emiten hallazgos obliga a subir `scoring.Version`** (formato `AAAA.N`: `2026.1`, `2026.2`…), para que una nota guardada se interprete siempre con el modelo con que se calculó. Corregir la redacción de un texto no cambia la versión. La columna para guardarla llega con la tabla `scans` (hito 2.3).

Historial: **2026.1** (13/10/2026, primera versión en producción) · **2026.2** (15/10/2026, hito 2.7.4: hallazgos y topes de las sondas en crudo) · **2026.3** (27/10/2026, hito 2.8.6: dos ejes, «Seguridad hoy» y «Preparación cuántica»; sin suelo; certificado post-cuántico y penalización de Kyber).

---

## English

**Model 2026.3 (milestone 2.8.6)** has two axes. **Security today** (0–100, A–F; same thresholds and caps as before, no floor): TLS 1.3 30, forward secrecy in every active version 25 (partial: 0), certificate 45 (valid 15, key 10 — RSA ≥ 2048, ECDSA ≥ 256, Ed25519, ML-DSA, SLH-DSA —, signature 10, automated renewal 10). **Quantum readiness** (0–100 in three levels: Ready ≥ 70, On track 35–69, Not ready < 35, no letter): hybrid ML-KEM chosen with a browser-like offer 50 (accepted only when offered alone 30), TLS 1.3 10, more than one standard post-quantum group 10, automated renewal 15, a post-quantum certificate 15; accepting the obsolete Kyber draft −15 (not below 0). While there are no post-quantum certificates on the public web, 85 is usually the most a server gets. `CERT_PQ_NO_CLASSICAL_FALLBACK` is a medium finding that takes no points on either axis; Grover (AES-128) is informational. Scans judged with an earlier model keep their judgement and are shown with it, a notice and a "run the analysis again" button. The paragraphs below describe the earlier models, for reading their reports.

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
