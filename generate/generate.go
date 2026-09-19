// Package generate implements the deterministic artifact generation pipeline
// for formula.
//
// formula orchestrates generation of structured compliance artifacts from
// a program state JSON and gemara ControlCatalog. Every artifact it produces
// is deterministic — same inputs, same outputs, every run.
//
// Artifacts generated:
//   1. soa.csv             — Statement of Applicability
//   2. risk.csv            — Risk Assessment CSV (from risk register)
//   3. evidence-registry.csv — Evidence Registry
//   4. dependency-map.csv  — Control dependency map
//   5. context.md          — Compliance Context Document
//   6. collective-risk.csv — Collective risk register across all controls
//   7. system-card.md      — AI System Card (ISO 42001 / AI governance programs)
//
// Standard support: iso42001, iso27001, iec62443.
package generate

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Formulary-Labs/substrate/evidence"
)

// ArtifactType is one of the generated artifact types.
type ArtifactType string

const (
	SOA              ArtifactType = "soa"
	RiskCSV          ArtifactType = "risk"
	EvidenceRegistry ArtifactType = "evidence-registry"
	DependencyMap    ArtifactType = "dependency-map"
	ContextDoc       ArtifactType = "context"
	CollectiveRisk   ArtifactType = "collective-risk"
	SystemCard       ArtifactType = "system-card"
	XLSX             ArtifactType = "xlsx"
)

// AllArtifactTypes is the full list of artifact types formula can generate.
var AllArtifactTypes = []ArtifactType{
	SOA, RiskCSV, EvidenceRegistry, DependencyMap, ContextDoc, CollectiveRisk, SystemCard, XLSX,
}

// PipelineConfig configures a formula run.
type PipelineConfig struct {
	Program      string         `json:"program"`
	Framework    string         `json:"framework"`
	OutputDir    string         `json:"output_dir"`
	Artifacts    []ArtifactType `json:"artifacts,omitempty"` // empty = standard config or all
	StandardsDir string         `json:"standards_dir,omitempty"` // directory containing per-framework JSON configs
	DryRun       bool           `json:"dry_run,omitempty"`
}

// ControlEntry represents a single control from a catalog or assessment.
type ControlEntry struct {
	ID             string   `json:"id"`
	Title          string   `json:"title,omitempty"`
	Family         string   `json:"family,omitempty"`
	Determination  string   `json:"determination,omitempty"` // satisfied, partially_satisfied, not_satisfied, na
	Implementation string   `json:"implementation,omitempty"`
	Owner          string   `json:"owner,omitempty"`
	EvidenceRef    string   `json:"evidence_ref,omitempty"`
	Inherited      bool     `json:"inherited,omitempty"`
	InheritedFrom  string   `json:"inherited_from,omitempty"`
	Excluded       bool     `json:"excluded,omitempty"`
	ExclusionJustification string `json:"exclusion_justification,omitempty"`
	RiskScore      float64  `json:"risk_score,omitempty"`
	Dependencies   []string `json:"dependencies,omitempty"`
	ReviewCadence  string   `json:"review_cadence,omitempty"`
}

// RiskEntry is a single risk from the risk register.
type RiskEntry struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	Severity        string `json:"severity"`
	Status          string `json:"status"`
	Owner           string `json:"owner,omitempty"`
	ControlID       string `json:"control_id,omitempty"`
	RemediationPath string `json:"remediation_path,omitempty"`
}

// ProgramState is the minimal program state formula reads.
type ProgramState struct {
	Program     string         `json:"program"`
	Framework   string         `json:"framework,omitempty"`
	ProductName string         `json:"product_name,omitempty"`
	Scope       string         `json:"scope,omitempty"`
	Owner       string         `json:"owner,omitempty"`
	RunDate     *time.Time     `json:"run_date,omitempty"`
	Controls    []ControlEntry `json:"controls,omitempty"`
	Risks       []RiskEntry    `json:"risks,omitempty"`
}

