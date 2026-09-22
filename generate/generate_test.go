package generate_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Formulary-Labs/formula/generate"
)

func makeState(controls []generate.ControlEntry, risks []generate.RiskEntry) *generate.ProgramState {
	return &generate.ProgramState{
		Program:     "test",
		Framework:   "iso42001",
		ProductName: "Test Product",
		Controls:    controls,
		Risks:       risks,
	}
}

func TestRunPipeline_dryRun(t *testing.T) {
	ps := makeState(
		[]generate.ControlEntry{
			{ID: "A.5.1", Title: "Policy", Family: "A.5", Determination: "satisfied", Owner: "alice"},
		},
		nil,
	)
	cfg := generate.PipelineConfig{
		Program:   "test",
		Framework: "iso42001",
		OutputDir: t.TempDir(),
		DryRun:    true,
	}
	results := generate.RunPipeline(cfg, ps)
	for _, r := range results {
		if r.Error != nil {
			t.Errorf("artifact %s error: %v", r.Artifact, r.Error)
		}
		if !r.DryRun {
			t.Errorf("artifact %s: expected DryRun=true", r.Artifact)
		}
	}
	// No files should have been created.
	entries, _ := os.ReadDir(cfg.OutputDir)
	if len(entries) > 0 {
		t.Errorf("dry-run created %d files, expected 0", len(entries))
	}
}

func TestGenerateSOA(t *testing.T) {
	ps := makeState(
		[]generate.ControlEntry{
			{ID: "A.5.1", Title: "Policies", Family: "A.5", Determination: "satisfied", Owner: "alice"},
			{ID: "A.5.2", Title: "Roles", Family: "A.5", Determination: "partially_satisfied", Owner: "bob"},
			{ID: "A.8.1", Title: "Physical", Family: "A.8", Excluded: true, ExclusionJustification: "Cloud hosted"},
		},
		nil,
	)
	dir := t.TempDir()
	cfg := generate.PipelineConfig{
		Program:   "test",
		OutputDir: dir,
		Artifacts: []generate.ArtifactType{generate.SOA},
	}
	results := generate.RunPipeline(cfg, ps)
	if results[0].Error != nil {
		t.Fatalf("SOA error: %v", results[0].Error)
	}

	data, err := os.ReadFile(filepath.Join(dir, "soa.csv"))
	if err != nil {
		t.Fatalf("soa.csv not created: %v", err)
	}
	if !strings.Contains(string(data), "A.5.1") {
		t.Error("soa.csv missing A.5.1")
	}
	if !strings.Contains(string(data), "no") { // excluded control
		t.Error("soa.csv missing excluded control marker")
	}
}

func TestGenerateRisk_fromRegister(t *testing.T) {
	ps := makeState(
		nil,
		[]generate.RiskEntry{
			{ID: "RISK-001", Title: "No MFA", Severity: "high", Status: "open", Owner: "sec-team"},
		},
	)
	dir := t.TempDir()
	cfg := generate.PipelineConfig{
		Program:   "test",
		OutputDir: dir,
		Artifacts: []generate.ArtifactType{generate.RiskCSV},
	}
	results := generate.RunPipeline(cfg, ps)
	if results[0].Error != nil {
		t.Fatalf("risk error: %v", results[0].Error)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "risk.csv"))
	if !strings.Contains(string(data), "RISK-001") {
		t.Error("risk.csv missing RISK-001")
	}
}

func TestFromFile(t *testing.T) {
	ps := &generate.ProgramState{
		Program:   "test",
		Framework: "iso27001",
		Controls: []generate.ControlEntry{
			{ID: "A.5.1", Title: "Policy", Determination: "satisfied"},
		},
	}
	data, _ := json.Marshal(ps)
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	os.WriteFile(path, data, 0o600) //nolint:errcheck

	loaded, err := generate.Load(path)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if loaded.Framework != "iso27001" {
		t.Errorf("framework = %q, want iso27001", loaded.Framework)
	}
}
