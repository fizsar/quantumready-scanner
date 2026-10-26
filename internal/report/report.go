// Package report turns what is stored about a finished scan into a document
// a person reads: the neutral model every output serializes (the API's JSON
// today; PDF or .docx some day), with every text resolved from the catalog in
// one language (ADR-0002). Build is pure: no database, no HTTP. It only ever
// reads facts (ADR-0001), so an old scan renders with today's wording.
package report

import (
	"encoding/json"
	"time"

	"github.com/fizsar/quantumready-saas/internal/catalog"
	"github.com/fizsar/quantumready-saas/internal/scoring"
)

// Finding is a stored finding: code, judgement and parameters, no text.
type Finding struct {
	Code     string
	Axis     string
	Severity string
	Category string
	Params   map[string]string
}

// Input is everything stored about a finished scan.
type Input struct {
	// Scoring is the stored judgement, without its findings (they are rows).
	Scoring scoring.Result
	// Result is the scanner's raw output, passed through as is.
	Result json.RawMessage
	// Findings, in their stored order (severity first).
	Findings []Finding
	// ScanTime is when the scan ran: some texts depend on it.
	ScanTime time.Time
	Lang     string
}

// Document is a scan's report, every text in one language. The JSON tags are
// the API's (docs/api.md): the API serializes it unchanged.
type Document struct {
	ScoringVersion string          `json:"scoring_version"`
	Score          int             `json:"score"`
	Grade          string          `json:"grade"`
	ScoreGrade     string          `json:"score_grade"`
	FloorPoints    int             `json:"floor_points,omitempty"`
	Cap            *Cap            `json:"cap,omitempty"`
	PQC            bool            `json:"pqc"`
	SelectedGroup  string          `json:"selected_group,omitempty"`
	TLSVersion     string          `json:"tls_version,omitempty"`
	QuantumRisk    Labeled         `json:"quantum_risk"`
	CurrentRisk    Labeled         `json:"current_risk"`
	Blocks         []Block         `json:"blocks"`
	Findings       []FindingText   `json:"findings"`
	NotEvaluated   []Labeled       `json:"not_evaluated"`
	Result         json.RawMessage `json:"result"` // raw scanner facts
}

// Labeled is a code with its display name.
type Labeled struct {
	Code  string `json:"code"`
	Label string `json:"label"`
}

// Cap is the worst grade allowed and the findings that impose it, by title.
type Cap struct {
	Grade string    `json:"grade"`
	Codes []Labeled `json:"findings"`
}

// Block is one part of the score breakdown.
type Block struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Points int    `json:"points"`
	Max    int    `json:"max"`
	Items  []Item `json:"items,omitempty"`
}

// Item is one scored criterion of a block.
type Item struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Points int    `json:"points"`
	Max    int    `json:"max"`
}

// FindingText is a finding with its labels and texts.
type FindingText struct {
	Code          string `json:"code"`
	Axis          string `json:"axis"`
	AxisLabel     string `json:"axis_label"`
	Severity      string `json:"severity"`
	SeverityLabel string `json:"severity_label"`
	Category      string `json:"category"`
	catalog.Text
}

// Build renders a finished scan's report in in.Lang.
func Build(in Input) (*Document, error) {
	lang, s := in.Lang, in.Scoring
	var facts struct {
		Default struct {
			Version string `json:"version"`
		} `json:"default"`
		KeyExchange struct {
			Selected string `json:"selected"`
		} `json:"key_exchange"`
	}
	if err := json.Unmarshal(in.Result, &facts); err != nil {
		return nil, err
	}
	doc := &Document{
		ScoringVersion: s.ScoringVersion, Score: s.Score, Grade: s.Grade, ScoreGrade: s.ScoreGrade,
		FloorPoints: s.FloorPoints, PQC: s.PQC,
		SelectedGroup: facts.KeyExchange.Selected, TLSVersion: facts.Default.Version,
		QuantumRisk: Labeled{s.QuantumRisk, catalog.Label("severity."+s.QuantumRisk, lang)},
		CurrentRisk: Labeled{s.CurrentRisk, catalog.Label("severity."+s.CurrentRisk, lang)},
		Result:      in.Result,
	}
	// Finding parameters by code: some titles use them, e.g. "({bits} bits)".
	params := make(map[string]map[string]string, len(in.Findings))
	// Some texts depend on the scan: its date and its other findings.
	tctx := catalog.Context{ScanTime: in.ScanTime}
	for _, f := range in.Findings {
		params[f.Code] = f.Params
		tctx.Codes = append(tctx.Codes, f.Code)
	}
	if s.Cap != nil {
		doc.Cap = &Cap{Grade: s.Cap.Grade}
		for _, c := range s.Cap.Codes {
			doc.Cap.Codes = append(doc.Cap.Codes, Labeled{c, catalog.DescribeIn(c, params[c], lang, tctx).Title})
		}
	}
	for _, b := range s.Blocks {
		bv := Block{ID: b.ID, Label: catalog.Label("block."+b.ID, lang), Points: b.Points, Max: b.Max}
		for _, it := range b.Items {
			bv.Items = append(bv.Items, Item{it.ID, catalog.Label("item."+it.ID, lang), it.Points, it.Max})
		}
		doc.Blocks = append(doc.Blocks, bv)
	}
	doc.Findings = make([]FindingText, 0, len(in.Findings))
	for _, f := range in.Findings {
		doc.Findings = append(doc.Findings, FindingText{
			Code: f.Code, Axis: f.Axis, AxisLabel: catalog.Label("axis."+f.Axis, lang),
			Severity: f.Severity, SeverityLabel: catalog.Label("severity."+f.Severity, lang),
			Category: f.Category, Text: catalog.DescribeIn(f.Code, f.Params, lang, tctx),
		})
	}
	doc.NotEvaluated = make([]Labeled, 0, len(s.NotEvaluated))
	for _, c := range s.NotEvaluated {
		doc.NotEvaluated = append(doc.NotEvaluated, Labeled{c, catalog.Label("noteval."+c, lang)})
	}
	return doc, nil
}
