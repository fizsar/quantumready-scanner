package catalog

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"
)

// Context is what a finding's text may depend on besides its own params:
// when the scan ran and which other findings it has. It is applied when the
// text is rendered, so stored findings (code and params only) keep reading
// right, old scans included.
type Context struct {
	ScanTime time.Time // zero: now
	Codes    []string  // every finding code of the scan
}

// The CA/Browser Forum ballot SC-081: maximum validity of TLS certificates
// issued on or after each date.
var validitySchedule = []struct {
	from time.Time
	days int
}{
	{time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC), 200},
	{time.Date(2027, 3, 15, 0, 0, 0, 0, time.UTC), 100},
	{time.Date(2029, 3, 15, 0, 0, 0, 0, time.UTC), 47},
}

// params returns p plus the placeholders computed from the context:
//   - {validity_schedule}: the SC-081 schedule, what is past in the past
//     tense and what is still ahead in the future, as of the scan's date;
//   - {renewal_advice}: keep automated renewal, or automate it, depending on
//     whether the scan found automated renewal (CERT_AUTOMATED_*).
func (c Context) params(p map[string]string, lang string) map[string]string {
	out := maps.Clone(p)
	if out == nil {
		out = map[string]string{}
	}
	at := c.ScanTime
	if at.IsZero() {
		at = time.Now()
	}
	out["validity_schedule"] = scheduleText(at, lang)
	automated := slices.ContainsFunc(c.Codes, func(code string) bool { return strings.HasPrefix(code, "CERT_AUTOMATED_") })
	out["renewal_advice"] = renewalAdvice[lang][automated]
	return out
}

var renewalAdvice = map[string]map[bool]string{
	"es": {
		true:  "Mantén la renovación automatizada para poder cambiar a certificados ML-DSA o híbridos en cuanto las autoridades los emitan.",
		false: "Automatiza la renovación (ACME) para poder cambiar a certificados post-cuánticos (ML-DSA o híbridos) en cuanto las autoridades los emitan: hoy este certificado no da señales de renovarse solo.",
	},
	"en": {
		true:  "Keep renewal automated so you can switch to ML-DSA or hybrid certificates as soon as authorities issue them.",
		false: "Automate renewal (ACME) so you can switch to post-quantum (ML-DSA or hybrid) certificates as soon as authorities issue them: today this certificate shows no sign of renewing itself.",
	},
}

var monthNames = map[string][]string{
	"es": {"enero", "febrero", "marzo", "abril", "mayo", "junio", "julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre"},
	"en": {"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"},
}

func longDate(t time.Time, lang string) string {
	m := monthNames[lang][t.Month()-1]
	if lang == "es" {
		return fmt.Sprintf("%d de %s de %d", t.Day(), m, t.Year())
	}
	return fmt.Sprintf("%d %s %d", t.Day(), m, t.Year())
}

// scheduleText says where the SC-081 schedule stands on the scan's date, for
// example (ES, scanned in October 2026): "La validez máxima de los
// certificados ya es de 200 días (desde el 15 de marzo de 2026) y bajará a
// 100 días el 15 de marzo de 2027 y a 47 el 15 de marzo de 2029 (votación
// SC-081 del CA/Browser Forum)".
func scheduleText(at time.Time, lang string) string {
	es := lang == "es"
	at = at.UTC()
	current := -1 // index of the last step already in force
	var steps []string
	for i, s := range validitySchedule {
		if !at.Before(s.from) {
			current = i
			continue
		}
		unit := "" // the unit only on the first upcoming step
		if len(steps) == 0 {
			unit = map[bool]string{true: " días", false: " days"}[es]
		}
		if es {
			steps = append(steps, fmt.Sprintf("a %d%s el %s", s.days, unit, longDate(s.from, lang)))
		} else {
			steps = append(steps, fmt.Sprintf("to %d%s on %s", s.days, unit, longDate(s.from, lang)))
		}
	}
	upcoming := steps
	if len(steps) > 1 {
		and := map[bool]string{true: " y ", false: " and "}[es]
		upcoming = []string{strings.Join(steps[:len(steps)-1], ", ") + and + steps[len(steps)-1]}
	}
	source := map[bool]string{true: "votación SC-081 del CA/Browser Forum", false: "CA/Browser Forum ballot SC-081"}[es]
	if current < 0 {
		if es {
			return fmt.Sprintf("La validez máxima de los certificados bajará %s (%s)", upcoming[0], source)
		}
		return fmt.Sprintf("Maximum certificate validity will drop %s (%s)", upcoming[0], source)
	}
	now := validitySchedule[current]
	switch {
	case es && len(upcoming) == 0:
		return fmt.Sprintf("La validez máxima de los certificados ya es de %d días (desde el %s; %s)", now.days, longDate(now.from, lang), source)
	case es:
		return fmt.Sprintf("La validez máxima de los certificados ya es de %d días (desde el %s) y bajará %s (%s)", now.days, longDate(now.from, lang), upcoming[0], source)
	case len(upcoming) == 0:
		return fmt.Sprintf("Maximum certificate validity is already %d days (since %s; %s)", now.days, longDate(now.from, lang), source)
	default:
		return fmt.Sprintf("Maximum certificate validity is already %d days (since %s) and will drop %s (%s)", now.days, longDate(now.from, lang), upcoming[0], source)
	}
}
