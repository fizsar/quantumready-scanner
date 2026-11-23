# ADR-0004: Un objetivo es host + puerto + protocolo

- **Estado:** aceptada el 24/10/2026 (versión pública del 17/11/2026).
- **Cumplimiento hoy:** en parte.

## Contexto

Hoy solo se escanea HTTPS en el puerto 443. Otros servicios con TLS usan otros puertos y otros modos de conexión: SMTP con STARTTLS en el 25 o el 587, el 465 con TLS directo, IMAPS… Con el mismo puerto puede cambiar el protocolo, porque STARTTLS no es TLS directo.

## Decisión

- **Todo objetivo** es `(host, puerto, protocolo)` en el modelo de datos, en el motor y en la API.
- **La política de qué combinaciones se permiten** (hoy solo `443/https`) es configuración de la API, no una constante repartida por el código.

## Consecuencias

- `scans` guarda `host` y `port` (443 por defecto), y el índice es por `(host, port, created_at)` ([migrations/00002_scans.sql](../../migrations/00002_scans.sql)). El motor acepta el puerto y lo anota en el resultado ([internal/scanner/result.go](../../internal/scanner/result.go)).
- El protocolo aún es implícito («TLS directo sobre TCP»), y el 443 está fijado en la API (`ScanPort`, en [internal/httpapi/scans.go](../../internal/httpapi/scans.go); `NormalizeHost`, en [internal/httpapi/target.go](../../internal/httpapi/target.go)) y en algunos textos de la web.
- Siguiente paso: una columna `protocol` con lista cerrada, un `scanner.Target{Host, Port, Protocol}` y una lista de combinaciones permitidas en lugar de `ScanPort`.
- Con el protocolo explícito, un modo de conexión nuevo es una variante del motor, no un escáner paralelo, y la reutilización de escaneos y los límites se calculan por objetivo completo.
