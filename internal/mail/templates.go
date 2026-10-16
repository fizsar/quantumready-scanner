// Package mail renders the transactional emails (plain text, ES/EN) and
// sends them over SMTP (cmd/mailer). Emails are rendered by the API when it
// queues them, so the outbox rows are complete and the mailer reads nothing
// else (docs/security.md).
package mail

import (
	"fmt"
	"strings"
	"text/template"

	"github.com/fizsar/quantumready-saas/internal/store"
)

// Templates.
const (
	VerifyEmail     = "verify_email"
	AccountExists   = "account_exists"
	ResetPassword   = "reset_password"
	PasswordChanged = "password_changed"
	Invitation      = "invitation"
)

// Data fills a template. Link is an absolute URL to the site.
type Data struct {
	Link     string
	ValidFor string // e.g. "24 horas" / "24 hours"
	SiteURL  string
	Support  string // support address
	// Invitations.
	Inviter string // inviter's email
	OrgName string
	Role    string // localized role name
}

type text struct{ subject, body string }

var catalog = map[string]map[string]text{
	VerifyEmail: {
		"es": {"Confirma tu email en QuantumReady", `Hola:

Para activar tu cuenta de QuantumReady, confirma tu dirección de email con este enlace:

{{.Link}}

El enlace caduca en {{.ValidFor}} y solo sirve una vez.

Si no has creado tú esta cuenta, ignora este mensaje: sin confirmar, la cuenta se borra en 7 días.

—
QuantumReady · {{.SiteURL}}
Dudas: {{.Support}}`},
		"en": {"Confirm your email for QuantumReady", `Hello,

To activate your QuantumReady account, confirm your email address with this link:

{{.Link}}

The link expires in {{.ValidFor}} and works only once.

If you did not create this account, ignore this message: unconfirmed accounts are deleted after 7 days.

—
QuantumReady · {{.SiteURL}}
Questions: {{.Support}}`},
	},
	AccountExists: {
		"es": {"Ya tienes una cuenta en QuantumReady", `Hola:

Alguien (seguramente tú) ha intentado registrarse en QuantumReady con este email, pero ya tienes una cuenta.

Puedes entrar aquí: {{.SiteURL}}/login
Si no recuerdas la contraseña, puedes cambiarla aquí: {{.SiteURL}}/forgot

Si no has sido tú, no tienes que hacer nada: tu cuenta no ha cambiado.

—
QuantumReady · {{.SiteURL}}
Dudas: {{.Support}}`},
		"en": {"You already have a QuantumReady account", `Hello,

Someone (probably you) tried to sign up for QuantumReady with this email, but you already have an account.

You can sign in here: {{.SiteURL}}/login
If you forgot your password, you can change it here: {{.SiteURL}}/forgot

If it was not you, there is nothing to do: your account has not changed.

—
QuantumReady · {{.SiteURL}}
Questions: {{.Support}}`},
	},
	ResetPassword: {
		"es": {"Cambia tu contraseña de QuantumReady", `Hola:

Hemos recibido una petición para cambiar la contraseña de tu cuenta de QuantumReady. Puedes hacerlo con este enlace:

{{.Link}}

El enlace caduca en {{.ValidFor}} y solo sirve una vez. Al cambiarla se cerrarán todas tus sesiones abiertas.

Si no lo has pedido tú, ignora este mensaje: tu contraseña no cambia.

—
QuantumReady · {{.SiteURL}}
Dudas: {{.Support}}`},
		"en": {"Change your QuantumReady password", `Hello,

We received a request to change the password of your QuantumReady account. You can do it with this link:

{{.Link}}

The link expires in {{.ValidFor}} and works only once. Changing it signs you out everywhere.

If you did not ask for this, ignore this message: your password does not change.

—
QuantumReady · {{.SiteURL}}
Questions: {{.Support}}`},
	},
	PasswordChanged: {
		"es": {"Tu contraseña de QuantumReady ha cambiado", `Hola:

La contraseña de tu cuenta de QuantumReady acaba de cambiar.

Si has sido tú, no tienes que hacer nada.
Si no has sido tú, cámbiala ya desde {{.SiteURL}}/forgot y escríbenos a {{.Support}}.

—
QuantumReady · {{.SiteURL}}`},
		"en": {"Your QuantumReady password has changed", `Hello,

The password of your QuantumReady account has just changed.

If it was you, there is nothing to do.
If it was not you, change it now at {{.SiteURL}}/forgot and write to us at {{.Support}}.

—
QuantumReady · {{.SiteURL}}`},
	},
	Invitation: {
		"es": {"Te han invitado a una organización en QuantumReady", `Hola:

{{.Inviter}} te invita a unirte a «{{.OrgName}}» en QuantumReady con el rol {{.Role}}.

QuantumReady revisa si las webs de una organización están preparadas para la criptografía post-cuántica.

Para aceptar, abre este enlace (si no tienes cuenta, podrás crearla ahí):

{{.Link}}

El enlace caduca en {{.ValidFor}} y solo sirve una vez.

Si no esperabas esta invitación, ignora este mensaje.

—
QuantumReady · {{.SiteURL}}
Dudas: {{.Support}}`},
		"en": {"You have been invited to an organization on QuantumReady", `Hello,

{{.Inviter}} invites you to join "{{.OrgName}}" on QuantumReady as {{.Role}}.

QuantumReady checks whether an organization's websites are ready for post-quantum cryptography.

To accept, open this link (if you have no account, you can create one there):

{{.Link}}

The link expires in {{.ValidFor}} and works only once.

If you were not expecting this invitation, ignore this message.

—
QuantumReady · {{.SiteURL}}
Questions: {{.Support}}`},
	},
}

var parsed = func() map[string]map[string]*template.Template {
	out := map[string]map[string]*template.Template{}
	for name, langs := range catalog {
		out[name] = map[string]*template.Template{}
		for lang, t := range langs {
			out[name][lang] = template.Must(template.New(name + "." + lang).Option("missingkey=error").Parse(t.body))
		}
	}
	return out
}()

// Render returns the email ready for the outbox. Unknown languages fall
// back to Spanish.
func Render(name, lang, to string, d Data) (store.Email, error) {
	langs, ok := catalog[name]
	if !ok {
		return store.Email{}, fmt.Errorf("mail: unknown template %q", name)
	}
	if _, ok := langs[lang]; !ok {
		lang = "es"
	}
	var b strings.Builder
	if err := parsed[name][lang].Execute(&b, d); err != nil {
		return store.Email{}, err
	}
	return store.Email{To: to, Template: name, Lang: lang, Subject: langs[lang].subject, Body: b.String()}, nil
}

// ValidFor renders a duration for the emails ("24 horas", "30 minutos").
func ValidFor(lang string, hours, minutes int) string {
	if lang == "en" {
		if hours > 0 {
			return fmt.Sprintf("%d hours", hours)
		}
		return fmt.Sprintf("%d minutes", minutes)
	}
	if hours > 0 {
		return fmt.Sprintf("%d horas", hours)
	}
	return fmt.Sprintf("%d minutos", minutes)
}
