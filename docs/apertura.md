# Escaneo público: abrir, cerrar y vigilar

## Abrir o cerrar en segundos

`deploy/scan-switch.sh` cambia `PUBLIC_SCAN_OPEN` en el `.env` del servidor y recrea **solo la API**, con la misma imagen: sin pull y sin tocar la base de datos, el worker ni la web.

```bash
deploy/scan-switch.sh close   # PUBLIC_SCAN_OPEN=false: vuelve a la allowlist (SCAN_ALLOWLIST)
deploy/scan-switch.sh open    # PUBLIC_SCAN_OPEN=true: cualquier dominio (exige ALTCHA_HMAC_KEY)
QR_SWITCH_DRY_RUN=1 deploy/scan-switch.sh close   # solo el plan; no cambia nada
```

Qué hace, en orden:
1. Escribe la línea `PUBLIC_SCAN_OPEN` en el `.env`, después de copiarlo en `.env.switch` (mismos permisos).
2. Para abrir, exige `ALTCHA_HMAC_KEY` con al menos 32 dígitos hexadecimales; sin ella se niega y no cambia nada.
3. Calcula el plan de compose con `recreate_plan` (el mismo de `deploy.sh`). Si no es exactamente `api recreate`, porque otra cosa del `.env` o del compose difiere de lo que corre, devuelve el `.env` a como estaba y se para.
4. `docker compose up -d --no-deps --pull never --wait api`. Si la API no queda sana, devuelve el `.env`, recrea la API como estaba y termina con error.
5. Comprueba que `GET /api/targets` dice `"open": true|false`, en el servidor y desde fuera.

Probado con `deploy/test-scan-switch.sh` (en `make test-deploy`, también en la CI) sobre un proyecto compose desechable: abrir y cerrar sin tocar la base de datos, un servicio parado sigue parado, repetir no recrea nada, el modo de prueba no cambia nada, se niega sin `ALTCHA_HMAC_KEY` y con un plan que incluye más que la API, y vuelve atrás si la API no queda sana. Con la pila de producción real, abrir y cerrar tardan unos 7 s.

Cuándo cerrar: abuso evidente (rechazos por el límite global sostenidos, la cola llena una y otra vez), una queja de un tercero que no se resuelve con una exclusión, o cualquier fallo del escáner contra dominios ajenos. Cerrar no borra nada: los escaneos hechos siguen visibles por su enlace hasta que caducan (30 días).

## Vigilar: `qradmin public-report`

```bash
docker compose run --rm --entrypoint /qradmin migrate public-report -days 2
```

Por día (UTC), desde `-days N` días atrás (1–31; los escaneos públicos se guardan 30 días):

| Columna | Qué es | De dónde sale |
|---|---|---|
| `scans`, `done`, `failed`, `full` | escaneos públicos creados ese día, cómo acabaron y cuántos fueron completos (nuestros dominios) | `scans` |
| `domains` | dominios registrables distintos (`www.shop.example.co.uk` cuenta como `example.co.uk`) | `scans.host` y la Public Suffix List |
| `queue_timeouts` | escaneos que esperaron en la cola más de `QR_PUBLIC_QUEUE_TIMEOUT` | `scans.error = 'queue_timeout'` |
| `max_queue` | lo máximo que hubo esperando a la vez, reconstruido con la hora de creación y la del último intento (sobreestima con reintentos) | `scans.created_at`, `started_at`, `finished_at` |
| `exclusions_added` | dominios añadidos ese día a la lista de exclusión | `scan_exclusions` |

Después, los rechazos por día y tipo, los dominios más escaneados del periodo (solo los que llegan a `-min K` escaneos, 3 por defecto) y el número de exclusiones vigentes. Solo lee: la tabla `scans` no guarda nada de quién pidió un escaneo público (ni IP, ni cuenta, ni cliente). Tests con base de datos y mutaciones en `cmd/qradmin/report_test.go`.

### Rechazos (migración 00022)

| Columna | Respuesta (sin cambios) |
|---|---|
| `client_limit` | 429 `rate_limited`: el límite por cliente |
| `domain_limit` | 429 `domain_rate_limited`: el límite por dominio |
| `global_limit` | 429 `busy`: el límite global |
| `queue_full` | 429 `busy`: la cola pública está llena |
| `excluded` | 403 `scan_not_allowed`: dominio excluido u opt-out |
| `proof_failed` | 400 `proof_failed`: sin prueba de trabajo válida |
| `closed_403` | 403 `host_not_allowed` con el escáner cerrado: el contador de M1 (`scan_blocked_403`), que no cuenta bots |

- **Qué se guarda.** La tabla `scan_refusals_daily` solo tiene día, tipo (lista cerrada, con `CHECK`) y número: por sus columnas no puede guardar una IP, un dominio ni nada de quién pidió (`TestScanRefusals` fija esas columnas).
- **Permisos.** `qr_app` no tiene ningún permiso sobre la tabla: solo `EXECUTE` de `add_scan_refusals(tipo, n)` (`SECURITY DEFINER`), que suma al día de hoy según el reloj de la base de datos. No puede leer, cambiar ni borrar, ni elegir el día.
- **Escritura.** La API cuenta en memoria y escribe cada 30 s, y una vez más al apagarse: un aluvión de peticiones rechazadas no se convierte en una escritura por petición. Si una escritura falla, los recuentos se guardan para la siguiente.
- **Qué se cuenta.** Todo, bots incluidos. La respuesta al cliente no cambia: mismo código y mismo mensaje.
- **Retención:** sin purga, como `usage_daily` (M1): números agregados por día, sin nada personal.
