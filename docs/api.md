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
- **Espera por destino (2 min)**: si el mismo host tiene un escaneo de menos de 2 minutos que no ha fallado, con la misma intensidad y el mismo propietario, se devuelve ese (**200**, `"reused": true`) y no cuenta para el límite. Dos peticiones simultáneas no crean dos escaneos. Desde el hito 3.1 los escaneos de esta API son **públicos** (sin organización), de intensidad `basic`, y se borran a los 30 días; la intensidad `full` queda para los dominios verificados de una organización (hito 3.4).
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
    "scoring_version": "2026.1", "score": 95, "grade": "A", "score_grade": "A",
    "floor_points": 0, "cap": null, "pqc": true,
    "selected_group": "X25519MLKEM768", "tls_version": "TLS 1.3",
    "quantum_risk": {"code": "low", "label": "Bajo"}, "current_risk": {"code": "none", "label": "Ninguno"},
    "blocks": [{"id": "key_exchange", "label": "…", "points": 50, "max": 50, "items": […]}],
    "findings": [{"code": "…", "axis": "quantum", "axis_label": "…", "severity": "low", "severity_label": "…",
                  "category": "…", "title": "…", "detail": "…", "remediation": "…"}],
    "not_evaluated": [{"code": "ssl2_ssl3", "label": "…"}],
    "result": { … }            // hechos del escáner (docs/scanner.md)
  }
}
```

## Errores

Siempre `{"error": {"code": "…", "message": "…"}}` con el mensaje en el idioma de la petición: `invalid_json` (400), `invalid_host` (400), `port_not_allowed` (400), `body_too_large` (413), `unsupported_media_type` (415), `host_not_allowed` (403), `rate_limited` (429), `not_found` (404), `method_not_allowed` (405), `internal` (500). Códigos de fallo de un escaneo: `blocked_address`, `invalid_target`, `dns_not_found`, `dns_temporary`, `dns`, `connection_refused`, `timeout`, `network`, `unreachable`, `scan_error`, `lease_expired`.

---

## English

JSON only, no CORS. **`GET /api/targets`** lists the allowed hosts. **`POST /api/scans`** `{"host": "…"}` (≤ 1 KiB, `application/json`): the host is normalized (lower case, no scheme/user/path, IDN → punycode; port 443 only, any other explicit port → 400 `port_not_allowed`), must be on `SCAN_ALLOWLIST` (exact match, else 403); a non-failed scan of the same host, intensity and owner younger than 2 minutes is returned (200, `reused`) without counting (from milestone 3.1 these scans are public, intensity basic, deleted after 30 days; full is for verified domains of an organization, milestone 3.4); otherwise 5/min and 30/day per client IP (IPv6 /64; in memory, no IP stored; `X-Forwarded-For` trusted only from the Caddy network) → 429 with `Retry-After`; a new scan → 202 with `Location`. **`GET /api/scans/{id}`** returns the scan, and its `report` when done (score, grade, cap, floor, PQC, risks, blocks, findings with texts in `?lang=` / `Accept-Language` / Spanish, not-evaluated list, raw scanner facts). Errors are `{"error": {"code", "message"}}`.
