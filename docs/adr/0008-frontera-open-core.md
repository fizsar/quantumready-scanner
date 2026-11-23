# ADR-0008: Qué paquetes pueden importarse desde fuera

- **Estado:** aceptada el 24/10/2026 (versión pública del 17/11/2026).
- **Cumplimiento hoy:** en parte: la regla de dependencias se comprueba en la CI; los paquetes siguen bajo `internal/`.

## Contexto

El motor de escaneo debería poder usarse sin el servicio: una CLI pública del escáner, o cualquier programa que quiera analizar endpoints TLS con las mismas reglas. Para eso el motor, los evaluadores y el catálogo deben poder importarse desde otro módulo, y no deben arrastrar la API ni la base de datos.

## Decisión

**Paquetes importables:**
- el motor: `scanner` y `scanner/rawtls`;
- los evaluadores: `scoring`, sin textos;
- el catálogo de textos y referencias ([ADR-0002](0002-tres-capas.md) y [ADR-0003](0003-normas-como-datos.md));
- `domains` (validación de dominios y Public Suffix List).

**Paquetes del servicio:** `store`, `httpapi`, `worker`, `auth`, `org`, `mail`, `altcha`, `testdb` y los `cmd/` del servicio.

**Regla:** los importables nunca dependen de los del servicio, ni de pgx, ni de net/http como servidor. Lo comprueba la CI.

## Estado

- `internal/scanner` → solo `internal/scanner/rawtls`; `rawtls` → nada interno; `scoring` → `scanner` y `rawtls`; `domains` → nada interno. Ninguno depende de `store`, `httpapi`, pgx ni chi.
- La CI lo comprueba con `go list -deps`.
- Lo que falta: están bajo `internal/`, que Go prohíbe importar desde otro módulo, y `scoring` aún mezcla evaluador y textos ([ADR-0002](0002-tres-capas.md)).

## Consecuencias

- Una CLI y el servicio comparten el mismo motor y las mismas reglas, sin copiar código.
- La CI impide que el motor empiece a depender de la base de datos por descuido.

## Siguiente paso

Mover esos paquetes fuera de `internal/` (a `pkg/` o a un módulo propio), junto con la CLI `qrscan`, después de separar el catálogo ([ADR-0002](0002-tres-capas.md)).