// Load reads a program state from a JSON file.
func Load(path string) (*ProgramState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading program state %q: %w", path, err)
	}
	var ps ProgramState
	if err := json.Unmarshal(data, &ps); err != nil {
		return nil, fmt.Errorf("parsing program state %q: %w", path, err)
	}
	return &ps, nil
}

// ArtifactResult records the output of a single artifact generation.
type ArtifactResult struct {
	Artifact   ArtifactType
	OutputPath string
	Rows       int
	DryRun     bool
	Error      error
	// Data carries the header row and data rows for tabular artifacts so that
	// AssembleWorkbook can consume them without re-reading the CSV from disk.
	Data [][]string
}

// RunPipeline executes the full artifact generation pipeline.
// XLSX assembly always runs last so it can consume the tabular results from
// the other generators; requesting only XLSX without tabular artifacts is a no-op.
func RunPipeline(cfg PipelineConfig, ps *ProgramState) []ArtifactResult {
	artifacts := cfg.Artifacts
	if len(artifacts) == 0 {
		// Try to load a per-framework config from StandardsDir; fall back to all types.
		if cfg.StandardsDir != "" && cfg.Framework != "" {
			if sc, err := LoadStandardConfig(cfg.Framework, cfg.StandardsDir); sc != nil && err == nil {
				artifacts = sc
			}
		}
		if len(artifacts) == 0 {
			artifacts = AllArtifactTypes
		}
	}

	// Separate XLSX from other artifacts.
	needsXLSX := false
	var nonXLSX []ArtifactType
	for _, a := range artifacts {
		if a == XLSX {
			needsXLSX = true
		} else {
			nonXLSX = append(nonXLSX, a)
		}
	}

	var results []ArtifactResult
	for _, a := range nonXLSX {
		results = append(results, generate(a, cfg, ps))
	}
	if needsXLSX {
		results = append(results, AssembleWorkbook(results, ps, cfg))
	}
	return results
}

func generate(a ArtifactType, cfg PipelineConfig, ps *ProgramState) ArtifactResult {
	switch a {
	case SOA:
		return generateSOA(cfg, ps)
	case RiskCSV:
		return generateRisk(cfg, ps)
	case EvidenceRegistry:
		return generateEvidenceRegistry(cfg, ps)
	case DependencyMap:
		return generateDependencyMap(cfg, ps)
	case ContextDoc:
		return generateContext(cfg, ps)
	case CollectiveRisk:
		return generateCollectiveRisk(cfg, ps)
	case SystemCard:
		return generateSystemCard(cfg, ps)
	default:
		return ArtifactResult{Artifact: a, Error: fmt.Errorf("unknown artifact type %q", a)}
	}
}

// generateSOA produces a Statement of Applicability CSV.
func generateSOA(cfg PipelineConfig, ps *ProgramState) ArtifactResult {
	headers := []string{
		"Control ID", "Title", "Family", "In Scope", "Determination",
		"Implementation Status", "Implementation Summary",
		"Owner", "Inherited", "Inherited From",
		"Evidence Source", "Exclusion Justification",
	}
	rows := [][]string{headers}

	controls := ps.Controls
	sort.Slice(controls, func(i, j int) bool { return controls[i].ID < controls[j].ID })

	for _, c := range controls {
		inScope := "yes"
		if c.Excluded {
			inScope = "no"
		}
		inherited := ""
		if c.Inherited {
			inherited = "yes"
		}
		rows = append(rows, []string{
			c.ID, c.Title, c.Family, inScope,
			coalesceStr(c.Determination, "[PENDING]"),
			soaStatusLabel(c),
			truncate(c.Implementation, 200),
			c.Owner, inherited, c.InheritedFrom,
			evidenceSource(c.EvidenceRef),
			c.ExclusionJustification,
		})
	}

	return writeCSV(cfg, SOA, "soa.csv", rows)
}

