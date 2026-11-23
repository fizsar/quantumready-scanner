# ADR-0005: Las comprobaciones que no son TLS son recolectores tras una interfaz

- **Estado:** aceptada el 24/10/2026 (versión pública del 17/11/2026).
- **Cumplimiento hoy:** en parte.

## Contexto

Algunas comprobaciones útiles no son un handshake TLS:
- **DNS:** registros TXT (hoy, para verificar la propiedad de un dominio), y en el futuro SPF, DKIM, DMARC, MTA-STS, TLSA o CAA;
- **Certificate Transparency,** para descubrir certificados.

Algunas dependen de un servicio externo (un registro de CT, un agregador) que puede caer, limitar las peticiones o cambiar.

## Decisión

- **Cada fuente es un recolector** detrás de una interfaz común, por ejemplo `Collect(ctx, Target) (Observation, error)`. Devuelve observaciones con fuente y fecha, con la misma regla de [ADR-0001](0001-hechos-no-textos.md): hechos, no textos.
- **El servicio externo va detrás de su propia interfaz**, con una implementación falsa en los tests: cambiar de proveedor no toca el recolector.
- **Las reglas anti-SSRF del motor** se aplican también a los recolectores.

## Consecuencias

- Hoy el worker usa el escáner a través de una interfaz (`Scanner`, en [internal/worker/worker.go](../../internal/worker/worker.go)) y la verificación DNS va detrás de `TXTLookup` ([internal/worker/domains.go](../../internal/worker/domains.go)). `dnscheck.Checker` tiene resolvers configurables, pregunta a los servidores autoritativos y reutiliza las reglas de direcciones del escáner ([internal/dnscheck/dnscheck.go](../../internal/dnscheck/dnscheck.go)).
- Todavía no hay una interfaz común de recolector ni código que consulte Certificate Transparency.
- Añadir una fuente supone un recolector, su interfaz de servicio y sus tests con datos falsos, sin cambios en el worker.
