package scoring

import "strings"

// Languages supported by the catalog. The first one is the fallback.
var Languages = []string{"es", "en"}

// Text is what a user reads about a finding.
type Text struct {
	Title       string `json:"title"`
	Detail      string `json:"detail"`
	Remediation string `json:"remediation,omitempty"`
}

// Describe renders the texts of a finding code in lang, filling {params}.
// Unknown languages fall back to Spanish; unknown codes return the code.
func Describe(code string, params map[string]string, lang string) Text {
	byLang, ok := findingTexts[code]
	if !ok {
		return Text{Title: code}
	}
	t, ok := byLang[lang]
	if !ok {
		t = byLang[Languages[0]]
	}
	return Text{Title: fill(t.Title, params), Detail: fill(t.Detail, params), Remediation: fill(t.Remediation, params)}
}

// Label returns the display name of a block, item, axis, severity, risk
// level or not-evaluated code (keys such as "block.tls13", "noteval.ssl2_ssl3").
func Label(key, lang string) string {
	byLang, ok := labels[key]
	if !ok {
		return key
	}
	if s, ok := byLang[lang]; ok {
		return s
	}
	return byLang[Languages[0]]
}

func fill(s string, params map[string]string) string {
	for k, v := range params {
		s = strings.ReplaceAll(s, "{"+k+"}", v)
	}
	return s
}

type langTexts map[string]Text

