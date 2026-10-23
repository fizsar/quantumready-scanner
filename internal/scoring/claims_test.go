package scoring

import "testing"

// claim returns the pattern s matches, if any.
func claim(s string) string {
	for _, re := range ForbiddenClaims {
		if re.MatchString(s) {
			return re.String()
		}
	}
	return ""
}

// Every finding text and label, in every language.
func TestReportsMakeNoForbiddenClaims(t *testing.T) {
	for code, byLang := range findingTexts {
		for lang, tx := range byLang {
			for _, s := range []string{tx.Title, tx.Detail, tx.Remediation} {
				if re := claim(s); re != "" {
					t.Errorf("%s (%s) matches %s: %q", code, lang, re, s)
				}
			}
		}
	}
	for key, byLang := range labels {
		for lang, s := range byLang {
			if re := claim(s); re != "" {
				t.Errorf("label %s (%s) matches %s: %q", key, lang, re, s)
			}
		}
	}
}

func TestForbiddenClaimPatterns(t *testing.T) {
	for _, s := range []string{
		"Herramienta oficial del CCN", "Official report", "Informe certificado ENS", "certificado conforme al ENS",
		"ENS-certified scan", "Tu web cumple el ENS", "cumple con el ENS", "Your site complies with the ENS", "ENS compliant",
		"NIS2 exige criptografía post-cuántica", "NIS 2 requires post-quantum key exchange",
		"La criptografía post-cuántica es obligatoria según NIS2", "PQC is mandatory under NIS2",
	} {
		if claim(s) == "" {
			t.Errorf("not caught: %q", s)
		}
	}
	for _, s := range []string{
		"Mapeo orientativo frente a NIS2 y DORA", "Tokens de sesión", "Lens",
		"El ENS (Esquema Nacional de Seguridad) es una norma española.",
		"NIS2 pide gestionar los riesgos; la criptografía post-cuántica es una buena práctica.",
	} {
		if re := claim(s); re != "" {
			t.Errorf("ordinary text caught by %s: %q", re, s)
		}
	}
}
