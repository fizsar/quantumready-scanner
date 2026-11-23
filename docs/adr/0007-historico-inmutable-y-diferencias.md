# ADR-0007: Histórico inmutable y diferencias entre resultados

- **Estado:** aceptada el 24/10/2026 (versión pública del 17/11/2026).
- **Cumplimiento hoy:** en parte: la inmutabilidad sí; las diferencias no.

## Contexto

Comparar el resultado de hoy con el anterior y avisar de lo que cambia (aparece SSLv3, desaparece ML-KEM, caduca el certificado…) solo tiene sentido si un resultado guardado no puede cambiar después.

## Decisión

- **Un escaneo terminado** (`done` o `failed`) **no se modifica nunca**, y la base de datos lo impide. Solo lo borra la retención.
- **Las diferencias** entre dos resultados las calcula una función pura de la capa de evaluadores ([ADR-0002](0002-tres-capas.md)): sobre observaciones y códigos, no sobre textos.

## Consecuencias

- Cada escaneo es una fila nueva. El resultado se escribe una sola vez (`CompleteScan`, en [internal/store/queries/scans.sql](../../internal/store/queries/scans.sql)), y se guardan `scanner_schema` y `scoring_version`, así que un resultado antiguo sigue siendo interpretable.
- La garantía está en la base de datos: la política `scans_worker_update` de `qr_worker` solo alcanza los escaneos `queued` o `running` ([migrations/00020_freeze_finished_scans.sql](../../migrations/00020_freeze_finished_scans.sql)). `TestFinishedScansFrozenForWorker` ([internal/store/purge_test.go](../../internal/store/purge_test.go)) intenta modificar uno terminado, con prueba de mutación.
- Las organizaciones conservan 13 meses de historial; los escaneos públicos, 30 días.
- Aún no existe la función `Diff(antes, después)` sobre resultados y hallazgos. Sería la base de las alertas y de la comparación entre fechas ([ADR-0006](0006-informe-conjunto-de-escaneos.md)).
