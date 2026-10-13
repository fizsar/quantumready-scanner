# Grupos de intercambio de claves en el Caddy del VPS (NO aplicado)

Objetivo: que fizsar.com (y quantumready.fizsar.com) acepten también el grupo híbrido **SecP256r1MLKEM768** (P-256 + ML-KEM-768, útil para clientes FIPS), además de X25519MLKEM768. Daría los 5 puntos de "variedad de híbridos" (95 → 100).

## Estado (13/10/2026): no es posible con Caddy publicado

- La directiva `curves` de Caddy **reemplaza** la lista por defecto. Hoy, sin `curves`, Caddy ofrece `x25519mlkem768`, `x25519` y `secp256r1` (lo que mide nuestro escáner; `secp384r1` no).
- Caddy **2.11.4, 2.11.6 y 2.11.7** (la última publicada; la del VPS es 2.11.7) rechazan el nombre:
  `Wrong curve name or curve not supported: 'secp256r1mlkem768'`.
  Probado con `caddy validate` en las imágenes oficiales `caddy:2.11.x`. Tampoco acepta `SecP256r1MLKEM768`.
- Nombres que **sí** acepta 2.11.7: `x25519mlkem768`, `x25519`, `secp256r1`, `secp384r1`, `secp521r1`.
- Go (dentro de Caddy) sí implementa SecP256r1MLKEM768, pero Caddy fija su propia lista y no tiene nombre para ese grupo. Las alternativas (compilar Caddy parcheado con xcaddy) no compensan el mantenimiento para 5 puntos.

**Recomendación**: esperar a una versión de Caddy que lo añada. Al actualizar la imagen `caddy:2` del VPS, repetir la comprobación de abajo.

## Cambio preparado (para cuando Caddy lo admita)

1. Comprobar el nombre exacto en la nueva versión (debe decir `Valid configuration`):

   ```sh
   dir=$(mktemp -d)
   printf 'example.test {\n\ttls internal {\n\t\tcurves x25519mlkem768 secp256r1mlkem768 x25519 secp256r1 secp384r1\n\t}\n\trespond "ok"\n}\n' > "$dir/Caddyfile"
   docker run --rm -v "$dir:/etc/caddy:ro" caddy:<versión> caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile
   ```

2. Añadir dentro del bloque `fizsar.com { … }` (y, si se quiere, en `quantumready.fizsar.com { … }`):

   ```caddyfile
   	tls {
   		# Reemplaza la lista por defecto: incluir también los clásicos.
   		curves x25519mlkem768 secp256r1mlkem768 x25519 secp256r1 secp384r1
   	}
   ```

   `tls { … }` sin emisor mantiene los certificados automáticos (ACME).

3. Aplicarlo con el procedimiento de CLAUDE.md ("Cambios en Caddy"): copia con fecha en `/srv/caddy/backups/`, `conf/Caddyfile.new`, `caddy validate` sobre el `.new`, `mv` y `caddy reload` (nunca restart). Vuelta atrás: restaurar la copia y reload. **Requiere OK de Fiz: toca fizsar.com.**

## Verificación después de aplicarlo

```sh
# Nuestro escáner: el grupo debe salir "supported"
go run ./cmd/qrscan -facts fizsar.com | grep -E 'SecP256r1MLKEM768|X25519MLKEM768|secp384r1'

# OpenSSL 3.5+ (en WSL: 3.5.5): debe completar el handshake con ese grupo
openssl s_client -connect fizsar.com:443 -servername fizsar.com -groups SecP256r1MLKEM768 -brief </dev/null 2>&1 | grep -iE 'group|Protocol|error'

# Nota completa: variedad de híbridos 5/5
go run ./cmd/qrscan fizsar.com | head -12
```

Y comprobar que fizsar.com sigue igual para los navegadores (grupo elegido X25519MLKEM768, mismo HTML y cabeceras).
