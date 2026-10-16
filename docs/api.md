# API

**ES** · [EN](#english)

Todas las respuestas son JSON (`Cache-Control: no-store`, `X-Content-Type-Options: nosniff`, `Content-Security-Policy: default-src 'none'; frame-ancestors 'none'`). No hay CORS: la API solo se usa desde el propio sitio. Con `PUBLIC_ORIGIN` configurado, toda petición que cambia algo (POST…) y que un navegador marca como de otro sitio (`Origin` distinto o `Sec-Fetch-Site` que no sea `same-origin`/`none`) → **403** `bad_origin`.

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

## Cuentas: `/api/auth/*` (hito 3.2)

Solo existen si la API tiene `PUBLIC_ORIGIN` (si no, 404). Cuerpos JSON de hasta 4 KiB, campos desconocidos rechazados. La sesión va en la cookie `__Host-qr_session` (`Secure`, `HttpOnly`, `SameSite=Lax`, `Path=/`): 30 días como máximo y se cierra tras 7 días sin uso. Las peticiones que cambian algo con sesión llevan además la cabecera `X-QR-CSRF` con el `csrf_token` de `/me` (si no, **403** `csrf_failed`). Detalles de seguridad en [security.md](security.md#cuentas).

| Endpoint | Cuerpo | Respuesta |
|---|---|---|
| `POST /register` | `email`, `password`, `signup_code`, opcionales `name`, `organization`, `lang` | **202** `{"status":"check_email"}` y email de confirmación (24 h). Si el email ya tiene cuenta, la **misma** respuesta y a su dueño le llega un aviso. Crea la organización del usuario (owner). 400: `invalid_email`, `invalid_name`, `invalid_organization`, `invalid_signup_code`, `password_too_short` (< 12), `password_too_long` (> 128), `password_common`, `password_contains_email` |
| `POST /login` | `email`, `password` | **200** con la vista de `/me` y la cookie. **401** `invalid_credentials` (email desconocido o contraseña mal: idénticos), **403** `email_not_verified` (solo con la contraseña correcta), **429** `too_many_attempts` |
| `GET /me` | — | **200** `{"user": {id, email, name, lang}, "organizations": [{id, name, role}], "active_org_id", "csrf_token"}`; **401** `unauthenticated` |
| `POST /logout` | — (CSRF) | **204**, borra la sesión y la cookie |
| `POST /verify-email` | `token` | **200** `{"status":"verified"}`; **400** `invalid_token` |
| `POST /resend-verification` | `email` | Siempre **202** (si la cuenta existe y no está confirmada, nuevo enlace) |
| `POST /forgot-password` | `email` | Siempre **202** (si la cuenta existe, enlace de 30 min) |
| `POST /reset-password` | `token`, `password` | **200**; cierra todas las sesiones y marca el email como confirmado. **400** `invalid_token` o de contraseña (el enlace sigue valiendo) |
| `POST /change-password` | `current_password`, `new_password` (CSRF) | **204**; cierra las demás sesiones. **400** `wrong_password` o de contraseña |

Los enlaces de los emails son `PUBLIC_ORIGIN/verify#token=…` y `/reset#token=…`: el token va en el fragmento, que el navegador no envía nunca al servidor, y la web lo envía por POST.

**Límites** (en memoria): login 10/min y 50/día por IP; 5 contraseñas fallidas por cuenta en 15 min (también para emails que no existen, para no delatarlos); registro, verificación, reenvío, recuperación y reset 5/min y 30/día por IP → **429** `too_many_attempts` con `Retry-After`. Además, como mucho 3 emails por dirección y hora: los que sobran no se envían, sin avisar.

## Organización: `/api/org/*` (hito 3.3)

Con sesión (y CSRF en lo que cambia algo). Actúan siempre sobre la **organización activa** de la sesión; los IDs de la URL son de miembros dentro de ella. Permisos por rol en [security.md](security.md#roles-en-una-organización): sin permiso → **403** `forbidden`; un ID que no es de un miembro → **404**; dejarla sin owner → **409** `last_owner`; el usuario sin ninguna organización → **403** `no_organization`.

| Endpoint | Cuerpo | Respuesta |
|---|---|---|
| `GET /org` | — | `{"id", "name", "role", "permissions": ["view", …]}` |
| `PATCH /org` | `name` | **204** (owner, admin) |
| `GET /org/members` | — | `{"members": [{"user_id", "email", "name", "role", "joined_at", "you"}]}` |
| `PATCH /org/members/{userID}` | `role` | **204** (owner) |
| `DELETE /org/members/{userID}` | — | **204** (owner: cualquiera; admin: viewers; uno mismo: salir) |
| `POST /org/leave` | — | **204**; la sesión pasa a otra organización del usuario |
| `POST /org/invitations` | `email`, `role` (`admin`/`viewer`), opcional `lang` | **201** `{id, email, role, created_at, expires_at}` y email con `PUBLIC_ORIGIN/invite#token=…` (7 días, un solo uso). Sustituye a la pendiente para la misma dirección. **409** `already_member`, `limit_reached` (20 pendientes) |
| `GET /org/invitations` | — | `{"invitations": [{id, email, role, inviter_email, created_at, expires_at, expired}]}` (owner, admin) |
| `DELETE /org/invitations/{id}` | — | **204**; **404** si no está pendiente |

| `GET /org/domains` | — | `{"domains": [{id, name, status, verified, verified_at, last_checked_at, lost_at, grace_until, check_pending, last_check_error, record: {type: "TXT", name: "_quantumready.<dominio>", value: "qr-verify=<token>"}}]}` |
| `POST /org/domains` | `name` | **201** (owner, admin). Se normaliza (minúsculas, punycode). **400**: `invalid_domain`, `domain_ip`, `domain_reserved` (`.local`, `.test`, `.internal`, `arpa`…), `domain_unknown_tld`, `domain_public_suffix` (según la Public Suffix List incrustada). **409** `domain_exists`, `limit_reached` (20) |
| `GET /org/domains/{id}` | — | El dominio |
| `DELETE /org/domains/{id}` | — | **204** (owner, admin); sus escaneos siguen en el historial |
| `POST /org/domains/{id}/verify` | — | **202** con `check_pending: true` (owner, admin; 10 por hora y dominio). El worker consulta el TXT en segundos: consultar `GET` hasta que `check_pending` sea `false` |

| `POST /org/scans` | `host` | Escaneo **`full`** de un dominio verificado de la organización o de un subdominio suyo (owner, admin). **202** (o **200** reutilizado, como `/api/scans`), `Location: /api/org/scans/{id}`. **403** `domain_not_verified`; **429** (30 al día por organización; los reutilizados no cuentan). La allowlist de escaneos públicos no se aplica: la verificación prueba el control |
| `GET /org/scans` | — | Historial (13 meses, los 200 últimos): `{"scans": [{id, host, status, intensity, created_at, finished_at, score, grade}]}` |
| `GET /org/scans/{id}` | — | El escaneo, como `GET /api/scans/{id}` (con `intensity`). Solo los de la organización; un escaneo público → **404** |

`status`: `pending` (nunca verificado o verificación perdida), `verified`, `lost` (el registro desapareció; los escaneos `full` siguen hasta `grace_until`, 14 días). `verified` indica si ahora se permiten escaneos `full`.

Fuera de `/org`:

| Endpoint | Cuerpo | Respuesta |
|---|---|---|
| `POST /invitations/preview` | `token` | Sin sesión. `{organization, email, role, inviter_email, expires_at}`; **400** `invalid_token` (usada, revocada, caducada o desconocida) |
| `POST /invitations/accept` | `token` (sesión, CSRF) | **200** como `GET /org`; pasa a ser la organización activa. **403** `invitation_other_email` si la sesión no es de la dirección invitada |
| `POST /auth/register` | `invitation_token` en vez de `signup_code` | **201** `{"status":"registered"}`: cuenta ya confirmada (el enlace llegó a esa dirección), sin organización propia. Si la dirección ya tiene cuenta, **202** como siempre y la invitación no se gasta |
| `POST /organizations` | `name` (sesión, CSRF) | **201**: organización nueva, el usuario es owner y pasa a ser la activa. **409** `limit_reached` (10 organizaciones por usuario) |
| `POST /organizations/{orgID}/switch` | — (sesión, CSRF) | **200** como `GET /org`; **404** si no es miembro |

## Errores

Siempre `{"error": {"code": "…", "message": "…"}}` con el mensaje en el idioma de la petición: `invalid_json` (400), `invalid_host` (400), `port_not_allowed` (400), `body_too_large` (413), `unsupported_media_type` (415), `host_not_allowed` (403), `bad_origin` (403), `rate_limited` (429), `not_found` (404), `method_not_allowed` (405), `internal` (500); de cuentas, los de la tabla anterior. Códigos de fallo de un escaneo: `blocked_address`, `invalid_target`, `dns_not_found`, `dns_temporary`, `dns`, `connection_refused`, `timeout`, `network`, `unreachable`, `scan_error`, `lease_expired`.

---

## English

JSON only, no CORS. **`GET /api/targets`** lists the allowed hosts. **`POST /api/scans`** `{"host": "…"}` (≤ 1 KiB, `application/json`): the host is normalized (lower case, no scheme/user/path, IDN → punycode; port 443 only, any other explicit port → 400 `port_not_allowed`), must be on `SCAN_ALLOWLIST` (exact match, else 403); a non-failed scan of the same host, intensity and owner younger than 2 minutes is returned (200, `reused`) without counting (from milestone 3.1 these scans are public, intensity basic, deleted after 30 days; full is for verified domains of an organization, milestone 3.4); otherwise 5/min and 30/day per client IP (IPv6 /64; in memory, no IP stored; `X-Forwarded-For` trusted only from the Caddy network) → 429 with `Retry-After`; a new scan → 202 with `Location`. **`GET /api/scans/{id}`** returns the scan, and its `report` when done (score, grade, cap, floor, PQC, risks, blocks, findings with texts in `?lang=` / `Accept-Language` / Spanish, not-evaluated list, raw scanner facts). Errors are `{"error": {"code", "message"}}`. With `PUBLIC_ORIGIN` set, state-changing requests a browser marks as cross-site (other `Origin`, `Sec-Fetch-Site` not `same-origin`/`none`) get 403 `bad_origin`.

**Accounts (`/api/auth/*`, milestone 3.2; only with `PUBLIC_ORIGIN`)**: `register` (email, password, signup code; always 202 — a taken email gets the same answer and its owner an email), `login` (401 `invalid_credentials` for unknown email and wrong password alike; 403 `email_not_verified` only with the right password), `me`, `logout`, `verify-email`, `resend-verification` and `forgot-password` (always 202), `reset-password` (ends every session), `change-password` (ends the other sessions). Session cookie `__Host-qr_session` (Secure, HttpOnly, SameSite=Lax; 30 days, 7 idle); state-changing calls with a session need the `X-QR-CSRF` header. Email links carry the token in the URL fragment. Limits: login 10/min and 50/day per IP, 5 failed passwords per account in 15 min, email-sending endpoints 5/min and 30/day per IP (429 with `Retry-After`), and at most 3 emails per address per hour (silently). See the table above and [security.md](security.md#english).
