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
| `qr_worker` | Worker: cola, emails, verificación de dominios, retención | `BYPASSRLS`: trabaja para todas las organizaciones |
| propietario (`POSTGRES_USER`) | Migraciones | No aplica |

Hasta el hito 3.7 la API y el worker se conectan como propietario y cambian de rol en cada transacción; en el 3.7 pasarán a conectarse con usuarios propios (`qr_app`, `qr_worker`) con contraseñas generadas en el VPS, de modo que la API nunca tenga privilegios de propietario.

## Tests

- **Cobertura de RLS** (`TestRLSCoversEveryOrganizationTable`): recorre el catálogo de PostgreSQL y falla si una tabla con `organization_id` no tiene RLS activado, forzado y con políticas. Una tabla nueva que lo olvide rompe el test.
- **Aislamiento** (`TestTenantIsolation`): dos organizaciones con usuarios, dominios y escaneos:
  - las consultas de la aplicación no cruzan organizaciones;
  - una consulta **sin filtro** solo ve el inquilino (RLS);
  - insertar, modificar o borrar en otra organización se rechaza o no afecta a ninguna fila (incluido ascenderse a owner en otra organización);
  - el inquilino público solo ve escaneos públicos;
  - el inquilino no se filtra entre transacciones del pool.
- **Prueba de mutación** (15/10/2026): quitando `SET LOCAL ROLE qr_app`, `TestTenantIsolation` falla en 13 comprobaciones (B ve los dominios, escaneos y hallazgos de A, borra su dominio, se asciende…); con RLS, todas pasan. Las barreras dependen de verdad de RLS.
- **Reutilización** (`TestReuseNeverCrossesIntensityOrTenant`): `basic` nunca responde a `full`, ni entre organizaciones; `full` sin organización se rechaza.
- **Endpoints (hitos 3.2–3.4)**: cada endpoint nuevo entra en una batería IDOR que lo llama con IDs de la otra organización y exige 404 y que no aparezcan en ningún listado.

---

## English

From phase 3, scans belong to an organization (or to nobody: public scans). Two independent barriers keep organizations apart: **(1) the application** takes the organization from the session (never from the request), filters every query by `organization_id`, answers 404 for other organizations' resources, and reuses scans only with the same host, port, intensity and organization; **(2) PostgreSQL row-level security**: API queries run in `store.Store.Tx` as role `qr_app` with transaction-local `app.org_id`/`app.user_id`, and every organization table has RLS enabled and forced, so a query that forgets its filter still sees and changes only its tenant's rows (public scans stay readable). Memberships and organizations have per-command policies: users see their memberships everywhere but change only the active organization's. The worker runs as `qr_worker` (`BYPASSRLS`, it serves every organization); migrations as the owner. Until milestone 3.7 the API and worker log in as the owner and switch role per transaction; from 3.7 they log in as their own roles. Tests: a catalog check that every organization table has forced RLS with policies; an isolation suite (application queries, unfiltered queries, writes into another tenant, the public tenant, no tenant leak across pooled connections); a mutation run without `SET LOCAL ROLE` makes 13 of those checks fail, proving they rely on RLS; reuse never crosses intensity or tenant. New endpoints join an IDOR suite in milestones 3.2–3.4.
