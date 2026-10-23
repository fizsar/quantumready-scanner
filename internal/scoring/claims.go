package scoring

import "regexp"

// ForbiddenClaims are what no report or page may say (decided on 24/10/2026):
// QuantumReady is not official, certifies nothing against the ENS and NIS2
// does not require post-quantum cryptography. The same patterns as
// web/src/test/forbidden.ts (forbiddenClaims); internal/httpapi checks its
// messages with them too, and so must any future report generator (PDF,
// .docx).
var ForbiddenClaims = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\boficial(es|mente)?\b|\bofficial(ly)?\b`),
	regexp.MustCompile(`\bcertific\w*\s+(\w+\s+){0,2}ENS\b|\bENS[\s-]+certifi\w*`),
	regexp.MustCompile(`\bcumpl\w*\s+(\w+\s+){0,2}ENS\b|\bcompl(y|ies|iant|iance)\s+(\w+\s+){0,2}ENS\b|\bENS[\s-]+compliant`),
	regexp.MustCompile(`(?i)\bNIS\s?2\b[^.]{0,80}\b(exig\w*|oblig\w*|requier\w*|require[sd]?|mandat\w*)\b[^.]{0,80}(cu[aá]ntic|quantum|PQC|ML-KEM)`),
	regexp.MustCompile(`(?i)(cu[aá]ntic|quantum|PQC|ML-KEM)[^.]{0,80}\b(exigid\w*|obligatori\w*|required|mandatory|mandated)\b[^.]{0,40}\bNIS\s?2\b`),
}