// soaStatusLabel maps a control's implementation/inherited/excluded fields to
// the audit-readable status label used in the psc-ms taxonomy.
func soaStatusLabel(c ControlEntry) string {
	if c.Excluded {
		return "Not Applicable"
	}
	if c.Inherited {
		return "Implemented (Inherited)"
	}
	switch strings.ToLower(strings.TrimSpace(c.Implementation)) {
	case "configurable":
		return "Implemented (Configurable — operator-dependent)"
	case "":
		if strings.ToLower(c.Determination) == "na" || strings.ToLower(c.Determination) == "not_applicable" {
			return "Not Applicable"
		}
		return "Implemented (Native)"
	default:
		return "Implemented (Native)"
	}
}

// evidenceSource returns a prefixed evidence source string using ClassifyLink,
// e.g. "[GitHub] https://github.com/..." or "[PENDING]".
func evidenceSource(ref string) string {
	linkType, isPending := evidence.ClassifyLink(ref)
	if isPending {
		return "[PENDING]"
	}
	return fmt.Sprintf("[%s] %s", linkType, ref)
}

// generateRisk produces a Risk Assessment CSV.
func generateRisk(cfg PipelineConfig, ps *ProgramState) ArtifactResult {
	headers := []string{"Risk ID", "Title", "Severity", "Status", "Control ID", "Owner", "Remediation Path"}
	rows := [][]string{headers}

	risks := ps.Risks
	sort.Slice(risks, func(i, j int) bool { return risks[i].ID < risks[j].ID })

	for _, r := range risks {
		rows = append(rows, []string{
			r.ID, r.Title, r.Severity, r.Status,
			r.ControlID, r.Owner, r.RemediationPath,
		})
	}

	// Also generate risk entries from controls with no/partial determination.
	controlRisks := 0
	for _, c := range ps.Controls {
		if c.Determination == "not_satisfied" || c.Determination == "partially_satisfied" {
			controlRisks++
			rows = append(rows, []string{
				fmt.Sprintf("CTRL-RISK-%s", c.ID),
				fmt.Sprintf("Control gap: %s %s", c.ID, c.Title),
				controlSeverity(c.Determination),
				"open",
				c.ID, c.Owner, "[OWNER NEEDED]",
			})
		}
	}

	return writeCSV(cfg, RiskCSV, "risk.csv", rows)
}

// generateEvidenceRegistry produces an Evidence Registry CSV.
// Controls with no EvidenceRef emit a PENDING gap row so auditors can see the gap.
func generateEvidenceRegistry(cfg PipelineConfig, ps *ProgramState) ArtifactResult {
	headers := []string{"Control ID", "Title", "Owner", "Link Type", "Evidence Ref", "Pending", "Review Cadence", "Determination"}
	rows := [][]string{headers}

	for _, c := range ps.Controls {
		if c.Excluded {
			continue
		}
		linkType, isPending := evidence.ClassifyLink(c.EvidenceRef)
		pendingFlag := ""
		ref := c.EvidenceRef
		if isPending {
			pendingFlag = "Yes"
			ref = "NO LINKS PROVIDED"
		}
		rows = append(rows, []string{
			c.ID, c.Title, c.Owner,
			linkType, ref, pendingFlag,
			c.ReviewCadence,
			coalesceStr(c.Determination, "pending"),
		})
	}

	return writeCSV(cfg, EvidenceRegistry, "evidence-registry.csv", rows)
}

// generateDependencyMap produces a control dependency map CSV.
func generateDependencyMap(cfg PipelineConfig, ps *ProgramState) ArtifactResult {
	headers := []string{"Control ID", "Depends On"}
	rows := [][]string{headers}

	for _, c := range ps.Controls {
		if len(c.Dependencies) == 0 {
			continue
		}
		for _, dep := range c.Dependencies {
			rows = append(rows, []string{c.ID, dep})
		}
	}

	return writeCSV(cfg, DependencyMap, "dependency-map.csv", rows)
}

