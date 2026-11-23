package review

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Severity classifies a Review Finding as a blocking error or an advisory warning.
type Severity string

const (
	SeverityError Severity = "error"
	SeverityWarn  Severity = "warn"
)

// Finding represents a single deterministic diagnostic item emitted by a Review.
type Finding struct {
	RuleID   string   `json:"rule_id"`
	File     string   `json:"file"`
	Line     int      `json:"line"`
	Severity Severity `json:"severity"`
	Message  string   `json:"message"`
	Hint     string   `json:"hint"`
}

// Report collects all Findings emitted across reviewed artifacts with deterministic ordering and summary counts.
type Report struct {
	Findings   []Finding `json:"findings"`
	ErrorCount int       `json:"error_count"`
	WarnCount  int       `json:"warn_count"`
	Files      int       `json:"files"`
}

// Add appends one or more Findings and updates summary counts and ordering.
func (r *Report) Add(findings ...Finding) {
	r.Findings = append(r.Findings, findings...)
	r.Finalize()
}

// Finalize sorts Findings deterministically by file, line, rule ID, severity, and message,
// and recomputes ErrorCount and WarnCount.
func (r *Report) Finalize() {
	if r.Findings == nil {
		r.Findings = []Finding{}
	}
	sort.SliceStable(r.Findings, func(i, j int) bool {
		a, b := r.Findings[i], r.Findings[j]
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		if a.RuleID != b.RuleID {
			return a.RuleID < b.RuleID
		}
		if a.Severity != b.Severity {
			return severityRank(a.Severity) < severityRank(b.Severity)
		}
		return a.Message < b.Message
	})

	errs, warns := 0, 0
	for _, f := range r.Findings {
		switch f.Severity {
		case SeverityError:
			errs++
		case SeverityWarn:
			warns++
		}
	}
	r.ErrorCount = errs
	r.WarnCount = warns
}

func severityRank(s Severity) int {
	if s == SeverityError {
		return 0
	}
	return 1
}

// Failed reports whether the Review should fail given strict mode.
func (r Report) Failed(strict bool) bool {
	if r.ErrorCount > 0 {
		return true
	}
	return strict && r.WarnCount > 0
}

// Format renders the Report in the requested output format ("text", "json", or "md").
func (r Report) Format(format string) (string, error) {
	r.Finalize()
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "text":
		return r.formatText(), nil
	case "json":
		return r.formatJSON()
	case "md":
		return r.formatMarkdown(), nil
	default:
		return "", fmt.Errorf("unsupported review format %q (supported: text, json, md)", format)
	}
}

func (r Report) formatText() string {
	var b strings.Builder
	if len(r.Findings) == 0 {
		fmt.Fprintf(&b, "review: 0 errors, 0 warnings (%d %s checked)\n", r.Files, pluralize("file", r.Files))
		return b.String()
	}
	for _, f := range r.Findings {
		fmt.Fprintf(&b, "%s:%d: %s [%s] %s\n", f.File, f.Line, f.Severity, f.RuleID, f.Message)
		if strings.TrimSpace(f.Hint) != "" {
			fmt.Fprintf(&b, "  hint: %s\n", f.Hint)
		}
	}
	fmt.Fprintf(&b, "\nSummary: %d %s, %d %s (%d %s checked)\n",
		r.ErrorCount, pluralize("error", r.ErrorCount),
		r.WarnCount, pluralize("warning", r.WarnCount),
		r.Files, pluralize("file", r.Files),
	)
	return b.String()
}

func (r Report) formatJSON() (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(r); err != nil {
		return "", fmt.Errorf("encode review report json: %w", err)
	}
	return buf.String(), nil
}

func (r Report) formatMarkdown() string {
	var b strings.Builder
	b.WriteString("# DataFolio Review\n\n")
	fmt.Fprintf(&b, "- **Files checked**: %d\n", r.Files)
	fmt.Fprintf(&b, "- **Errors**: %d\n", r.ErrorCount)
	fmt.Fprintf(&b, "- **Warnings**: %d\n\n", r.WarnCount)
	if len(r.Findings) == 0 {
		b.WriteString("No findings.\n")
		return b.String()
	}
	b.WriteString("| Severity | Rule | File | Line | Message | Hint |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- |\n")
	for _, f := range r.Findings {
		fmt.Fprintf(&b, "| %s | `%s` | `%s` | %d | %s | %s |\n",
			escapeMarkdownCell(string(f.Severity)),
			escapeMarkdownCell(f.RuleID),
			escapeMarkdownCell(f.File),
			f.Line,
			escapeMarkdownCell(f.Message),
			escapeMarkdownCell(f.Hint),
		)
	}
	return b.String()
}

func escapeMarkdownCell(s string) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "|", `\|`)
	return s
}

func pluralize(word string, n int) string {
	if n == 1 {
		return word
	}
	return word + "s"
}
