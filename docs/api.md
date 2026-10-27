# API

**ES** · [EN](#english)

Todas las respuestas son JSON (`Cache-Control: no-store`, `X-Content-Type-Options: nosniff`, `Content-Security-Policy: default-src 'none'; frame-ancestors 'none'`). No hay CORS: la API solo se usa desde el propio sitio.

## `GET /api/targets`

Los dominios que se pueden escanear (la allowlist), para que el frontend no tenga su propia copia: `{"hosts": ["fizsar.com", "quantumready.fizsar.com"], "port": 443}`.

## `POST /api/scans`

Pide un escaneo. Cuerpo (máx. 1 KiB, `Content-Type: application/json`, campos desconocidos rechazados):

```json
{"host": "https://FizSar.com/ruta"}
```

- **Normalización**: minúsculas, sin esquema, usuario, ruta, consulta ni punto final; IDN a punycode (`bücher.de` → `xn--bcher-kva.de`). Se escanea el puerto **443**: `:443` explícito se acepta; cualquier otro puerto → **400** `port_not_allowed` ("por ahora solo se escanea el puerto 443").
- **Allowlist** (`SCAN_ALLOWLIST`, coincidencia exacta): en esta fase, `fizsar.com` y `quantumready.fizsar.com`. Otro host → **403** `host_not_allowed`.
- **Espera por destino (2 min)**: si el mismo host tiene un escaneo de menos de 2 minutos que no ha fallado, se devuelve ese (**200**, `"reused": true`) y no cuenta para el límite. Dos peticiones simultáneas no crean dos escaneos.
- **Límite por cliente** (en memoria, sin guardar IP): 5 escaneos por minuto y 30 al día por IP (IPv6 por /64) → **429** `rate_limited` con `Retry-After`. `X-Forwarded-For` solo se cree si la conexión viene de la red de Caddy (`TRUSTED_PROXIES`).
- Nuevo escaneo → **202** con `Location: /api/scans/{id}` y el escaneo en estado `queued`.

## `GET /api/scans/{id}`

`id` es un UUID (otro valor → 404). Idioma de los textos: `?lang=es|en`, si no `Accept-Language`, si no español (`Content-Language` lo indica).

```jsonc
{
  "id": "…", "host": "fizsar.com", "port": 443,
  "status": "done",            // queued | running | done | failed
  "retries": 0,                // reintentos tras un error transitorio
  "created_at": "…", "started_at": "…", "finished_at": "…",
  "error": {"code": "connection_refused", "message": "…"},   // si failed (o esperando reintento)
  "lang": "es",
  "report": {                  // solo si done
    "scoring_version": "2026.3", "score": 100, "grade": "A", "score_grade": "A",   // seguridad hoy
    "cap": null, "pqc": true,
    "selected_group": "X25519MLKEM768", "tls_version": "TLS 1.3",
    "readiness": {"score": 75, "level": {"code": "ready", "label": "Preparado"},   // preparación cuántica (2026.3)
                  "blocks": [{"id": "key_exchange", "label": "…", "points": 50, "max": 50}, …]},
    "quantum_risk": {"code": "low", "label": "Bajo"}, "current_risk": {"code": "none", "label": "Ninguno"},
    "blocks": [{"id": "tls13", "label": "TLS 1.3", "points": 30, "max": 30, "items": […]}, …],
    "findings": [{"code": "…", "axis": "quantum", "axis_label": "…", "threat": "shor", "severity": "low", "severity_label": "…",
                  "category": "…", "title": "…", "detail": "…", "remediation": "…"}],
    "not_evaluated": [{"code": "ssl2_ssl3", "label": "…"}],
    "result": { … }            // hechos del escáner (docs/scanner.md)
  }
}
```

Desde el modelo **2026.3** el informe tiene dos ejes: `score`, `grade`, `cap` y `blocks` son «Seguridad hoy», y `readiness` es «Preparación cuántica» (nivel `ready`, `on_track` o `not_ready`; un bloque puede tener puntos negativos: `kyber_draft`, −15). `threat` marca, en los hallazgos del eje cuántico, si son de Shor o de Grover. Un escaneo analizado con un modelo anterior conserva su juicio: sin `readiness`, con `floor_points` si lo tuvo (2026.2) y con `"outdated_model": true`; la web ofrece repetir el análisis. `POST /api/scans` solo reutiliza un escaneo reciente del modelo actual (o aún sin analizar).

## `POST /api/e` (métricas de uso, M1)

La web cuenta visitas y algunos usos (`web/src/metrics.ts`, con `sendBeacon`). Responde **siempre 204**, se cuente o no, para que probarlo no enseñe nada. Solo cuenta si la petición viene de nuestras propias páginas (`Sec-Fetch-Site: same-origin`, o `Origin` igual al host) y de un navegador (fuera robots conocidos y peticiones sin User-Agent), con un límite por cliente en memoria (sin guardar IP). Cuerpo de máx. 256 bytes con campos desconocidos rechazados y listas cerradas: `page_view` (`p`: `/`, `/examples`, `/architecture`, `/privacy`, `scan_report` o `not_found`; la primera vista de una carga lleva `entry` y, si se llega desde otro sitio, `r` con su host, que se reduce a su dominio registrable), `scan_started` (`d`: un dominio de la allowlist), `under_the_hood_open` y `json_download` (`d`: `scan_report` o `examples`) y `lang_switch` (`d`: `es` o `en`). Cualquier otra cosa se descarta. `scan_blocked_403` no se acepta desde la web: lo cuenta la API al responder 403 `host_not_allowed`, junto con el dominio registrable pedido (tabla `blocked_targets_daily`, purgada a los 30 días y fuera de las copias de seguridad). Todo son contadores diarios (`usage_daily`), con un máximo de valores distintos por evento y día (lo que pase va a `other`). Se consultan con `qradmin stats`.

## `POST /api/demo` (solicitar demo gratis)

Solo con `DEMO_ENABLED=true` (por defecto false: el proyecto es de portfolio). Apagada, `POST /api/demo` y `GET /api/demo/challenge` responden **404**, `GET /api/targets` devuelve `"demo": false` (la web oculta /demo y sus botones) y no se cuentan ni `demo_requested` ni las visitas a `/demo`; la purga de las solicitudes guardadas y `qradmin demos` siguen funcionando.

Cuerpo JSON (máx. 8 KiB, campos desconocidos rechazados): `name` y `company` (1–100 caracteres, una línea), `email` (una dirección sin nombre, máx. 254), `domain` (se normaliza con la Public Suffix List; ni IPs ni nombres reservados), `message` (opcional, máx. 2000 caracteres), `privacy` (debe ser `true`), `lang` (`es`/`en`), `website` (campo trampa: vacío) y `altcha` (prueba de trabajo, solo si la pide el servidor). Solo desde el propio sitio (`Sec-Fetch-Site: same-origin` u `Origin` igual al host; si no, **403** `bad_origin`) y con `Content-Type: application/json` (**415**). Límite por cliente en memoria, sin guardar la IP: 3/min y 10/día → **429** `too_many_requests` con `Retry-After`. Con el campo trampa relleno responde **202** como siempre y no guarda nada. Errores de validación (**400**): `privacy_required`, `invalid_name`, `invalid_company`, `invalid_email`, `invalid_domain`, `invalid_message`, `invalid_proof`. Bien → **202** `{"status":"received"}`: se guarda en `demo_requests` con la versión de /privacy aceptada; se borra a los 12 meses (purga diaria) y sí entra en las copias de seguridad. **`GET /api/demo/challenge`**: el reto ALTCHA (solo si `ALTCHA_HMAC_KEY` está definida; si no, 404 y no se pide prueba). Se consultan con `qradmin demos list|show|delete`.

## Errores

Siempre `{"error": {"code": "…", "message": "…"}}` con el mensaje en el idioma de la petición: `invalid_json` (400), `invalid_host` (400), `port_not_allowed` (400), `body_too_large` (413), `unsupported_media_type` (415), `host_not_allowed` (403), `rate_limited` (429), `not_found` (404), `method_not_allowed` (405), `internal` (500). Códigos de fallo de un escaneo: `blocked_address`, `invalid_target`, `dns_not_found`, `dns_temporary`, `dns`, `connection_refused`, `timeout`, `network`, `unreachable`, `scan_error`, `lease_expired`.

---

## English

JSON only, no CORS. **`GET /api/targets`** lists the allowed hosts. **`POST /api/scans`** `{"host": "…"}` (≤ 1 KiB, `application/json`): the host is normalized (lower case, no scheme/user/path, IDN → punycode; port 443 only, any other explicit port → 400 `port_not_allowed`), must be on `SCAN_ALLOWLIST` (exact match, else 403); a non-failed scan of the same host younger than 2 minutes is returned (200, `reused`) without counting; otherwise 5/min and 30/day per client IP (IPv6 /64; in memory, no IP stored; `X-Forwarded-For` trusted only from the Caddy network) → 429 with `Retry-After`; a new scan → 202 with `Location`. **`GET /api/scans/{id}`** returns the scan, and its `report` when done (score, grade, cap, floor, PQC, risks, blocks, findings with texts in `?lang=` / `Accept-Language` / Spanish, not-evaluated list, raw scanner facts). **`POST /api/e`** (M1 usage metrics) always answers 204; it counts only same-origin requests from browsers (known bots and empty user agents left out), within a per-client in-memory limit, with closed lists of events and values (`page_view` with its route, the referring site's registrable domain on entry, `scan_started` of an allowlisted host, `under_the_hood_open`, `json_download`, `lang_switch`); everything else is dropped. Refused scans (403) are counted by the API itself, with the registrable domain asked for, kept 30 days and left out of backups. Daily counters only; read them with `qradmin stats`. **`POST /api/demo`** (only with `DEMO_ENABLED=true`, default false: otherwise 404, `GET /api/targets` says `"demo": false` and the web hides it) stores a "Request a free demo" (name, company, email, domain, optional message, `privacy: true`; same-origin JSON only, 3/min and 10/day per client in memory, a `website` honeypot that is answered 202 and dropped, and an ALTCHA proof of work from `GET /api/demo/challenge` when `ALTCHA_HMAC_KEY` is set); kept 12 months, in the backups; read with `qradmin demos`. Errors are `{"error": {"code", "message"}}`.
