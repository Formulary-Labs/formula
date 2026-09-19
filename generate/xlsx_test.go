package generate_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Formulary-Labs/formula/generate"
)

func TestAssembleWorkbook_createsFile(t *testing.T) {
	ps := makeState(
		[]generate.ControlEntry{
			{ID: "A.5.1", Title: "Policies", Family: "A.5", Determination: "satisfied", Owner: "alice"},
			{ID: "A.5.2", Title: "Roles", Family: "A.5", Determination: "partially_satisfied", Owner: "bob"},
		},
		[]generate.RiskEntry{
			{ID: "RISK-001", Title: "No MFA", Severity: "high", Status: "open", Owner: "sec-team"},
		},
	)
	dir := t.TempDir()
	cfg := generate.PipelineConfig{
		Program:   "test",
		Framework: "iso27001",
		OutputDir: dir,
		Artifacts: []generate.ArtifactType{generate.SOA, generate.RiskCSV, generate.XLSX},
	}

	results := generate.RunPipeline(cfg, ps)
	var xlsxResult generate.ArtifactResult
	for _, r := range results {
		if r.Artifact == generate.XLSX {
			xlsxResult = r
		}
	}

	if xlsxResult.Error != nil {
		t.Fatalf("XLSX generation error: %v", xlsxResult.Error)
	}

	path := filepath.Join(dir, "compliance.xlsx")
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected workbook.xlsx to exist: %v", err)
	}
	if xlsxResult.Rows == 0 {
		t.Error("expected non-zero row count for workbook")
	}
}

func TestAssembleWorkbook_dryRun(t *testing.T) {
	ps := makeState(
		[]generate.ControlEntry{
			{ID: "A.5.1", Title: "Policy", Family: "A.5", Determination: "satisfied"},
		},
		nil,
	)
	dir := t.TempDir()
	cfg := generate.PipelineConfig{
		Program:   "test",
		Framework: "iso27001",
		OutputDir: dir,
		Artifacts: []generate.ArtifactType{generate.SOA, generate.XLSX},
		DryRun:    true,
	}

	results := generate.RunPipeline(cfg, ps)
	for _, r := range results {
		if r.Error != nil {
			t.Errorf("dry-run artifact %s error: %v", r.Artifact, r.Error)
		}
		if !r.DryRun {
			t.Errorf("artifact %s: expected DryRun=true", r.Artifact)
		}
	}

	entries, _ := os.ReadDir(dir)
	if len(entries) > 0 {
		t.Errorf("dry-run created %d files, expected 0", len(entries))
	}
}

func TestAssembleWorkbook_multipleSheets(t *testing.T) {
	ps := makeState(
		[]generate.ControlEntry{
			{ID: "A.5.1", Title: "Policy", Determination: "satisfied"},
		},
		[]generate.RiskEntry{
			{ID: "RISK-001", Title: "No MFA", Severity: "high", Status: "open"},
		},
	)
	dir := t.TempDir()
	cfg := generate.PipelineConfig{
		Program:   "test",
		Framework: "iso27001",
		OutputDir: dir,
		Artifacts: []generate.ArtifactType{generate.SOA, generate.RiskCSV, generate.EvidenceRegistry, generate.XLSX},
	}

	results := generate.RunPipeline(cfg, ps)
	var xlsxResult generate.ArtifactResult
	for _, r := range results {
		if r.Artifact == generate.XLSX {
			xlsxResult = r
		}
	}

	if xlsxResult.Error != nil {
		t.Fatalf("XLSX error: %v", xlsxResult.Error)
	}
	if xlsxResult.Rows <= 0 {
		t.Error("expected non-zero total rows across all sheets")
	}
}
