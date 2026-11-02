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
	"host_not_allowed":       {"es": "Ahora mismo el escaneo está limitado a los dominios de demostración.", "en": "Right now, scanning is limited to the demo domains."},
	"rate_limited":           {"es": "Demasiados escaneos. Espera un poco y vuelve a intentarlo.", "en": "Too many scans. Wait a little and try again."},
	"not_found":              {"es": "No encontrado.", "en": "Not found."},
	"method_not_allowed":     {"es": "Método no permitido.", "en": "Method not allowed."},
	"internal":               {"es": "Error interno. Inténtalo más tarde.", "en": "Internal error. Try again later."},
	"bad_origin":             {"es": "Petición rechazada: no viene de esta web.", "en": "Request refused: it does not come from this site."},

	// Accounts.
	"invalid_email":             {"es": "El email no es válido.", "en": "The email is not valid."},
	"invalid_name":              {"es": "El nombre no es válido (máximo 100 caracteres).", "en": "The name is not valid (100 characters at most)."},
	"invalid_organization":      {"es": "El nombre de la organización no es válido (máximo 100 caracteres).", "en": "The organization name is not valid (100 characters at most)."},
	"invalid_signup_code":       {"es": "El código de invitación no es válido, ha caducado o ya se usó.", "en": "The invitation code is not valid, has expired or was already used."},
	"password_too_short":        {"es": "La contraseña debe tener al menos 12 caracteres.", "en": "The password must have at least 12 characters."},
	"password_too_long":         {"es": "La contraseña puede tener como mucho 128 caracteres.", "en": "The password can have 128 characters at most."},
	"password_common":           {"es": "Esa contraseña aparece en listas de contraseñas filtradas. Elige otra.", "en": "That password appears in lists of leaked passwords. Choose another one."},
	"password_contains_email":   {"es": "La contraseña no puede contener tu email.", "en": "The password cannot contain your email."},
	"invalid_credentials":       {"es": "Email o contraseña incorrectos.", "en": "Wrong email or password."},
	"email_not_verified":        {"es": "Confirma tu email antes de entrar: busca el enlace en tu correo.", "en": "Confirm your email before signing in: look for the link in your inbox."},
	"too_many_attempts":         {"es": "Demasiados intentos. Espera un poco y vuelve a intentarlo.", "en": "Too many attempts. Wait a little and try again."},
	"invalid_token":             {"es": "El enlace no es válido, ha caducado o ya se usó. Pide uno nuevo.", "en": "The link is not valid, has expired or was already used. Ask for a new one."},
	"unauthenticated":           {"es": "Tienes que iniciar sesión.", "en": "You need to sign in."},
	"csrf_failed":               {"es": "Petición rechazada (falta el token CSRF). Recarga la página.", "en": "Request refused (missing CSRF token). Reload the page."},
	"forbidden":                 {"es": "Tu rol en esta organización no permite hacer esto.", "en": "Your role in this organization does not allow this."},
	"no_organization":           {"es": "No perteneces a ninguna organización.", "en": "You do not belong to any organization."},
	"last_owner":                {"es": "La organización necesita al menos un propietario: nombra antes a otro.", "en": "The organization needs at least one owner: make someone else owner first."},
	"invalid_domain":            {"es": "No es un nombre de dominio válido (por ejemplo, ejemplo.com).", "en": "That is not a valid domain name (for example, example.com)."},
	"domain_ip":                 {"es": "Una dirección IP no es un dominio: añade el nombre del dominio.", "en": "An IP address is not a domain: add the domain name."},
	"domain_reserved":           {"es": "Ese nombre es de uso reservado o interno y no se puede verificar.", "en": "That name is reserved or internal and cannot be verified."},
	"domain_unknown_tld":        {"es": "La terminación del dominio no existe.", "en": "The domain's ending does not exist."},
	"domain_public_suffix":      {"es": "Es un sufijo público (como .com o .co.uk), no un dominio que se pueda poseer: añade tu dominio completo.", "en": "That is a public suffix (like .com or .co.uk), not a domain anyone owns: add your full domain."},
	"scan_not_allowed":          {"es": "Este dominio está excluido de los análisis públicos.", "en": "This domain is excluded from public scans."},
	"proof_failed":              {"es": "La comprobación anti-abuso ha fallado o ha caducado. Vuelve a intentarlo.", "en": "The anti-abuse check failed or expired. Try again."},
	"domain_rate_limited":       {"es": "Este dominio ya se ha analizado muchas veces hoy. Vuelve a intentarlo mañana.", "en": "This domain has been scanned many times today. Try again tomorrow."},
	"busy":                      {"es": "Hay muchos análisis en curso. Vuelve a intentarlo dentro de un minuto.", "en": "Many scans are running. Try again in a minute."},
	"optout_needs_verification": {"es": "Para bloquear o desbloquear los análisis públicos, el dominio tiene que estar verificado. Verifícalo de nuevo.", "en": "To block or unblock public scans, the domain must be verified. Verify it again."},
	"domain_not_verified":       {"es": "Ese dominio no está verificado en tu organización. Verifícalo (o el dominio del que cuelga) para hacer un análisis completo.", "en": "That domain is not verified in your organization. Verify it (or its parent domain) to run a full scan."},
	"domain_exists":             {"es": "Ese dominio ya está añadido.", "en": "That domain is already added."},
	"dns_record_missing":        {"es": "No encontramos el registro TXT de verificación en los servidores DNS del dominio.", "en": "We did not find the verification TXT record on the domain's DNS servers."},
	"dns_unavailable":           {"es": "Los servidores DNS del dominio no respondieron; lo intentaremos más tarde.", "en": "The domain's DNS servers did not answer; we will try again later."},
	"new_organizations_closed":  {"es": "Durante la beta, las cuentas creadas por invitación no pueden crear organizaciones nuevas.", "en": "During the beta, accounts created by invitation cannot create new organizations."},
	"already_member":            {"es": "Esa dirección ya es miembro de la organización.", "en": "That address is already a member of the organization."},
	"limit_reached":             {"es": "Has llegado al límite permitido. Revoca o borra algo antes de añadir más.", "en": "You have reached the limit. Revoke or remove something before adding more."},
	"invitation_other_email":    {"es": "Esta invitación es para otra dirección de email. Entra con la cuenta de esa dirección.", "en": "This invitation is for another email address. Sign in with that address's account."},
	"invalid_role":              {"es": "El rol no es válido (owner, admin o viewer).", "en": "The role is not valid (owner, admin or viewer)."},
	"wrong_password":            {"es": "La contraseña actual no es correcta.", "en": "The current password is not correct."},

	// Demo requests (POST /api/demo); bad_origin, invalid_name, invalid_email and
	// invalid_domain are the accounts and domains ones above.
	"too_many_requests": {"es": "Demasiadas solicitudes. Espera un poco y vuelve a intentarlo.", "en": "Too many requests. Wait a little and try again."},
	"invalid_proof":     {"es": "No se pudo comprobar que eres una persona. Recarga la página y vuelve a intentarlo.", "en": "We could not check you are a person. Reload the page and try again."},
	"privacy_required":  {"es": "Para enviar la solicitud tienes que aceptar la política de privacidad.", "en": "To send the request you have to accept the privacy policy."},
	"invalid_company":   {"es": "Escribe el nombre de tu empresa (máximo 100 caracteres).", "en": "Enter your company name (100 characters at most)."},
	"invalid_message":   {"es": "El mensaje es demasiado largo (máximo 2000 caracteres).", "en": "The message is too long (2000 characters at most)."},

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
	"queue_timeout":      {"es": "El análisis esperó demasiado en la cola porque había mucha demanda. Vuelve a intentarlo.", "en": "The scan waited too long in the queue because of high demand. Try again."},
	"lease_expired":      {"es": "El escaneo no pudo completarse tras varios intentos.", "en": "The scan could not complete after several attempts."},
}
