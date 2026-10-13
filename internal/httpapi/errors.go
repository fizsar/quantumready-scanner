package httpapi

import "net/http"

// apiError is the body of every error: {"error": {"code": "...", "message": "..."}}.
type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeError(w http.ResponseWriter, r *http.Request, status int, code string) {
	lang := pickLang(r)
	w.Header().Set("Content-Language", lang)
	writeJSON(w, status, map[string]apiError{"error": {Code: code, Message: message(code, lang)}})
}

func message(code, lang string) string {
	if m, ok := messages[code]; ok {
		if s, ok := m[lang]; ok {
			return s
		}
		return m["es"]
	}
	return code
}

// messages covers API errors and the stable failure codes stored in scans.error.
var messages = map[string]map[string]string{
	"invalid_json":           {"es": "El cuerpo de la petición no es un JSON válido.", "en": "The request body is not valid JSON."},
	"body_too_large":         {"es": "El cuerpo de la petición es demasiado grande.", "en": "The request body is too large."},
	"unsupported_media_type": {"es": "Content-Type debe ser application/json.", "en": "Content-Type must be application/json."},
	"invalid_host":           {"es": "El dominio no es válido.", "en": "The domain is not valid."},
	"port_not_allowed":       {"es": "Por ahora solo se escanea el puerto 443 (HTTPS). Quita el puerto o usa :443.", "en": "For now only port 443 (HTTPS) is scanned. Remove the port or use :443."},
	"host_not_allowed":       {"es": "Por ahora solo se pueden escanear los dominios de demostración.", "en": "For now only the demo domains can be scanned."},
	"rate_limited":           {"es": "Demasiados escaneos. Espera un poco y vuelve a intentarlo.", "en": "Too many scans. Wait a little and try again."},
	"not_found":              {"es": "No encontrado.", "en": "Not found."},
	"method_not_allowed":     {"es": "Método no permitido.", "en": "Method not allowed."},
	"internal":               {"es": "Error interno. Inténtalo más tarde.", "en": "Internal error. Try again later."},

	// Scan failures (scans.error).
	"blocked_address":    {"es": "El dominio apunta a una dirección interna o reservada; no se escanea.", "en": "The domain points to an internal or reserved address; it is not scanned."},
	"invalid_target":     {"es": "El destino no es válido.", "en": "The target is not valid."},
	"dns_not_found":      {"es": "El dominio no existe o no tiene dirección.", "en": "The domain does not exist or has no address."},
	"dns_temporary":      {"es": "El DNS no respondió a tiempo.", "en": "DNS did not answer in time."},
	"dns":                {"es": "No se pudo resolver el dominio.", "en": "The domain could not be resolved."},
	"connection_refused": {"es": "El servidor rechazó la conexión en el puerto 443.", "en": "The server refused the connection on port 443."},
	"timeout":            {"es": "El servidor no respondió a tiempo.", "en": "The server did not answer in time."},
	"network":            {"es": "La conexión se interrumpió.", "en": "The connection was interrupted."},
	"unreachable":        {"es": "El servidor no responde con TLS en el puerto 443.", "en": "The server does not speak TLS on port 443."},
	"scan_error":         {"es": "El escaneo falló.", "en": "The scan failed."},
	"lease_expired":      {"es": "El escaneo no pudo completarse tras varios intentos.", "en": "The scan could not complete after several attempts."},
}
