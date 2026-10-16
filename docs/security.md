# Seguridad: aislamiento y cuentas / Security: isolation and accounts

**ES** · [EN](#english)

Desde la Fase 3 los escaneos pertenecen a una organización (o a nadie: escaneos públicos). Que una organización no pueda ver ni tocar los datos de otra se garantiza con **dos barreras independientes**: si una falla, la otra sigue protegiendo.

## 1. Aplicación: cada consulta lleva la organización

- La organización sale **siempre de la sesión** (membresía del usuario), nunca de un parámetro de la petición.
- Todas las consultas de sqlc sobre datos de una organización filtran por `organization_id` (`internal/store/queries/tenancy.sql`).
- Un recurso de otra organización responde **404**, no 403: no se revela que existe.
- La reutilización de escaneos busca por host, puerto, **intensidad** y **organización**: un escaneo `basic` nunca responde a una petición `full`, y una organización nunca recibe el de otra.

## 2. PostgreSQL: seguridad a nivel de fila (RLS)

- Las peticiones de la API se ejecutan dentro de `store.Store.Tx`: una transacción que hace `SET LOCAL ROLE qr_app` y fija `app.org_id` y `app.user_id` con `set_config(..., true)`. Ambos son locales a la transacción: una conexión del pool nunca arrastra el inquilino a la siguiente petición (hay un test).
- Las tablas con `organization_id` (y `organizations` y `findings`) tienen **RLS activado y forzado** (`FORCE ROW LEVEL SECURITY`) con políticas para `qr_app`: aunque una consulta olvide el filtro, solo ve y modifica filas de su organización (los escaneos públicos se pueden leer, como su URL).
- `memberships` y `organizations` tienen políticas por operación: un usuario **ve** sus membresías en todas sus organizaciones (para el selector), pero **solo modifica** las de la organización activa.
- Roles (`migrations/00004_accounts.sql`):

| Rol | Uso | RLS |
|---|---|---|
| `qr_app` | API | Sí. Sin permiso para modificar ni borrar escaneos |
| `qr_worker` | Worker: la cola de escaneos (`scans`: leer y actualizar), guardar hallazgos (`findings`: insertar) y verificar dominios (`domains`: **solo** leer `id, name, token, status, verified_at, last_checked_at, lost_at, check_requested_at, check_lease_until` y actualizar las columnas de verificación) | Sí, sin `BYPASSRLS`: políticas explícitas en esas tablas para todas las organizaciones. **Sin acceso** a `users`, `sessions`, `auth_tokens`, organizaciones, a qué organización pertenece un dominio (`organization_id`), a crear, renombrar o borrar dominios, auditoría ni emails |
| `qr_mailer` | Mailer (`cmd/mailer`, proceso aparte): enviar la cola de emails | Solo `email_outbox` (leer, actualizar, purgar). Cada email llega a la cola ya resuelto: destinatario, asunto y cuerpo; el mailer no lee nada más |
| propietario (`POSTGRES_USER`) | Migraciones | No aplica |

Cada servicio actúa con su rol desde que abre la conexión (`store.UseRole`: `SET ROLE` en cada conexión del pool): la API como `qr_app`, el worker como `qr_worker`, el mailer como `qr_mailer`. Hasta el hito 3.7 se conectan con el usuario propietario y cambian de rol al conectar; en el 3.7 pasarán a conectarse con usuarios propios con contraseñas generadas en el VPS, de modo que ningún servicio tenga privilegios de propietario ni pueda volver a ellos.

**Por qué el envío de emails es un proceso aparte**: el binario del worker relaja `crypto/tls` a propósito para poder analizar servidores antiguos (`GODEBUG tlssha1=1`, `rsa1024min=0`); esa relajación no debe afectar a la conexión SMTP con el proveedor. Además, un worker comprometido no puede enviar correo ni leer direcciones.

## Tests

- **Cobertura de RLS** (`TestRLSCoversEveryOrganizationTable`): recorre el catálogo de PostgreSQL y falla si una tabla con `organization_id` no tiene RLS activado, forzado y con políticas. Una tabla nueva que lo olvide rompe el test.
- **Aislamiento** (`TestTenantIsolation`): dos organizaciones con usuarios, dominios y escaneos:
  - las consultas de la aplicación no cruzan organizaciones;
  - una consulta **sin filtro** solo ve el inquilino (RLS);
  - insertar, modificar o borrar en otra organización se rechaza o no afecta a ninguna fila (incluido ascenderse a owner en otra organización);
  - el inquilino público solo ve escaneos públicos;
  - el inquilino no se filtra entre transacciones del pool.
- **Prueba de mutación** (15/10/2026): quitando `SET LOCAL ROLE qr_app`, `TestTenantIsolation` falla en 13 comprobaciones (B ve los dominios, escaneos y hallazgos de A, borra su dominio, se asciende…); con RLS, todas pasan. Las barreras dependen de verdad de RLS.
- **Mínimo privilegio** (`TestBackgroundRolesLeastPrivilege`): `qr_worker` no tiene `BYPASSRLS` y recibe *permission denied* al leer `users`, `sessions`, `auth_tokens`, organizaciones, membresías, invitaciones, dominios, auditoría, la cola de emails y la lista de borrados; puede reclamar y completar escaneos de cualquier organización, pero no crear ni borrar escaneos ni leer hallazgos. `qr_mailer` solo puede leer `email_outbox`. Además, los tests del worker y la prueba de extremo a extremo de la API ejecutan el worker **con su rol de producción**, así que cualquier consulta que necesite un permiso de más falla en los tests (así apareció que `COPY` no funciona bajo RLS: los hallazgos se insertan ahora con un único `INSERT … unnest`).
- **Reutilización** (`TestReuseNeverCrossesIntensityOrTenant`): `basic` nunca responde a `full`, ni entre organizaciones; `full` sin organización se rechaza.
- **Batería IDOR** (`internal/httpapi/idor_test.go`, hito 3.3c): dos organizaciones completas (owner, admin, viewer, una invitación pendiente y un escaneo de organización). Desde cada miembro de una, en los dos sentidos: cambiar el rol y quitar a cada miembro de la otra, revocar su invitación, cambiarse a ella, leer su escaneo y aceptar su invitación → siempre 404 (o 403 cuando el rol ya no lo permite); poner el ID de la otra en `?organization_id=` o en una cabecera `X-Org-ID` no cambia nada. Además: una foto de `organizations`, `memberships`, `invitations` y `scans` antes y después demuestra que ninguna petición cruzada cambió nada, y ninguna respuesta (`/org`, miembros, invitaciones, `/me`, errores) contiene un identificador, nombre o email de la otra organización. `TestNoRoleEscalationAcrossOrganizations`: la owner de A, viewer en B, no puede nada en B con su sesión en B, y desde A sus cambios no tocan B. Cada endpoint nuevo con IDs (dominios y escaneos en el 3.4) se añade a esta batería.
- **Prueba de mutación de la batería** (16/10/2026), estropeando a propósito una barrera cada vez:

| Mutación | Resultado |
|---|---|
| Quitar a un miembro no comprueba que el afectado sea de la organización | detectada (19 comprobaciones fallan) |
| Cambiar de organización sin comprobar la membresía | detectada (18) |
| Aceptar una invitación sin comprobar el email | detectada (33) |
| Listado de miembros sin el filtro `organization_id`, con RLS | no detectada: **RLS sola basta** |
| RLS apagada (conexión de superusuario), filtros de la aplicación intactos | no detectada: **los filtros solos bastan** |
| Sin filtro y sin RLS | detectada (72: aparecen los miembros de la otra organización) |

  Es lo que se busca con dos barreras independientes: cualquiera de las dos sola protege, y la batería detecta cuando faltan las dos.
- **Hito 3.4**: los endpoints de dominios y de escaneos de organización entran en la batería: leer, borrar y pedir la verificación de un dominio de la otra organización, leer su escaneo por la ruta de organización, y lanzar un escaneo `full` de su dominio verificado (403: el dominio verificado de otra organización no cuenta). Segunda prueba de mutación (16/10/2026):

| Mutación | Resultado |
|---|---|
| Escaneo `full` sin comprobar que el dominio esté verificado | detectada |
| La ruta de escaneos de organización sirve escaneos públicos | detectada |
| Los hallazgos de un escaneo de organización se leen con el inquilino público | detectada: el informe salía **sin hallazgos** (RLS los ocultaba). Era un fallo real del código, encontrado y corregido en este hito |
| Consultas de dominios sin el filtro `organization_id`, con RLS | no detectada: RLS sola basta |
| Sin filtro y sin RLS | detectada (10) |

  Al preparar esta prueba apareció que, en los tests, la comprobación de `GET /api/scans/{id}` daba 404 porque la ruta no estaba registrada (la configuración de los tests no tenía escaneos): no probaba nada. Ahora la ruta existe en esos tests y la comprobación es real.
- **Límites por organización**: 20 invitaciones pendientes, 50 miembros contando las pendientes (aceptar nunca lo supera), 50 invitaciones creadas al día (en memoria; 429 con `Retry-After`) y 10 organizaciones por usuario. Cada dirección recibe como mucho 3 emails por hora.

## Roles en una organización

Definidos en `internal/org` (un test fija la matriz entera). La API decide con las filas del actor y del afectado bloqueadas en la misma transacción que el cambio, así que un rol no puede cambiar entre la comprobación y la acción.

| Acción | owner | admin | viewer |
|---|---|---|---|
| Ver la organización, sus miembros, escaneos y dominios | ✓ | ✓ | ✓ |
| Renombrar la organización | ✓ | ✓ | – |
| Invitar (como admin o viewer) y revocar invitaciones | ✓ | ✓ | – |
| Cambiar el rol de un miembro (a cualquier rol) | ✓ | – | – |
| Quitar a un miembro | ✓ | solo viewers | – |
| Salir de la organización | ✓ | ✓ | ✓ |
| Gestionar dominios y escaneos `full` (hito 3.4) | ✓ | ✓ | – |

- **Siempre queda un owner**: PostgreSQL lo impone con un trigger (`memberships_keep_owner`) en todo cambio o baja de una membresía; bloquea la fila de la organización, de modo que dos owners que se degradan a la vez no la dejan sin owner (hay un test de la carrera). La API responde **409** `last_owner`. Borrar la organización entera sigue funcionando.
- Nadie da más poder del que tiene: los owners se hacen por ascenso, nunca por invitación.
- **Invitaciones**: token de 256 bits (solo su SHA-256), en el fragmento del enlace (`/invite#token=…`), 7 días, un solo uso; una nueva a la misma dirección anula la anterior, y se pueden revocar. Solo la acepta la cuenta **con la dirección invitada** (un enlace reenviado no sirve a otra cuenta). Quien no tiene cuenta puede registrarse con la invitación en lugar del código de beta; su email queda confirmado (el enlace llegó a esa dirección) y no se le crea organización propia. Aceptar nunca baja el rol de quien ya es miembro. Durante la beta, las cuentas creadas por invitación (`users.signup_via = 'invitation'`) no pueden crear organizaciones propias (403 `new_organizations_closed`); se quita con `BETA_INVITED_CAN_CREATE_ORGS=true`.
- **`invitation_by_token`**: quien tiene el enlace aún no es miembro y RLS le oculta la invitación, así que la busca esta función `SECURITY DEFINER` (migración 00008). Es estrecha a propósito, y un test (`TestInvitationLookupIsNarrow`) lo comprueba todo:
  - `search_path` explícito: `pg_catalog`, el esquema de las tablas (`public` en producción; la migración lo escribe al crearla para que funcionen los esquemas por test) y `pg_temp` al final (si no se nombra, PostgreSQL lo consulta el primero);
  - una sola firma, `(hash bytea)`; solo busca por un hash de 32 bytes exacto (vacío, prefijo, más largo o distinto → nada);
  - devuelve solo invitaciones pendientes y sin caducar, y solo las columnas que usan la vista previa y la aceptación (id, organización y su nombre, email, rol, email de quien invita, caducidad);
  - `EXECUTE` revocado a `PUBLIC` y concedido solo a `qr_app` (`qr_worker` recibe *permission denied*).
  Necesita que el propietario de las migraciones no esté sujeto a RLS (superusuario, como en desarrollo, CI y producción); si no lo estuviera, falla cerrada (ninguna invitación es válida). **Hito 3.7**: las migraciones deben seguir ejecutándose con el propietario de la base de datos.
- Si a alguien lo quitan de la organización activa, su sesión pasa a su primera organización en la siguiente petición; sin ninguna, **403** `no_organization`.

## Dominios verificados (hito 3.4)

- **Qué se puede añadir** (`internal/domains`): nombres reales normalizados (minúsculas, punycode). Se rechazan IPs, nombres de uso especial (`.local`, `.localhost`, `.test`, `.example`, `.invalid`, `.onion`, `.internal`, `.alt`, todo `arpa`…), dominios de nivel superior desconocidos y **sufijos públicos** según la Public Suffix List (secciones ICANN y privada: `com`, `co.uk`, `github.io`…). La lista va incrustada y fijada a un commit de github.com/publicsuffix/list con su SHA-256 (`psl_version.go`; un test comprueba el archivo); `make psl` la actualiza al commit más reciente con al menos 14 días.
- **Comprobación del TXT** (`internal/dnscheck`), siempre en el worker:
  - Se pregunta a los **servidores autoritativos** de la zona, sin recursión, y solo cuenta una respuesta autoritativa. Los resolvers públicos fijos (1.1.1.1, 9.9.9.9, 8.8.8.8) solo sirven para encontrar los NS y sus direcciones.
  - Así no interviene ninguna caché: ni la del VPS (systemd-resolved), ni el DNS interno de Docker, ni la de un resolver. Una verificación refleja el registro tal como está ahora, y un registro recién borrado se detecta en la siguiente comprobación.
  - **Solo DNS**: CNAMEs, como mucho 4 saltos; nunca HTTP ni nada más. Timeout de 3 s por consulta y 20 s por dominio. Se usa UDP, y TCP si la respuesta llega truncada.
  - Las respuestas con otro ID o pregunta se descartan (tests con un servidor falso que miente).
  - Las direcciones de los NS pasan el filtro anti-SSRF del escáner: un dominio no puede apuntar sus NS a la red interna para hacer que el worker la sondee.
  - Si ningún servidor responde, es un **error temporal** (`dns_unavailable`, reintento en 5 min), nunca "registro ausente": una caída de DNS no quita la verificación.
  - DNSSEC no se valida: un atacante en la ruta entre el VPS y los servidores autoritativos podría falsear la respuesta. Es un riesgo aceptado para este tipo de verificación, el mismo método que usan los proveedores habituales.
- **Ciclo**: `pending` → (registro encontrado) `verified` → comprobación semanal → si falta, `lost` (los escaneos `full` siguen 14 días y se comprueba a diario) → si sigue faltando pasados los 14 días, `pending` otra vez: ya no está verificado, pero el historial se conserva. Si el registro vuelve, `verified`. Pedir la verificación (botón) la encola para el worker.

## Escaneo público (hito 3.5)

- **Cerrado por defecto** (`PUBLIC_SCAN_OPEN` desactivado): solo los dominios de la allowlist. Se abre después de la revisión de seguridad.
- **Abierto**: cualquier dominio, siempre en intensidad `basic` y siempre como inquilino público. Se rechazan IPs, nombres reservados, terminaciones desconocidas y sufijos públicos (las mismas reglas que al añadir dominios). Se exige además la prueba de trabajo (ALTCHA, 3.5b) y se aplican límites:
  - por IP: 5 por minuto y 30 al día (`RATE_LIMIT_*`);
  - por dominio registrable: 10 al día;
  - globales: 30 por minuto y 1.000 al día;
  - los escaneos reutilizados no cuentan.
- **Exclusiones**: hay dos vías. La lista del operador (`qradmin exclude add|remove|list`, tabla `scan_exclusions`, que ningún rol de servicio puede leer) y el **opt-out del dueño verificado** (`PATCH /api/org/domains/{id}` con `block_public`; solo cuenta mientras el dominio está verificado, incluida la gracia). Las dos cubren los subdominios y dan la **misma** respuesta (403 `scan_not_allowed`).
- **La función `public_scan_blocked`** (`SECURITY DEFINER`, con el mismo endurecimiento que `invitation_by_token` y su test) solo devuelve sí o no. Así una petición pública no puede saber qué organización tiene el dominio, ni si alguien lo ha verificado.
- **Un escaneo público nunca devuelve ni reutiliza el de una organización.** `TestOpenPublicScans` lo comprueba con un dominio verificado por una organización que tiene un escaneo `full` reciente: la respuesta pública es idéntica a la de un dominio cualquiera (salvo id, fechas y host), es un escaneo nuevo `basic` sin organización, y la URL pública del escaneo de la organización da 404. Prueba de mutación (16/10/2026):

| Mutación | Resultado |
|---|---|
| La reutilización ignora la organización y la intensidad | la detectan los tests del store; en la API, **RLS sola** sigue impidiendo la fuga |
| Lo mismo, con RLS apagada | detectada: la petición pública recibía el escaneo `full` de la organización |
| El opt-out cuenta aunque el dominio no esté verificado | detectada |
| No se aplican exclusiones ni opt-out | detectada |

## Cuentas

Autenticación propia (hito 3.2; `internal/auth`, `internal/httpapi/auth.go`, `internal/store/accounts.go`). Endpoints en [api.md](api.md#cuentas-apiauth-hito-32).

- **Contraseñas**: argon2id con los parámetros de OWASP (m = 19 MiB, t = 2, p = 1), formato PHC; si cambian los parámetros, se vuelve a calcular el hash al entrar. 12–128 caracteres, sin reglas de composición (NIST SP 800-63B); se rechazan las de una lista de contraseñas filtradas (NCSC, 100.000 más comunes, las de 12 caracteres o más, incrustada) y las que contienen la parte local del email.
- **Sesiones en el servidor**: token aleatorio de 256 bits en la cookie `__Host-qr_session` (`Secure`, `HttpOnly`, `SameSite=Lax`, sin `Domain`: no la ve fizsar.com); en la base de datos solo su SHA-256. 30 días como máximo, 7 sin uso; la caducidad la decide la propia consulta. Cada login crea una sesión nueva (sin fijación). Cambiar la contraseña cierra las demás sesiones; restablecerla, todas. Las dos anulan los enlaces de recuperación pendientes.
- **CSRF**, tres capas: `SameSite=Lax`; las peticiones con sesión que cambian algo exigen la cabecera `X-QR-CSRF` con el token de la sesión (comparación en tiempo constante; un formulario de otro sitio no puede poner cabeceras); y, con `PUBLIC_ORIGIN`, todo POST que el navegador marca como de otro sitio (`Origin` distinto o `Sec-Fetch-Site` `cross-site`/`same-site`) se rechaza, lo que cubre también el login y el registro, que no tienen sesión. Además, la API solo acepta `application/json`.
- **Sin enumeración de cuentas**: el registro con un email ya usado responde igual (y a su dueño le llega un aviso, sin gastar el código); el login da el mismo 401 para un email desconocido que para una contraseña mal, y en los dos casos se calcula un argon2 (`VerifyDummy`), así que tarda lo mismo; `email_not_verified` solo se dice a quien acierta la contraseña; recuperar contraseña y reenviar la confirmación responden siempre 202, y el trabajo (buscar la cuenta, crear el token, encolar el email) se hace **después** de responder, para que el tiempo de respuesta no lo delate. El límite por cuenta también se aplica a emails inexistentes.
- **Fuerza bruta**: login 10/min y 50/día por IP; 5 contraseñas fallidas por cuenta en 15 min (incluido el cambio de contraseña); los endpoints que envían emails o gastan tokens, 5/min y 30/día por IP; como mucho 3 emails por dirección y hora (los demás se descartan sin avisar). Sin bloqueo permanente de cuentas (no se puede dejar a alguien fuera a propósito). En memoria, como el límite de escaneos.
- **Tokens de email** (verificación 24 h, recuperación 30 min): 256 bits, solo su SHA-256 en la base de datos, de un solo uso, y pedir uno nuevo invalida los anteriores. En los enlaces van en el **fragmento** (`/verify#token=…`), que el navegador no envía al servidor: no quedan en los logs de Caddy ni en `Referer`; la web lo quita de la barra de direcciones al leerlo.
- **Beta cerrada**: registrarse exige un código de invitación (`qradmin signup-code`, se muestra una sola vez; solo se guarda su SHA-256), de un solo uso y con caducidad.
- **Auditoría** (`audit_log`): registro, confirmación, login correcto y fallido, logout, petición de recuperación, restablecimiento y cambio de contraseña, con la IP truncada (/24 o /48). Sin emails de terceros: un login fallido con un email inexistente se anota sin usuario.
- **Emails**: los escribe la API (texto plano, ES/EN) y los envía `cmd/mailer`; los logs del mailer llevan el ID y la plantilla, nunca la dirección.

Pruebas (`internal/httpapi/auth_test.go`, contra PostgreSQL con el rol `qr_app`): ciclo completo, validación, no enumeración (respuestas idénticas y emails solo a quien corresponde), fuerza bruta, recuperación (un enlace sustituido deja de valer, un intento rechazado no gasta el enlace, cierra sesiones), cambio de contraseña, Origin y CSRF, caducidad de sesiones, límite de emails por dirección. Prueba de mutación (16/10/2026): sin la comprobación CSRF fallan 2 tests, sin la de `Origin` 1, sin contar los fallos de login 1, y si un email ocupado responde distinto (409) falla el de enumeración. Estos tests encontraron que el registro no funcionaba bajo RLS (`INSERT … RETURNING` en `organizations` exige poder leer la fila): ahora el ID de la organización lo genera la aplicación y se fija como inquilino antes de crearla, sin relajar la política.

---

## English

From phase 3, scans belong to an organization (or to nobody: public scans). Two independent barriers keep organizations apart: **(1) the application** takes the organization from the session (never from the request), filters every query by `organization_id`, answers 404 for other organizations' resources, and reuses scans only with the same host, port, intensity and organization; **(2) PostgreSQL row-level security**: API queries run in `store.Store.Tx` as role `qr_app` with transaction-local `app.org_id`/`app.user_id`, and every organization table has RLS enabled and forced, so a query that forgets its filter still sees and changes only its tenant's rows (public scans stay readable). Memberships and organizations have per-command policies: users see their memberships everywhere but change only the active organization's. The worker runs as `qr_worker`, without `BYPASSRLS`: explicit policies on the scan queue and findings only, and no access to users, sessions, auth tokens, organizations, domains, the audit log or emails (tested: permission denied). Email goes out through a separate process, `cmd/mailer`, as `qr_mailer`, which only touches the email outbox (rows carry recipient, subject and body already rendered); it is separate because the worker binary relaxes crypto/tls for scanning. Every service acts as its role from connection time (`store.UseRole`); until milestone 3.7 they log in as the owner, from 3.7 as their own users. Worker tests run the worker with its production role. Tests: a catalog check that every organization table has forced RLS with policies; an isolation suite (application queries, unfiltered queries, writes into another tenant, the public tenant, no tenant leak across pooled connections); a mutation run without `SET LOCAL ROLE` makes 13 of those checks fail, proving they rely on RLS; reuse never crosses intensity or tenant. Endpoints taking organization IDs join an IDOR suite in milestones 3.3–3.4.

**Accounts (milestone 3.2).** Passwords: argon2id (OWASP parameters, rehash on login when they change), 12–128 characters, no composition rules, rejected if on the embedded NCSC leaked-password list or containing the email's local part. Server-side sessions: a 256-bit token in the `__Host-qr_session` cookie (Secure, HttpOnly, SameSite=Lax, no Domain), only its SHA-256 stored, 30 days absolute and 7 idle (enforced by the query), a new session on every login, other sessions ended on password change and all on reset; both revoke pending reset links. CSRF: SameSite=Lax, an `X-QR-CSRF` header with the session token on state-changing calls (constant-time compare), and with `PUBLIC_ORIGIN` every POST a browser marks as cross-site is refused (covers login and signup too); JSON only. No account enumeration: a taken email gets the same signup answer (its owner gets an email; the code is not spent), unknown email and wrong password give the same 401 after the same argon2 work, `email_not_verified` only after the right password, forgot/resend always 202 with the work done after the response. Brute force: login 10/min and 50/day per IP, 5 failed passwords per account per 15 min (unknown emails too), 5/min and 30/day per IP on email and token endpoints, at most 3 emails per address per hour (silently); no permanent lockout. Email tokens (24 h verification, 30 min reset): 256-bit, hash only, single use, a new one revokes the old, carried in the URL fragment so they never reach server logs or Referer. Closed beta: one-time signup codes from `qradmin signup-code`, hash only. Audit log of account events with the IP truncated to /24 or /48. Tests run against PostgreSQL as `qr_app`; a mutation run (16/10/2026) showed each of the CSRF, Origin, failed-login and enumeration checks is covered. They also caught that signup failed under RLS (`INSERT … RETURNING` on organizations needs the row to be readable): the application now generates the organization ID and sets it as the tenant first, without loosening the policy.
