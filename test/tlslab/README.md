# Laboratorio TLS / TLS test lab

**ES** · [EN](#english)

Servidores **deliberadamente vulnerables** para validar las sondas de ClientHello en crudo (hito 2.7) y compararlas con testssl.sh. **Nunca se sondea a terceros**: todo ocurre dentro de este laboratorio.

```
make tlslab-up        # construye, arranca y comprueba el aislamiento
make tlslab-testssl   # pasada de referencia de testssl.sh → reference/
make tlslab-down      # lo borra todo y comprueba que no queda nada
make tlslab-test      # tests de integración del escáner (intensidad full y basic) y comparación con testssl.sh
make tlslab-scan ARGS="-facts tls13"   # cmd/qrscan -lab dentro del laboratorio
```

`make tlslab-test` y `tlslab-scan` ejecutan Go en un contenedor (`golang:1.27.4`, fijado por digest) conectado a la red del laboratorio, con el repositorio en solo lectura y la caché de módulos del host (el laboratorio no tiene Internet). Escriben en `results/` (los JSON se ignoran en git; `COMPARISON.md` se guarda). No van en la CI de cada push: tardan unos 10 minutos con la compilación de las imágenes.

## Aislamiento

- **Ningún puerto publicado**: nada escucha en el host (ni en 0.0.0.0, ni en la IP principal del host (la de WSL en local), ni en 127.0.0.1).
- Una sola red Docker **`internal`** (`qr-tlslab`, 172.31.213.0/24): sin salida a Internet ni entrada desde fuera.
- Los clientes (testssl.sh y, desde el 2.7.3, el escáner) se ejecutan en contenedores conectados a esa red (`lab.sh run`).
- `make tlslab-up` lo comprueba cada vez y falla si algo no se cumple: puertos publicados, redes, red interna, sockets nuevos en el host, conexiones a la IP de WSL, a 127.0.0.1 y a 0.0.0.0, y salida a Internet desde el laboratorio.
- `make tlslab-down` deja cero contenedores, redes y volúmenes del laboratorio, y lo comprueba. Las imágenes se conservan como caché (`TLSLAB_RMI=1` también las borra).
- Contenedores con `read_only`, `cap_drop: ALL`, `no-new-privileges`, usuario 65534 y límites de memoria y procesos.

## Procedencia (todo fijado por hash)

| Imagen | Origen |
|---|---|
| `qr-tlslab/openssl102:1.0.2u` | `openssl-1.0.2u.tar.gz`, SHA-256 `ecd0c6ff…f89669d16` (coincide con el publicado por OpenSSL), compilada con `enable-ssl2 enable-ssl3 enable-weak-ssl-ciphers` sobre `debian:bookworm-slim@sha256:7c7b2c96…` |
| `qr-tlslab/openssl35:3.5.9` | `openssl-3.5.9.tar.gz`, SHA-256 `603f5602…f1eb859a` (coincide con el publicado), misma imagen base |
| `qr-tlslab/kyber:go1.23.12` | `golang:1.23.12@sha256:60deed95…`, último Go con el borrador `X25519Kyber768Draft00` |
| testssl.sh | `ghcr.io/testssl/testssl.sh:3.2@sha256:51918b1e…` (la misma de la validación 2.6) |

## Servidores y lo que deben mostrar

| Servidor | Software | Versiones | Lo relevante |
|---|---|---|---|
| `ssl2` | OpenSSL 1.0.2u | solo SSLv2 | SSLv2 (DROWN) con sus cifrados: RC4-MD5, RC2-CBC-MD5, IDEA-CBC-MD5, DES-CBC3-MD5 |
| `ssl3` | OpenSSL 1.0.2u | solo SSLv3 | SSLv3 (POODLE); DHE con grupo de 1024 bits |
| `export` | OpenSSL 1.0.2u | TLS 1.0–1.2 | EXPORT (RSA y DHE), DES simple, NULL y anónimas (ADH, AECDH); DHE de 512 bits |
| `dhe1024` | OpenSSL 1.0.2u | solo TLS 1.2 | Suites DHE (AES-CBC/GCM, Camellia, SEED) con grupo de 1024 bits (Logjam) |
| `tls10` | OpenSSL 1.0.2u | solo TLS 1.0 | Intercambio RSA sin forward secrecy: 3DES, RC4, IDEA, SEED, Camellia |
| `secp224` | OpenSSL 1.0.2u | solo TLS 1.2 | ECDHE solo con la curva secp224r1 |
| `tls13` | OpenSSL 3.5.9 | solo TLS 1.3 | Las 5 suites (CCM y CCM_8 incluidas); grupos X25519MLKEM768, MLKEM768, MLKEM1024, x25519, secp256r1, ffdhe2048, ffdhe3072, brainpoolP256r1tls13 |
| `tls12` | OpenSSL 3.5.9 | solo TLS 1.2 | Camellia, ARIA, CCM, CCM_8; DHE de 2048 bits; ECDHE con brainpoolP256r1 y secp256r1 |
| `modern` | OpenSSL 3.5.9 | TLS 1.2 y 1.3 | El caso típico de hoy: bien configurado y sin PQC. Solo ECDHE con AEAD (AES-GCM, ChaCha20-Poly1305) y las 3 suites AEAD de TLS 1.3; grupos x25519 y secp256r1, ningún grupo post-cuántico ni híbrido |
| `kyber` | Go 1.23.12 | solo TLS 1.3 | PQC solo en borrador (`X25519Kyber768Draft00`, 0x6399) más x25519; sin ML-KEM estándar |
| `mldsa-dual` | OpenSSL 3.5.9 | TLS 1.2 y 1.3 | Certificado ML-DSA-65 y certificado ECDSA P-256: presenta uno u otro según las firmas que ofrezca el cliente (hito 2.8.5) |
| `mldsa-only` | OpenSSL 3.5.9 | solo TLS 1.3 | Solo certificado ML-DSA-65, sin respaldo clásico: rechaza a los clientes que no ofrecen ML-DSA (hito 2.8.5) |

`reference/` guarda la salida de testssl.sh (JSON y texto) de cada servidor: es la referencia para la comparación del hito 2.7.3.

---

## English

**Deliberately vulnerable** servers to validate the raw ClientHello probes (milestone 2.7) against testssl.sh. **Never probe third parties.** `make tlslab-up` builds, starts and checks isolation; `make tlslab-testssl` writes the testssl.sh reference to `reference/`; `make tlslab-down` removes everything and checks that no lab container, network or volume is left.

**Isolation**: no published ports (nothing listens on the host: not 0.0.0.0, the WSL address or 127.0.0.1); a single **internal** Docker network (no Internet either way); clients run as containers on that network. Every start checks it and fails otherwise. Containers are read-only, drop all capabilities, run as uid 65534 with memory and process limits.

**Provenance** (all pinned by hash): OpenSSL 1.0.2u and 3.5.9 built from tarballs verified against OpenSSL's published SHA-256, on a digest-pinned Debian base; Go 1.23.12 (last release with the Kyber draft) by digest; testssl.sh 3.2 by digest. The table above lists what each server must show.
