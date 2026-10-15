# Seguridad: aislamiento entre organizaciones / Security: tenant isolation

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
| `qr_worker` | Worker: solo la cola de escaneos (`scans`: leer y actualizar) y guardar hallazgos (`findings`: insertar) | Sí, sin `BYPASSRLS`: políticas explícitas en esas dos tablas para todas las organizaciones. **Sin acceso** a `users`, `sessions`, `auth_tokens`, organizaciones, dominios (llegará en el 3.4, solo lo necesario para re-verificar), auditoría ni emails |
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
- **Endpoints (hitos 3.2–3.4)**: cada endpoint nuevo entra en una batería IDOR que lo llama con IDs de la otra organización y exige 404 y que no aparezcan en ningún listado.

---

## English

From phase 3, scans belong to an organization (or to nobody: public scans). Two independent barriers keep organizations apart: **(1) the application** takes the organization from the session (never from the request), filters every query by `organization_id`, answers 404 for other organizations' resources, and reuses scans only with the same host, port, intensity and organization; **(2) PostgreSQL row-level security**: API queries run in `store.Store.Tx` as role `qr_app` with transaction-local `app.org_id`/`app.user_id`, and every organization table has RLS enabled and forced, so a query that forgets its filter still sees and changes only its tenant's rows (public scans stay readable). Memberships and organizations have per-command policies: users see their memberships everywhere but change only the active organization's. The worker runs as `qr_worker`, without `BYPASSRLS`: explicit policies on the scan queue and findings only, and no access to users, sessions, auth tokens, organizations, domains, the audit log or emails (tested: permission denied). Email goes out through a separate process, `cmd/mailer`, as `qr_mailer`, which only touches the email outbox (rows carry recipient, subject and body already rendered); it is separate because the worker binary relaxes crypto/tls for scanning. Every service acts as its role from connection time (`store.UseRole`); until milestone 3.7 they log in as the owner, from 3.7 as their own users. Worker tests run the worker with its production role. Tests: a catalog check that every organization table has forced RLS with policies; an isolation suite (application queries, unfiltered queries, writes into another tenant, the public tenant, no tenant leak across pooled connections); a mutation run without `SET LOCAL ROLE` makes 13 of those checks fail, proving they rely on RLS; reuse never crosses intensity or tenant. New endpoints join an IDOR suite in milestones 3.2–3.4.
