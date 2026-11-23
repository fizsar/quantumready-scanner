# ADR-0002: Motor, evaluadores y generadores de informe, separados

- **Estado:** aceptada el 24/10/2026 (versión pública del 17/11/2026).
- **Cumplimiento hoy:** sí.

## Contexto

El resultado de un escaneo se usa de varias maneras: la web, el JSON de la API, la línea de órdenes y, en el futuro, otros formatos de documento. Si el informe se arma dentro de la capa HTTP, cada formato nuevo duplica esa lógica, y las reglas de puntuación no se pueden probar sin servidor ni base de datos.

## Decisión

Tres capas, con dependencias en un solo sentido:
1. **Motor** (`internal/scanner`): los escáneres. Sin base de datos, sin HTTP y sin textos para el usuario. Devuelve observaciones.
2. **Evaluadores** (`internal/scoring`): funciones puras de observaciones a juicios (puntuación y hallazgos). Sin textos.
3. **Generadores de informe** (`internal/report`): `report.Build` es una función pura que construye un **modelo de documento neutro** (secciones, tablas, hallazgos con su texto ya resuelto) a partir de los juicios y del catálogo de textos (`internal/catalog`). Cada formato (web y JSON hoy) solo serializa ese modelo.

## Consecuencias

- Un formato nuevo es un serializador más, sin reglas propias.
- Los evaluadores se prueban sin base de datos, con los resultados del laboratorio TLS (`test/tlslab`).
- El motor se usa a través de una interfaz (`Scanner`, en [internal/worker/worker.go](../../internal/worker/worker.go)) y solo importa `internal/scanner/rawtls`.
- La API solo serializa el `Document` ([internal/report/report.go](../../internal/report/report.go)); los textos viven en [internal/catalog](../../internal/catalog/catalog.go), no en `scoring`.
- Esta separación es la base de [ADR-0001](0001-hechos-no-textos.md) (hechos en la base de datos, textos en el catálogo) y de [ADR-0008](0008-frontera-open-core.md) (qué paquetes pueden importarse desde fuera).
