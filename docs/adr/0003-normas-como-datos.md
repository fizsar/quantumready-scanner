# ADR-0003: Las correspondencias con normas son datos

- **Estado:** aceptada el 24/10/2026 (versión pública del 17/11/2026).
- **Cumplimiento hoy:** en parte.

## Contexto

Cada hallazgo cita su fuente técnica: un RFC, un CVE, un borrador del IETF, NIST o el CA/Browser Forum. Si alguna vez se quisiera relacionar los hallazgos con otros marcos (por ejemplo, guías nacionales de seguridad), esas tablas cambian con el tiempo y no deben obligar a tocar la lógica de evaluación.

## Decisión

- **Una tabla de datos:** «código de hallazgo → lista de referencias», con `{marco, identificador, título, URL, carácter}`. El carácter distingue lo normativo de lo orientativo.
- **Quién la lee:** los generadores de informe. Los evaluadores no la usan para puntuar.
- **Añadir un marco** consiste en añadir filas, no código.
- **Límite:** una correspondencia con una norma de gestión es siempre orientativa y nunca afirma cumplimiento. Lo comprueba la lista de afirmaciones prohibidas (ver el [índice](README.md)).

## Consecuencias

- La tabla vive hoy en Markdown, en la sección «Referencias por hallazgo» de [docs/scoring.md](../scoring.md). [internal/catalog/references_test.go](../../internal/catalog/references_test.go) comprueba que cada código del catálogo tiene su fila, que cada fuente enlaza a su sitio oficial y que no sobran filas.
- Todavía no la lee ningún código: ni la API ni los informes exponen las referencias. Además, el catálogo cita algunas fuentes dentro del texto (por ejemplo, «FIPS 203»), que no se pueden filtrar por marco.
- Siguiente paso: un fichero de datos embebido en el paquete del catálogo, del que la documentación se compruebe o se genere.
