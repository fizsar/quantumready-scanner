# ADR-0001: Guardar hechos, no textos

- **Estado:** aceptada el 24/10/2026 (versión pública del 17/11/2026).
- **Cumplimiento hoy:** sí.

## Contexto

Un informe puede regenerarse meses después: en otro idioma, con otra redacción, en otro formato o con una tabla de referencias ampliada. Si la base de datos guarda prosa, cada cambio de redacción deja los resultados antiguos con textos viejos u obliga a migrar datos.

## Decisión

El resultado guardado contiene solo:
- **observaciones:** lo que el escáner vio;
- **códigos de hallazgo**, con su eje, gravedad y categoría;
- **parámetros.**

Títulos, explicaciones, consejos y etiquetas se generan al mostrarlos, a partir de un catálogo versionado. Un error de una sonda también es un hecho: se guarda como código estable (`error_code`, `trust_error_code`), y el mensaje de la biblioteca solo como evidencia de depuración.

## Consecuencias

- Los resultados antiguos se muestran con el catálogo actual, en cualquier idioma.
- La nota y la letra sí se guardan (`score`, `grade`, `scoring_version`): el juicio de entonces no cambia aunque cambie el modelo.
- La tabla `findings` guarda `code`, `axis`, `severity`, `category` y `params` ([migrations/00002_scans.sql](../../migrations/00002_scans.sql)); no hay textos en la base de datos. El catálogo ([internal/catalog/catalog.go](../../internal/catalog/catalog.go)) los resuelve al servir la API, a través de [internal/report/report.go](../../internal/report/report.go).
- El resultado del escáner lleva un esquema versionado (`SchemaVersion = 2`, [internal/scanner/result.go](../../internal/scanner/result.go)); quien lee sigue aceptando la versión 1. Los códigos de error están en [internal/scanner/errcode.go](../../internal/scanner/errcode.go).
- Como el catálogo es la fuente de la redacción, las afirmaciones prohibidas se comprueban sobre él (ver el [índice](README.md)).
