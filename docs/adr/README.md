# Decisiones de arquitectura (ADR)

Estas decisiones fijan cómo se separan las piezas de QuantumReady para que cada una pueda cambiar sin reescribir las demás.

**Contexto técnico.** El resultado de un escaneo TLS debe poder:
- regenerarse más tarde, en otro idioma o con otra redacción;
- mostrarse en varios formatos (web, JSON y otros que se añadan);
- evaluarse con las mismas reglas desde el servicio y desde una CLI;
- apoyarse en referencias técnicas (RFC, NIST, CA/Browser Forum) y, si se quisiera, en otras normas representadas como datos.

Cada ADR dice qué se decidió, cómo está hoy el código y qué falta.

| ADR | Decisión | Hoy |
|---|---|---|
| [0001](0001-hechos-no-textos.md) | Guardar hechos, no textos | Sí |
| [0002](0002-tres-capas.md) | Motor, evaluadores y generadores de informe, separados | Sí |
| [0003](0003-normas-como-datos.md) | Las correspondencias con normas son datos | En parte |
| [0004](0004-objetivo-host-puerto-protocolo.md) | Un objetivo es host + puerto + protocolo | En parte |
| [0005](0005-recolectores-tras-interfaz.md) | Las comprobaciones que no son TLS son recolectores tras una interfaz | En parte |
| [0006](0006-informe-conjunto-de-escaneos.md) | Un informe es un conjunto de escaneos | No |
| [0007](0007-historico-inmutable-y-diferencias.md) | Histórico inmutable y diferencias entre resultados | En parte |
| [0008](0008-frontera-open-core.md) | Qué paquetes pueden importarse desde fuera | En parte |

**Afirmaciones prohibidas.** La web y los informes nunca dicen «oficial», «certificado ENS» ni «cumple el ENS», ni que NIS2 «exige» criptografía post-cuántica. Lo comprueban `forbiddenClaims` (`web/src/test/forbidden.ts`, todos los textos de la web) y `ForbiddenClaims` en [internal/catalog/claims.go](../../internal/catalog/claims.go) (textos del catálogo y mensajes de la API). Cualquier formato nuevo debe pasar los mismos patrones.