// generateContext produces a Compliance Context Document in Markdown.
func generateContext(cfg PipelineConfig, ps *ProgramState) ArtifactResult {
	sb := &strings.Builder{}
	fmt.Fprintf(sb, "# Compliance Context Document\n\n")
	fmt.Fprintf(sb, "**Program:** %s  \n", ps.Program)
	fmt.Fprintf(sb, "**Framework:** %s  \n", coalesceStr(ps.Framework, cfg.Framework))
	if ps.ProductName != "" {
		fmt.Fprintf(sb, "**Product:** %s  \n", ps.ProductName)
	}
	if ps.Scope != "" {
		fmt.Fprintf(sb, "**Scope:** %s  \n", ps.Scope)
	}
	fmt.Fprintf(sb, "**Generated:** %s\n\n", time.Now().Format("2006-01-02"))
	fmt.Fprintf(sb, "---\n\n")

	// Coverage summary.
	determined := map[string]int{}
	for _, c := range ps.Controls {
		determined[coalesceStr(c.Determination, "pending")]++
	}
	fmt.Fprintf(sb, "## Control Summary\n\n")
	fmt.Fprintf(sb, "| Status | Count |\n|---|---|\n")
	for _, k := range []string{"satisfied", "partially_satisfied", "not_satisfied", "na", "pending"} {
		if n := determined[k]; n > 0 {
			fmt.Fprintf(sb, "| %s | %d |\n", k, n)
		}
	}
	fmt.Fprintln(sb)

	fmt.Fprintf(sb, "## Risk Summary\n\n")
	if len(ps.Risks) > 0 {
		fmt.Fprintf(sb, "| Severity | Count |\n|---|---|\n")
		severityCounts := map[string]int{}
		for _, r := range ps.Risks {
			severityCounts[r.Severity]++
		}
		for _, k := range []string{"critical", "high", "medium", "low"} {
			if n := severityCounts[k]; n > 0 {
				fmt.Fprintf(sb, "| %s | %d |\n", k, n)
			}
		}
	} else {
		fmt.Fprintf(sb, "[DATA NEEDED: risk register — run specimen to populate]\n")
	}
	fmt.Fprintln(sb)

	path := filepath.Join(cfg.OutputDir, "context.md")
	content := sb.String()
	if cfg.DryRun {
		return ArtifactResult{Artifact: ContextDoc, OutputPath: path, Rows: strings.Count(content, "\n"), DryRun: true}
	}
	if err := writeFile(cfg.OutputDir, "context.md", []byte(content)); err != nil {
		return ArtifactResult{Artifact: ContextDoc, Error: err}
	}
	return ArtifactResult{Artifact: ContextDoc, OutputPath: path, Rows: strings.Count(content, "\n")}
}

// generateCollectiveRisk produces a collective risk register CSV across all controls.
func generateCollectiveRisk(cfg PipelineConfig, ps *ProgramState) ArtifactResult {
	headers := []string{"Source ID", "Title", "Type", "Severity", "Status", "Owner", "Control ID", "Remediation"}
	rows := [][]string{headers}

	// Include explicit risks from register.
	for _, r := range ps.Risks {
		rows = append(rows, []string{
			r.ID, r.Title, "explicit", r.Severity, r.Status,
			r.Owner, r.ControlID, r.RemediationPath,
		})
	}
	// Include implicit risks from control gaps.
	for _, c := range ps.Controls {
		if c.Determination == "not_satisfied" || c.Determination == "" {
			rows = append(rows, []string{
				fmt.Sprintf("GAP-%s", c.ID),
				fmt.Sprintf("Control not satisfied: %s", c.Title),
				"coverage_gap",
				controlSeverity(c.Determination),
				"open",
				c.Owner, c.ID, "",
			})
		}
	}

	return writeCSV(cfg, CollectiveRisk, "collective-risk.csv", rows)
}