var findingTexts = map[string]langTexts{
	// --- Key exchange (harvest now, decrypt later) ---
	"KEX_HYBRID_PREFERRED": {
		"es": {Title: "Intercambio de claves post-cuántico: {group}",
			Detail: "Con la oferta de un navegador actual, el servidor negocia el grupo híbrido {group} (clásico + ML-KEM, FIPS 203). El tráfico que alguien capture hoy seguirá protegido aunque un ordenador cuántico rompa la parte clásica (\"cosechar ahora, descifrar después\")."},
		"en": {Title: "Post-quantum key exchange: {group}",
			Detail: "With a current browser's offer, the server negotiates the hybrid group {group} (classical + ML-KEM, FIPS 203). Traffic captured today stays protected even if a quantum computer breaks the classical part (\"harvest now, decrypt later\")."},
	},
	"KEX_HYBRID_NOT_PREFERRED": {
		"es": {Title: "ML-KEM soportado pero no preferido",
			Detail:      "El servidor acepta {groups} si es lo único que se ofrece, pero cuando un navegador actual ofrece también un grupo clásico, elige el clásico. En la práctica, el tráfico real de los usuarios sigue expuesto a \"cosechar ahora, descifrar después\".",
			Remediation: "Configura el servidor para preferir el grupo híbrido: pon X25519MLKEM768 en primer lugar en la lista de grupos (por ejemplo, ssl_ecdh_curve / Groups en OpenSSL 3.5+, o la lista de curvas del balanceador o CDN)."},
		"en": {Title: "ML-KEM supported but not preferred",
			Detail:      "The server accepts {groups} when it is the only offer, but when a current browser also offers a classical group it picks the classical one. In practice, real user traffic remains exposed to \"harvest now, decrypt later\".",
			Remediation: "Make the server prefer the hybrid group: put X25519MLKEM768 first in its group list (e.g. ssl_ecdh_curve / Groups in OpenSSL 3.5+, or the load balancer or CDN curve list)."},
	},
	"KEX_NO_HYBRID": {
		"es": {Title: "Sin intercambio de claves post-cuántico",
			Detail:      "El servidor no acepta ningún grupo híbrido ML-KEM. El intercambio de claves es solo clásico (ECDHE o RSA), que un ordenador cuántico podrá romper con el algoritmo de Shor: el tráfico capturado hoy podría descifrarse en el futuro (\"cosechar ahora, descifrar después\"). Es el riesgo cuántico más urgente, porque afecta a los datos que ya están viajando.",
			Remediation: "Activa el grupo híbrido X25519MLKEM768 en TLS 1.3: OpenSSL 3.5+, BoringSSL, Go 1.24+, Caddy 2.10+, nginx compilado con OpenSSL 3.5, o la opción post-cuántica de tu CDN o balanceador. Mantén X25519 como alternativa para clientes antiguos."},
		"en": {Title: "No post-quantum key exchange",
			Detail:      "The server accepts no hybrid ML-KEM group. Key exchange is classical only (ECDHE or RSA), which a quantum computer will be able to break with Shor's algorithm: traffic captured today could be decrypted in the future (\"harvest now, decrypt later\"). This is the most urgent quantum risk because it affects data already in transit.",
			Remediation: "Enable the hybrid group X25519MLKEM768 in TLS 1.3: OpenSSL 3.5+, BoringSSL, Go 1.24+, Caddy 2.10+, nginx built with OpenSSL 3.5, or your CDN or load balancer's post-quantum option. Keep X25519 as a fallback for older clients."},
	},
	"KEX_HYBRID_UNKNOWN": {
		"es": {Title: "Intercambio post-cuántico no confirmado",
			Detail:      "Con la oferta de un navegador actual, el servidor no elige un grupo híbrido ML-KEM, y las comprobaciones de soporte no pudieron completarse (timeout o error de red). El tráfico real de los usuarios usa un intercambio clásico.",
			Remediation: "Repite el escaneo. Si se confirma, activa y prioriza X25519MLKEM768 en TLS 1.3."},
		"en": {Title: "Post-quantum exchange not confirmed",
			Detail:      "With a current browser's offer, the server does not pick a hybrid ML-KEM group, and the support checks could not complete (timeout or network error). Real user traffic uses a classical exchange.",
			Remediation: "Scan again. If confirmed, enable and prioritize X25519MLKEM768 in TLS 1.3."},
	},
	"KEX_MULTIPLE_HYBRIDS": {
		"es": {Title: "Varios grupos híbridos disponibles",
			Detail: "El servidor acepta {groups}, lo que da margen a clientes con requisitos distintos (por ejemplo, entornos FIPS que exigen curvas NIST)."},
		"en": {Title: "Several hybrid groups available",
			Detail: "The server accepts {groups}, which accommodates clients with different requirements (e.g. FIPS environments that require NIST curves)."},
	},
	"KEX_SINGLE_HYBRID": {
		"es": {Title: "Un solo grupo híbrido",
			Detail:      "Solo se acepta {group}. Es el grupo que usan los navegadores, así que el riesgo es bajo, pero clientes que exijan curvas NIST (entornos FIPS) no podrán negociar un intercambio post-cuántico.",
			Remediation: "Si tienes clientes en entornos regulados, añade SecP256r1MLKEM768 a la lista de grupos. Depende de que el software del servidor lo permita: OpenSSL 3.5+ (y nginx o Apache compilados con él) lo acepta en la lista de grupos (Groups / ssl_ecdh_curve); Caddy 2.11 aún no admite ese nombre en su directiva curves; en un CDN o balanceador, depende de su configuración post-cuántica."},
		"en": {Title: "A single hybrid group",
			Detail:      "Only {group} is accepted. It is the group browsers use, so the risk is low, but clients that require NIST curves (FIPS environments) cannot negotiate a post-quantum exchange.",
			Remediation: "If you serve clients in regulated environments, add SecP256r1MLKEM768 to the group list. It depends on the server software: OpenSSL 3.5+ (and nginx or Apache built with it) accepts it in the group list (Groups / ssl_ecdh_curve); Caddy 2.11 does not accept that name in its curves directive yet; on a CDN or load balancer, it depends on its post-quantum settings."},
	},

	// --- Protocol ---
	"TLS13_SUPPORTED": {
		"es": {Title: "TLS 1.3 disponible", Detail: "TLS 1.3 es la versión actual del protocolo y la única en la que existen los grupos híbridos post-cuánticos."},
		"en": {Title: "TLS 1.3 available", Detail: "TLS 1.3 is the current protocol version and the only one where hybrid post-quantum groups exist."},
	},
	"TLS13_MISSING": {
		"es": {Title: "Sin TLS 1.3 no es posible adoptar intercambio de claves híbrido ML-KEM",
			Detail:      "El servidor solo acepta versiones anteriores a TLS 1.3. Hoy puede ser seguro, pero los grupos híbridos ML-KEM solo existen en TLS 1.3: mientras no se active, el tráfico seguirá expuesto a \"cosechar ahora, descifrar después\" y no hay forma de corregirlo con configuración de grupos.",
			Remediation: "Activa TLS 1.3 (lo soporta cualquier servidor con OpenSSL 1.1.1 o posterior, y todos los navegadores actuales) y después el grupo X25519MLKEM768. Mantén TLS 1.2 para clientes antiguos."},
		"en": {Title: "Without TLS 1.3, hybrid ML-KEM key exchange cannot be adopted",
			Detail:      "The server only accepts versions older than TLS 1.3. That may be secure today, but hybrid ML-KEM groups only exist in TLS 1.3: until it is enabled, traffic stays exposed to \"harvest now, decrypt later\" and no group configuration can fix it.",
			Remediation: "Enable TLS 1.3 (any server with OpenSSL 1.1.1 or later supports it, as do all current browsers), then the X25519MLKEM768 group. Keep TLS 1.2 for older clients."},
	},
	"PROTO_LEGACY_TLS": {
		"es": {Title: "Versiones obsoletas activas: {versions}",
			Detail:      "TLS 1.0 y 1.1 están retirados (RFC 8996): solo admiten construcciones débiles y los navegadores ya no los usan. Mantenerlos activos amplía la superficie de ataque sin beneficio.",
			Remediation: "Desactiva TLS 1.0 y 1.1 y deja solo TLS 1.2 y 1.3."},
		"en": {Title: "Obsolete versions enabled: {versions}",
			Detail:      "TLS 1.0 and 1.1 are deprecated (RFC 8996): they only allow weak constructions and browsers no longer use them. Keeping them enabled widens the attack surface for no benefit.",
			Remediation: "Disable TLS 1.0 and 1.1 and keep only TLS 1.2 and 1.3."},
	},

	// --- Cipher suites ---
	"FS_ALL": {
		"es": {Title: "Secreto hacia adelante (forward secrecy)", Detail: "Todas las conexiones usan un intercambio de claves efímero: aunque la clave privada del servidor se filtrara, no permitiría descifrar conexiones pasadas."},
		"en": {Title: "Forward secrecy", Detail: "Every connection uses an ephemeral key exchange: even if the server's private key leaked, it would not decrypt past connections."},
	},
	"FS_PARTIAL": {
		"es": {Title: "Suites sin secreto hacia adelante (forward secrecy)",
			Detail:      "Se aceptan suites sin intercambio de claves efímero, con RSA o DH estático ({suites}). Quien obtenga la clave privada del servidor podría descifrar el tráfico capturado con ellas.",
			Remediation: "Elimina las suites TLS_RSA_WITH_* y deja solo las ECDHE."},
		"en": {Title: "Suites without forward secrecy",
			Detail:      "Suites without an ephemeral key exchange, with RSA or static DH, are accepted ({suites}). Whoever obtains the server's private key could decrypt traffic captured with them.",
			Remediation: "Remove the TLS_RSA_WITH_* suites and keep only ECDHE ones."},
	},
	"FS_NONE": {
		"es": {Title: "Ninguna suite con secreto hacia adelante (forward secrecy)",
			Detail:      "Ninguna suite aceptada usa un intercambio de claves efímero; todas usan RSA o DH estático ({suites}): la filtración de la clave privada del servidor expondría todo el tráfico pasado. Los navegadores actuales ya no ofrecen estas suites.",
			Remediation: "Configura suites ECDHE con AES-GCM o ChaCha20-Poly1305 y activa TLS 1.3."},
		"en": {Title: "No suite with forward secrecy",
			Detail:      "No accepted suite uses an ephemeral key exchange; they all use RSA or static DH ({suites}): a leak of the server's private key would expose all past traffic. Current browsers no longer offer these suites.",
			Remediation: "Configure ECDHE suites with AES-GCM or ChaCha20-Poly1305 and enable TLS 1.3."},
	},
	"SUITE_RC4": {
		"es": {Title: "Cifrado RC4 aceptado",
			Detail:      "RC4 tiene sesgos estadísticos explotables y está prohibido en TLS (RFC 7465). Suites: {suites}.",
			Remediation: "Elimina todas las suites RC4."},
		"en": {Title: "RC4 cipher accepted",
			Detail:      "RC4 has exploitable statistical biases and is prohibited in TLS (RFC 7465). Suites: {suites}.",
			Remediation: "Remove every RC4 suite."},
	},
	"SUITE_3DES": {
		"es": {Title: "Cifrado 3DES aceptado",
			Detail:      "3DES usa bloques de 64 bits y es vulnerable a Sweet32 en conexiones largas. Suites: {suites}.",
			Remediation: "Elimina las suites 3DES y usa AES-GCM o ChaCha20-Poly1305."},
		"en": {Title: "3DES cipher accepted",
			Detail:      "3DES uses 64-bit blocks and is vulnerable to Sweet32 on long connections. Suites: {suites}.",
			Remediation: "Remove the 3DES suites and use AES-GCM or ChaCha20-Poly1305."},
	},
	"SUITE_CBC": {
		"es": {Title: "Suites en modo CBC",
			Detail:      "El modo CBC de TLS 1.2 ha sufrido ataques de oráculo de relleno (Lucky13 y similares) y depende de implementaciones cuidadosas. Suites: {suites}.",
			Remediation: "Prefiere suites AEAD (AES-GCM, ChaCha20-Poly1305) y elimina las CBC si tus clientes lo permiten."},
		"en": {Title: "CBC-mode suites",
			Detail:      "TLS 1.2 CBC mode has suffered padding-oracle attacks (Lucky13 and the like) and depends on careful implementations. Suites: {suites}.",
			Remediation: "Prefer AEAD suites (AES-GCM, ChaCha20-Poly1305) and remove CBC ones if your clients allow it."},
	},
	// --- Found by the raw ClientHello probes (intensity full, model 2026.2) ---
	"PROTO_SSL2": {
		"es": {Title: "SSLv2 activo",
			Detail:      "El servidor todavía acepta SSLv2, un protocolo de 1995 roto por diseño y prohibido desde 2011 (RFC 6176). Además, tener SSLv2 activo permite el ataque DROWN (CVE-2016-0800), que puede descifrar conexiones TLS modernas que usen la misma clave del servidor. Cifrados: {ciphers}.",
			Remediation: "Desactiva SSLv2 por completo (en OpenSSL: \"-SSLv2\" o actualiza a una versión que ya no lo incluye) y comprueba que ningún otro servicio con la misma clave lo tenga activo."},
		"en": {Title: "SSLv2 enabled",
			Detail:      "The server still accepts SSLv2, a 1995 protocol broken by design and prohibited since 2011 (RFC 6176). Having SSLv2 enabled also allows the DROWN attack (CVE-2016-0800), which can decrypt modern TLS connections that use the same server key. Ciphers: {ciphers}.",
			Remediation: "Disable SSLv2 entirely (in OpenSSL: \"-SSLv2\", or upgrade to a release that no longer includes it) and check that no other service using the same key has it enabled."},
	},
	"PROTO_SSL3": {
		"es": {Title: "SSLv3 activo",
			Detail:      "El servidor todavía acepta SSLv3, prohibido desde 2015 (RFC 7568) por el ataque POODLE (CVE-2014-3566), que recupera datos cifrados como las cookies de sesión. Ningún navegador actual lo usa, así que solo sirve a un atacante que fuerce esa versión.",
			Remediation: "Desactiva SSLv3 (en nginx: ssl_protocols TLSv1.2 TLSv1.3; en Apache: SSLProtocol -all +TLSv1.2 +TLSv1.3)."},
		"en": {Title: "SSLv3 enabled",
			Detail:      "The server still accepts SSLv3, prohibited since 2015 (RFC 7568) because of the POODLE attack (CVE-2014-3566), which recovers encrypted data such as session cookies. No current browser uses it, so it only helps an attacker who forces that version.",
			Remediation: "Disable SSLv3 (nginx: ssl_protocols TLSv1.2 TLSv1.3; Apache: SSLProtocol -all +TLSv1.2 +TLSv1.3)."},
	},
	"SUITE_NULL": {
		"es": {Title: "Conexiones sin cifrar aceptadas (suites NULL)",
			Detail:      "El servidor acepta suites que no cifran nada: lo que viaje por esas conexiones se puede leer tal cual. Un atacante en la red puede forzarlas si el cliente las admite. Suites: {suites}.",
			Remediation: "Elimina las suites NULL de la configuración (en OpenSSL, añade \"!eNULL\" a la lista de cifrados)."},
		"en": {Title: "Unencrypted connections accepted (NULL suites)",
			Detail:      "The server accepts suites that encrypt nothing: whatever travels over them can be read as is. An attacker on the network can force them if the client allows them. Suites: {suites}.",
			Remediation: "Remove the NULL suites from the configuration (in OpenSSL, add \"!eNULL\" to the cipher list)."},
	},
	"SUITE_INTEGRITY_ONLY": {
		"es": {Title: "TLS 1.3 sin cifrado (suites solo de integridad)",
			Detail:      "El servidor acepta suites de TLS 1.3 que autentican pero no cifran (RFC 9150). Existen para redes industriales y de control (IoT, sistemas SCADA) donde los datos no son secretos y se necesita poder inspeccionarlos, pero en una web pública dejan el tráfico a la vista de cualquiera en el camino. Suites: {suites}.",
			Remediation: "Desactívalas en el servidor web: deja solo TLS_AES_128_GCM_SHA256, TLS_AES_256_GCM_SHA384 y TLS_CHACHA20_POLY1305_SHA256."},
		"en": {Title: "TLS 1.3 without encryption (integrity-only suites)",
			Detail:      "The server accepts TLS 1.3 suites that authenticate but do not encrypt (RFC 9150). They exist for industrial and control networks (IoT, SCADA) where data is not secret and must be inspectable, but on a public website they leave traffic readable by anyone on the path. Suites: {suites}.",
			Remediation: "Disable them on the web server: keep only TLS_AES_128_GCM_SHA256, TLS_AES_256_GCM_SHA384 and TLS_CHACHA20_POLY1305_SHA256."},
	},
	"SUITE_EXPORT": {
		"es": {Title: "Suites EXPORT aceptadas",
			Detail:      "Las suites EXPORT usan claves de 40 o 56 bits, debilitadas a propósito en los años 90 por las restricciones de exportación de EE. UU. Hoy se rompen en minutos y permiten los ataques FREAK (CVE-2015-0204) y Logjam (CVE-2015-4000). Suites: {suites}.",
			Remediation: "Elimínalas (en OpenSSL, \"!EXPORT\") o actualiza el software: las versiones actuales ya no las incluyen."},
		"en": {Title: "EXPORT suites accepted",
			Detail:      "EXPORT suites use 40 or 56-bit keys, deliberately weakened in the 1990s by US export rules. Today they break in minutes and enable the FREAK (CVE-2015-0204) and Logjam (CVE-2015-4000) attacks. Suites: {suites}.",
			Remediation: "Remove them (in OpenSSL, \"!EXPORT\") or upgrade the software: current releases no longer include them."},
	},
	"SUITE_ANON": {
		"es": {Title: "Suites anónimas aceptadas (sin autenticar al servidor)",
			Detail:      "Con estas suites el servidor no demuestra su identidad: no hay certificado. Cualquiera en la red puede hacerse pasar por él y leer o modificar el tráfico (ataque de intermediario). Suites: {suites}.",
			Remediation: "Elimínalas (en OpenSSL, \"!aNULL\")."},
		"en": {Title: "Anonymous suites accepted (server not authenticated)",
			Detail:      "With these suites the server does not prove its identity: there is no certificate. Anyone on the network can impersonate it and read or change the traffic (man in the middle). Suites: {suites}.",
			Remediation: "Remove them (in OpenSSL, \"!aNULL\")."},
	},
	"SUITE_DES": {
		"es": {Title: "Cifrado DES (56 bits) aceptado",
			Detail:      "DES simple usa claves de 56 bits, que se rompen por fuerza bruta desde 1998. Suites: {suites}.",
			Remediation: "Elimina las suites DES (en OpenSSL, \"!DES:!LOW\") y usa AES-GCM o ChaCha20-Poly1305."},
		"en": {Title: "DES cipher (56-bit) accepted",
			Detail:      "Single DES uses 56-bit keys, which have been brute-forced since 1998. Suites: {suites}.",
			Remediation: "Remove the DES suites (in OpenSSL, \"!DES:!LOW\") and use AES-GCM or ChaCha20-Poly1305."},
	},
	"SUITE_CCM8": {
		"es": {Title: "Suites CCM_8 (etiqueta de 64 bits)",
			Detail:      "CCM_8 es un cifrado autenticado con una etiqueta de integridad de 64 bits en lugar de 128, pensado para dispositivos muy limitados. No está roto, pero da menos margen que AES-GCM y no tiene sentido en una web. Suites: {suites}.",
			Remediation: "Desactívalas si no las necesita ningún cliente embebido."},
		"en": {Title: "CCM_8 suites (64-bit tag)",
			Detail:      "CCM_8 is authenticated encryption with a 64-bit integrity tag instead of 128, meant for very constrained devices. It is not broken, but it leaves less margin than AES-GCM and makes no sense on a website. Suites: {suites}.",
			Remediation: "Disable them unless an embedded client needs them."},
	},
	"SUITE_MODERN_AEAD": {
		"es": {Title: "Suites AEAD modernas: {suites}",
			Detail: "AEGIS y Ascon son cifrados autenticados modernos (Ascon es el estándar ligero del NIST, SP 800-232). Informativo: no suma ni resta."},
		"en": {Title: "Modern AEAD suites: {suites}",
			Detail: "AEGIS and Ascon are modern authenticated ciphers (Ascon is NIST's lightweight standard, SP 800-232). Informational: never scored."},
	},
	"TLS13_SUITES_AEAD": {
		"es": {Title: "Suites de TLS 1.3 correctas",
			Detail: "Todas las suites de TLS 1.3 aceptadas usan cifrado autenticado con etiqueta completa: {suites}."},
		"en": {Title: "Sound TLS 1.3 suites",
			Detail: "Every TLS 1.3 suite accepted uses authenticated encryption with a full-size tag: {suites}."},
	},
	"DH_WEAK": {
		"es": {Title: "Diffie-Hellman demasiado débil ({bits} bits)",
			Detail:      "Las suites DHE usan un grupo de menos de 1024 bits. Con el ataque Logjam (CVE-2015-4000) se puede romper en tiempo real y descifrar o manipular la conexión.",
			Remediation: "Configura un grupo DH de 2048 bits o más (por ejemplo ffdhe2048 o ffdhe3072, RFC 7919) o retira las suites DHE y deja solo ECDHE."},
		"en": {Title: "Diffie-Hellman far too weak ({bits} bits)",
			Detail:      "The DHE suites use a group under 1024 bits. The Logjam attack (CVE-2015-4000) can break it in real time and decrypt or tamper with the connection.",
			Remediation: "Configure a DH group of 2048 bits or more (e.g. ffdhe2048 or ffdhe3072, RFC 7919), or drop the DHE suites and keep ECDHE only."},
	},
	"DH_SMALL": {
		"es": {Title: "Grupo Diffie-Hellman pequeño ({bits} bits)",
			Detail:      "Las suites DHE usan un grupo de menos de 2048 bits. El estudio de Logjam (2015) estimó que un actor estatal puede romper grupos de 1024 bits muy usados; NIST pide 2048 bits o más.",
			Remediation: "Configura un grupo DH de 2048 bits o más (ffdhe2048 o ffdhe3072, RFC 7919) o retira las suites DHE y deja solo ECDHE."},
		"en": {Title: "Small Diffie-Hellman group ({bits} bits)",
			Detail:      "The DHE suites use a group under 2048 bits. The Logjam study (2015) estimated that a nation-state can break widely used 1024-bit groups; NIST requires 2048 bits or more.",
			Remediation: "Configure a DH group of 2048 bits or more (ffdhe2048 or ffdhe3072, RFC 7919), or drop the DHE suites and keep ECDHE only."},
	},
	"DH_OK": {
		"es": {Title: "Grupo Diffie-Hellman suficiente ({bits} bits)",
			Detail: "Las suites DHE usan grupos de 2048 bits o más."},
		"en": {Title: "Adequate Diffie-Hellman group ({bits} bits)",
			Detail: "The DHE suites use groups of 2048 bits or more."},
	},
	"GROUP_WEAK_CURVES": {
		"es": {Title: "Curvas elípticas débiles aceptadas: {groups}",
			Detail:      "El servidor acepta curvas de menos de 256 bits o desaconsejadas (RFC 8422, RFC 9847). Los navegadores no las ofrecen, así que solo afectan a clientes antiguos o mal configurados; por eso no limitan la nota.",
			Remediation: "Deja solo x25519, secp256r1 y secp384r1 (y los grupos híbridos ML-KEM)."},
		"en": {Title: "Weak elliptic curves accepted: {groups}",
			Detail:      "The server accepts curves under 256 bits or deprecated ones (RFC 8422, RFC 9847). Browsers never offer them, so they only affect old or misconfigured clients; that is why they do not cap the grade.",
			Remediation: "Keep only x25519, secp256r1 and secp384r1 (and the hybrid ML-KEM groups)."},
	},
	"GROUPS_EXTRA": {
		"es": {Title: "Otros grupos aceptados: {groups}",
			Detail: "Grupos clásicos válidos pero poco habituales (ffdhe, brainpool, x448…). Informativo: no suma ni resta."},
		"en": {Title: "Other groups accepted: {groups}",
			Detail: "Valid but uncommon classical groups (ffdhe, brainpool, x448…). Informational: never scored."},
	},
	"KEX_PURE_MLKEM": {
		"es": {Title: "ML-KEM puro aceptado: {groups}",
			Detail: "El servidor acepta ML-KEM sin combinarlo con un algoritmo clásico (FIPS 203). La NSA lo prefiere así para sistemas de seguridad nacional (CNSA 2.0), pero los navegadores usan los híbridos, así que cuenta en la variedad de grupos post-cuánticos y no en el intercambio de claves."},
		"en": {Title: "Pure ML-KEM accepted: {groups}",
			Detail: "The server accepts ML-KEM without a classical component (FIPS 203). The NSA prefers it that way for national security systems (CNSA 2.0), but browsers use the hybrids, so it counts towards post-quantum group variety, not towards key exchange."},
	},
	"KEX_MULTIPLE_PQC": {
		"es": {Title: "Varios grupos post-cuánticos estándar: {groups}",
			Detail: "Además del grupo que usan los navegadores, el servidor acepta otros grupos ML-KEM estándar, útiles para clientes con otros requisitos (FIPS, CNSA 2.0)."},
		"en": {Title: "Several standard post-quantum groups: {groups}",
			Detail: "Besides the group browsers use, the server accepts other standard ML-KEM groups, useful for clients with other requirements (FIPS, CNSA 2.0)."},
	},
	"KEX_EXPERIMENTAL_PQC_ONLY": {
		"es": {Title: "Solo PQC experimental: {groups}",
			Detail:      "El servidor acepta una versión experimental, anterior al estándar, del cifrado post-cuántico (Kyber, 2023–2024), pero no el estándar ML-KEM. Los navegadores ya no la ofrecen (Chrome la retiró a finales de 2024), así que hoy no protege a nadie: alguien empezó la migración post-cuántica y se quedó a medias.",
			Remediation: "Actualiza el software del servidor (OpenSSL 3.5+, Go 1.24+, la versión actual de tu CDN o balanceador) para usar X25519MLKEM768 y retira el grupo experimental."},
		"en": {Title: "Experimental PQC only: {groups}",
			Detail:      "The server accepts a pre-standard, experimental version of post-quantum encryption (Kyber, 2023–2024), but not the ML-KEM standard. Browsers no longer offer it (Chrome dropped it in late 2024), so today it protects nobody: someone started the post-quantum migration and stopped halfway.",
			Remediation: "Upgrade the server software (OpenSSL 3.5+, Go 1.24+, your CDN or load balancer's current release) to use X25519MLKEM768, and remove the experimental group."},
	},
	"KEX_EXPERIMENTAL_PQC_EXTRA": {
		"es": {Title: "Grupo PQC experimental todavía activo: {groups}",
			Detail:      "Además del estándar ML-KEM, el servidor acepta un grupo experimental anterior. No es un riesgo, pero ya no lo usa nadie.",
			Remediation: "Retíralo cuando actualices la configuración."},
		"en": {Title: "Experimental PQC group still enabled: {groups}",
			Detail:      "Besides the ML-KEM standard, the server accepts an earlier experimental group. It is not a risk, but nobody uses it any more.",
			Remediation: "Remove it next time you update the configuration."},
	},
	"SYM_AES128": {
		"es": {Title: "AES-128 y el algoritmo de Grover",
			Detail: "La conexión usa {suite}. En teoría, el algoritmo de Grover reduce la seguridad de AES-128 a unos 64 bits, pero no es paralelizable de forma práctica y NIST sigue considerando AES-128 adecuado. Informativo: no afecta a la nota."},
		"en": {Title: "AES-128 and Grover's algorithm",
			Detail: "The connection uses {suite}. In theory Grover's algorithm reduces AES-128 to about 64-bit security, but it does not parallelize in practice and NIST still considers AES-128 adequate. Informational: it does not affect the grade."},
	},

	// --- Certificate ---
	"CERT_VALID": {
		"es": {Title: "Certificado válido", Detail: "La cadena es de confianza, coincide con el nombre y está vigente (emisor: {issuer}; caduca en {days} días)."},
		"en": {Title: "Valid certificate", Detail: "The chain is trusted, matches the name and is current (issuer: {issuer}; expires in {days} days)."},
	},
	"CERT_EXPIRED": {
		"es": {Title: "Certificado caducado", Detail: "El certificado caducó el {date}. Los navegadores muestran un error y no se conectan.", Remediation: "Renueva el certificado y automatiza la renovación (ACME)."},
		"en": {Title: "Expired certificate", Detail: "The certificate expired on {date}. Browsers show an error and do not connect.", Remediation: "Renew the certificate and automate renewal (ACME)."},
	},
	"CERT_NOT_YET_VALID": {
		"es": {Title: "Certificado aún no válido", Detail: "El certificado no es válido hasta el {date}.", Remediation: "Comprueba la fecha del servidor y el certificado instalado."},
		"en": {Title: "Certificate not yet valid", Detail: "The certificate is not valid until {date}.", Remediation: "Check the server clock and the installed certificate."},
	},
	"CERT_UNTRUSTED": {
		"es": {Title: "Cadena de certificados no válida", Detail: "La cadena no lleva a una autoridad de confianza: {error}.", Remediation: "Instala la cadena completa (certificado + intermedios) emitida por una autoridad pública."},
		"en": {Title: "Invalid certificate chain", Detail: "The chain does not lead to a trusted authority: {error}.", Remediation: "Install the full chain (certificate + intermediates) issued by a public authority."},
	},
	"CERT_SELF_SIGNED": {
		"es": {Title: "Certificado autofirmado", Detail: "El certificado está firmado por sí mismo, así que ningún navegador confía en él.", Remediation: "Usa un certificado de una autoridad pública (por ejemplo, Let's Encrypt con ACME)."},
		"en": {Title: "Self-signed certificate", Detail: "The certificate is signed by itself, so no browser trusts it.", Remediation: "Use a certificate from a public authority (e.g. Let's Encrypt via ACME)."},
	},
	"CERT_HOSTNAME_MISMATCH": {
		"es": {Title: "El certificado no corresponde al dominio", Detail: "El certificado no cubre {host}. Nombres incluidos: {names}.", Remediation: "Emite un certificado que incluya {host}."},
		"en": {Title: "Certificate does not match the domain", Detail: "The certificate does not cover {host}. Names included: {names}.", Remediation: "Issue a certificate that includes {host}."},
	},
	"CERT_EXPIRES_SOON": {
		"es": {Title: "El certificado caduca pronto", Detail: "Caduca en {days} días.", Remediation: "Comprueba que la renovación automática funciona."},
		"en": {Title: "Certificate expires soon", Detail: "It expires in {days} days.", Remediation: "Check that automatic renewal works."},
	},
	"CERT_WEAK_KEY": {
		"es": {Title: "Clave del certificado débil: {key}", Detail: "Las claves RSA de menos de 2048 bits (y ECDSA de menos de 256) se consideran rompibles hoy con medios clásicos.", Remediation: "Emite un certificado con ECDSA P-256 o RSA de al menos 2048 bits (mejor 3072)."},
		"en": {Title: "Weak certificate key: {key}", Detail: "RSA keys under 2048 bits (and ECDSA under 256) are considered breakable today with classical means.", Remediation: "Issue a certificate with ECDSA P-256 or RSA of at least 2048 bits (preferably 3072)."},
	},
	"CERT_WEAK_SIGNATURE": {
		"es": {Title: "Firma del certificado débil: {algorithm}", Detail: "SHA-1 y MD5 tienen colisiones prácticas: una firma con ellos puede falsificarse.", Remediation: "Emite un certificado firmado con SHA-256 o superior."},
		"en": {Title: "Weak certificate signature: {algorithm}", Detail: "SHA-1 and MD5 have practical collisions: a signature using them can be forged.", Remediation: "Issue a certificate signed with SHA-256 or stronger."},
	},
	"CERT_CLASSICAL_SIGNATURE": {
		"es": {Title: "Firma clásica ({key}): riesgo cuántico futuro",
			Detail:      "La identidad del servidor se demuestra con {key}, que un ordenador cuántico podrá falsificar. No es \"cosechar ahora, descifrar después\": una firma solo tiene que resistir durante el handshake, así que el riesgo empieza el día que exista ese ordenador, no antes. Hoy no hay certificados post-cuánticos (ML-DSA) en la web pública. No afecta a la nota.",
			Remediation: "Nada que hacer todavía. Mantén la renovación automatizada para poder cambiar a certificados ML-DSA o híbridos en cuanto las autoridades los emitan."},
		"en": {Title: "Classical signature ({key}): future quantum risk",
			Detail:      "The server proves its identity with {key}, which a quantum computer will be able to forge. This is not \"harvest now, decrypt later\": a signature only has to hold during the handshake, so the risk starts the day such a computer exists, not before. There are no post-quantum (ML-DSA) certificates on the public web yet. It does not affect the grade.",
			Remediation: "Nothing to do yet. Keep renewal automated so you can switch to ML-DSA or hybrid certificates as soon as authorities issue them."},
	},
	"CERT_AUTOMATED_SHORT": {
		"es": {Title: "Certificado de vida corta ({days} días)", Detail: "Una validez tan corta solo es viable con renovación automatizada, que es lo que permitirá cambiar a certificados post-cuánticos sin esfuerzo."},
		"en": {Title: "Short-lived certificate ({days} days)", Detail: "Such a short validity is only workable with automated renewal, which is what will make the switch to post-quantum certificates effortless."},
	},
	"CERT_AUTOMATED_ACME": {
		"es": {Title: "Renovación automatizada ({issuer})", Detail: "El certificado lo emite {issuer}, una autoridad que funciona con ACME (renovación automática; validez de {days} días). Cambiar a certificados post-cuánticos será, previsiblemente, una actualización más."},
		"en": {Title: "Automated renewal ({issuer})", Detail: "The certificate is issued by {issuer}, an authority that works through ACME (automatic renewal; {days}-day validity). Switching to post-quantum certificates should be just another update."},
	},
	"CERT_NOT_AUTOMATED": {
		"es": {Title: "Sin señales de renovación automatizada",
			Detail:      "Validez de {days} días, emitido por {issuer}. La validez máxima baja a 200 días (marzo de 2026), 100 (2027) y 47 (2029), y el cambio a certificados post-cuánticos exigirá reemitirlos: sin automatización, cada cambio es trabajo manual y riesgo de caducidad.",
			Remediation: "Automatiza la emisión y renovación con ACME (certbot, Caddy, cert-manager…), con tu autoridad actual si lo soporta o con una autoridad gratuita."},
		"en": {Title: "No sign of automated renewal",
			Detail:      "{days}-day validity, issued by {issuer}. Maximum validity drops to 200 days (March 2026), 100 (2027) and 47 (2029), and moving to post-quantum certificates will require reissuing them: without automation, every change is manual work and an expiry risk.",
			Remediation: "Automate issuance and renewal with ACME (certbot, Caddy, cert-manager…), with your current authority if it supports it or a free one."},
	},
	"CERT_CT": {
		"es": {Title: "Transparencia de certificados", Detail: "El certificado aporta SCT: {embedded} incrustados por la autoridad y {handshake} entregados en el handshake. Informativo."},
		"en": {Title: "Certificate Transparency", Detail: "The certificate carries SCTs: {embedded} embedded by the authority and {handshake} delivered in the handshake. Informational."},
	},
	"CERT_CT_MISSING": {
		"es": {Title: "Sin SCT de transparencia", Detail: "No se han encontrado SCT (ni incrustados ni en el handshake). Los navegadores los exigen para certificados públicos, así que suele indicar un certificado privado. Informativo."},
		"en": {Title: "No transparency SCTs", Detail: "No SCTs were found (neither embedded nor in the handshake). Browsers require them for public certificates, so this usually means a private certificate. Informational."},
	},

	// --- Scan coverage ---
	"CERT_MISSING": {
		"es": {Title: "Certificado no obtenido", Detail: "Ningún handshake llegó a recibir el certificado, así que no se ha podido evaluar."},
		"en": {Title: "Certificate not obtained", Detail: "No handshake received the certificate, so it could not be evaluated."},
	},
	"SCAN_PARTIAL": {
		"es": {Title: "Escaneo incompleto", Detail: "{count} comprobaciones no pudieron completarse (timeout o error de red). No se han contado como \"no soportado\"; repite el escaneo para confirmarlas."},
		"en": {Title: "Incomplete scan", Detail: "{count} checks could not complete (timeout or network error). They were not counted as \"not supported\"; scan again to confirm them."},
	},
}

