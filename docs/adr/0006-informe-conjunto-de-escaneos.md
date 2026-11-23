# ADR-0006: Un informe es un conjunto de escaneos

- **Estado:** aceptada el 24/10/2026 (versión pública del 17/11/2026).
- **Cumplimiento hoy:** no. No bloquea, porque los datos lo permiten.

## Contexto

Hoy un informe es un escaneo de un objetivo. Un informe más amplio, que cubra varios dominios de una organización en un momento dado o compare dos momentos, no es un escaneo: es una selección de ellos.

## Decisión

- **Un informe es una entidad propia:** organización, fecha, tipo y la lista de escaneos que incluye.
- **Los escaneos no cambian** ([ADR-0007](0007-historico-inmutable-y-diferencias.md)) y un mismo escaneo puede aparecer en varios informes.
- **El generador** ([ADR-0002](0002-tres-capas.md)) recibe una lista de escaneos. La vista de un escaneo es una sección.

## Consecuencias

- Hoy todo asume un escaneo por informe: `GET /api/scans/{id}` devuelve el escaneo con su informe dentro ([internal/httpapi/scans.go](../../internal/httpapi/scans.go)), y el componente `Report` recibe un solo escaneo ([web/src/components/Report.tsx](../../web/src/components/Report.tsx)).
- Nada en la base de datos ata un informe a un escaneo: los escaneos son filas independientes con `organization_id`.
- Siguiente paso: tablas `reports` (id, organización, fecha, tipo) y `report_scans`, y que `report.Build` reciba una lista. La página de un escaneo seguiría igual, como un informe de una sola sección.