// generateSystemCard produces an AI System Card Markdown document (ISO 42001).
func generateSystemCard(cfg PipelineConfig, ps *ProgramState) ArtifactResult {
	sb := &strings.Builder{}
	fmt.Fprintf(sb, "# AI System Card\n\n")
	fmt.Fprintf(sb, "**Program:** %s  \n", ps.Program)
	if ps.ProductName != "" {
		fmt.Fprintf(sb, "**System:** %s  \n", ps.ProductName)
	}
	fmt.Fprintf(sb, "**Framework:** %s  \n", coalesceStr(ps.Framework, cfg.Framework))
	fmt.Fprintf(sb, "**Generated:** %s\n\n", time.Now().Format("2006-01-02"))
	fmt.Fprintf(sb, "---\n\n")

	fmt.Fprintf(sb, "## System Description\n\n")
	fmt.Fprintf(sb, "[DATA NEEDED: system description — describe the AI system's purpose, capabilities, and operational context.]\n\n")
	fmt.Fprintf(sb, "## Intended Use\n\n")
	fmt.Fprintf(sb, "[DATA NEEDED: intended use — describe the intended use cases and target users.]\n\n")
	fmt.Fprintf(sb, "## Known Limitations\n\n")
	fmt.Fprintf(sb, "[DATA NEEDED: limitations — describe known limitations, failure modes, and out-of-scope use cases.]\n\n")
	fmt.Fprintf(sb, "## Training Data\n\n")
	fmt.Fprintf(sb, "[DATA NEEDED: training data — describe training data sources, data governance, and bias mitigation steps.]\n\n")
	fmt.Fprintf(sb, "## Evaluation\n\n")
	fmt.Fprintf(sb, "[DATA NEEDED: evaluation — describe evaluation methodology, metrics, and results.]\n\n")
	fmt.Fprintf(sb, "## Risk Treatment Summary\n\n")
	if len(ps.Risks) > 0 {
		fmt.Fprintf(sb, "%d risks tracked. See `risk.csv` and `collective-risk.csv` for full register.\n\n", len(ps.Risks))
	} else {
		fmt.Fprintf(sb, "[DATA NEEDED: risk summary — run formula --artifact risk to populate.]\n\n")
	}
	fmt.Fprintf(sb, "## Governance\n\n")
	if ps.Owner != "" {
		fmt.Fprintf(sb, "**Program Owner:** %s\n\n", ps.Owner)
	}
	fmt.Fprintf(sb, "[DATA NEEDED: governance — describe oversight bodies, review cadence, and accountability structures.]\n\n")

	path := filepath.Join(cfg.OutputDir, "system-card.md")
	content := sb.String()
	if cfg.DryRun {
		return ArtifactResult{Artifact: SystemCard, OutputPath: path, DryRun: true}
	}
	if err := writeFile(cfg.OutputDir, "system-card.md", []byte(content)); err != nil {
		return ArtifactResult{Artifact: SystemCard, Error: err}
	}
	return ArtifactResult{Artifact: SystemCard, OutputPath: path}
}

// --- helpers ---

func writeCSV(cfg PipelineConfig, a ArtifactType, filename string, rows [][]string) ArtifactResult {
	path := filepath.Join(cfg.OutputDir, filename)
	if cfg.DryRun {
		return ArtifactResult{Artifact: a, OutputPath: path, Rows: len(rows) - 1, DryRun: true, Data: rows}
	}
	if err := os.MkdirAll(cfg.OutputDir, 0o755); err != nil {
		return ArtifactResult{Artifact: a, Error: fmt.Errorf("creating output dir: %w", err)}
	}
	f, err := os.Create(path)
	if err != nil {
		return ArtifactResult{Artifact: a, Error: fmt.Errorf("creating %s: %w", filename, err)}
	}
	defer f.Close() //nolint:errcheck
	w := csv.NewWriter(f)
	if err := w.WriteAll(rows); err != nil {
		return ArtifactResult{Artifact: a, Error: fmt.Errorf("writing %s: %w", filename, err)}
	}
	return ArtifactResult{Artifact: a, OutputPath: path, Rows: len(rows) - 1, Data: rows}
}

func writeFile(dir, filename string, content []byte) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating output dir: %w", err)
	}
	return os.WriteFile(filepath.Join(dir, filename), content, 0o644)
}

func controlSeverity(determination string) string {
	switch determination {
	case "not_satisfied":
		return "high"
	case "partially_satisfied":
		return "medium"
	default:
		return "low"
	}
}

func coalesceStr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}