var labels = map[string]map[string]string{
	"block.key_exchange":    {"es": "Intercambio de claves post-cuántico", "en": "Post-quantum key exchange"},
	"block.tls13":           {"es": "TLS 1.3", "en": "TLS 1.3"},
	"block.forward_secrecy": {"es": "Secreto hacia adelante (forward secrecy)", "en": "Forward secrecy"},
	"block.hybrid_variety":  {"es": "Variedad de grupos híbridos", "en": "Hybrid group variety"},
	"block.certificate":     {"es": "Certificado y agilidad", "en": "Certificate and agility"},
	"item.valid":            {"es": "Válido (cadena, nombre, fechas)", "en": "Valid (chain, name, dates)"},
	"item.key":              {"es": "Clave", "en": "Key"},
	"item.signature":        {"es": "Firma", "en": "Signature"},
	"item.automation":       {"es": "Renovación automatizada", "en": "Automated renewal"},
	"adjust.floor":          {"es": "Suelo: nada roto (mínimo D)", "en": "Floor: nothing broken (minimum D)"},

	"axis.quantum": {"es": "Riesgo cuántico", "en": "Quantum risk"},
	"axis.current": {"es": "Riesgo actual", "en": "Current risk"},

	"severity.critical": {"es": "Crítico", "en": "Critical"},
	"severity.high":     {"es": "Alto", "en": "High"},
	"severity.medium":   {"es": "Medio", "en": "Medium"},
	"severity.low":      {"es": "Bajo", "en": "Low"},
	"severity.info":     {"es": "Informativo", "en": "Info"},
	"severity.pass":     {"es": "Correcto", "en": "Pass"},
	"severity.none":     {"es": "Ninguno", "en": "None"},

	"noteval.ssl2_ssl3":                                  {"es": "SSLv2 y SSLv3", "en": "SSLv2 and SSLv3"},
	"noteval.tls12_dhe_suites":                           {"es": "Suites DHE de TLS 1.2", "en": "TLS 1.2 DHE suites"},
	"noteval.tls12_ccm_camellia_aria_null_export_suites": {"es": "Suites CCM, Camellia, ARIA, NULL y EXPORT", "en": "CCM, Camellia, ARIA, NULL and EXPORT suites"},
	"noteval.tls13_suite_list":                           {"es": "Lista completa de suites de TLS 1.3", "en": "Full TLS 1.3 suite list"},
	"noteval.tls12_server_cipher_preference":             {"es": "Orden de preferencia de suites del servidor", "en": "Server cipher suite preference"},
	"noteval.legacy_groups":                              {"es": "Grupos antiguos (ffdhe, brainpool…)", "en": "Legacy groups (ffdhe, brainpool…)"},
	"noteval.non_standard_pqc_groups":                    {"es": "Grupos post-cuánticos no estándar", "en": "Non-standard post-quantum groups"},
	"noteval.credential_suites":                          {"es": "Suites PSK, SRP, Kerberos y GOST (requieren credenciales o algoritmos que un escáner no tiene)", "en": "PSK, SRP, Kerberos and GOST suites (they need credentials or algorithms a scanner does not have)"},
	"noteval.implementation_vulnerabilities":             {"es": "Vulnerabilidades de implementación (Heartbleed, ROBOT…)", "en": "Implementation vulnerabilities (Heartbleed, ROBOT…)"},
}
