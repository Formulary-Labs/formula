// formula runs the deterministic compliance artifact generation pipeline.
//
// Usage:
//
//	formula --program <slug> --framework <framework> [flags]
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Formulary-Labs/formula/generate"
	"github.com/Formulary-Labs/substrate/exit"
	"github.com/Formulary-Labs/substrate/provenance"
)

const version = "0.1.0"

func main() {
	var (
		programFlag   = flag.String("program", "", "Program slug (required)")
		frameworkFlag = flag.String("framework", "", "Framework: iso42001, iso27001, iec62443")
		runStateFlag  = flag.String("run-state", "", "Path to program state JSON (default: runs/[program]/latest.json)")
		outputDirFlag = flag.String("output-dir", "", "Output directory (default: data/[program]/artifacts)")
		artifactsFlag = flag.String("artifacts", "", "Comma-separated artifact types (default: all): soa,risk,evidence-registry,dependency-map,context,collective-risk,system-card")
		dryRunFlag    = flag.Bool("dry-run", false, "Print what would be written without writing")
		fmtFlag       = flag.String("format", "json", "Summary output format: json (default), md")
		versionFlag   = flag.Bool("version", false, "Print version and exit")
	)
	flag.Usage = usage
	flag.Parse()

	if *versionFlag {
		fmt.Printf("formula version %s\n", version)
		os.Exit(exit.OK)
	}

	if *programFlag == "" {
		fmt.Fprintln(os.Stderr, `{"error": "--program is required", "code": 2}`)
		flag.Usage()
		os.Exit(exit.ToolError)
	}

	runStatePath := *runStateFlag
	if runStatePath == "" {
		runStatePath = filepath.Join("runs", *programFlag, "latest.json")
	}

	ps, err := generate.Load(runStatePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not load run state %q: %v — generating with empty state\n", runStatePath, err)
		ps = &generate.ProgramState{Program: *programFlag, Framework: *frameworkFlag}
	}
	if *frameworkFlag != "" {
		ps.Framework = *frameworkFlag
	}

	outputDir := *outputDirFlag
	if outputDir == "" {
		outputDir = filepath.Join("data", *programFlag, "artifacts")
	}

	var artifacts []generate.ArtifactType
	if *artifactsFlag != "" {
		for _, a := range strings.Split(*artifactsFlag, ",") {
			artifacts = append(artifacts, generate.ArtifactType(strings.TrimSpace(a)))
		}
	}

	cfg := generate.PipelineConfig{
		Program:   *programFlag,
		Framework: coalesceStr(ps.Framework, *frameworkFlag),
		OutputDir: outputDir,
		Artifacts: artifacts,
		DryRun:    *dryRunFlag,
	}

	results := generate.RunPipeline(cfg, ps)

	switch *fmtFlag {
	case "md":
		printMD(results)
	default:
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(map[string]interface{}{ //nolint:errcheck
			"program":    *programFlag,
			"output_dir": outputDir,
			"results":    results,
		})
	}

	totalRows := 0
	for _, r := range results {
		totalRows += r.Rows
	}
	_ = provenance.Write("logs/provenance.jsonl", provenance.Entry{
		Spec:        "functions/compliance-doc-generator-spec.md",
		Output:      outputDir,
		OutputType:  "other",
		Program:     *programFlag,
		Purpose:     fmt.Sprintf("formula: %d artifacts generated (%d rows) for %s", len(results), totalRows, *programFlag),
		Reusability: provenance.Instance,
		QualityGate: provenance.Pass,
		Tool:        "formula",
		ToolVersion: version,
	})
}

func printMD(results []generate.ArtifactResult) {
	fmt.Printf("# Formula Pipeline Results\n\n")
	fmt.Printf("| Artifact | Output | Rows | Status |\n|---|---|---|---|\n")
	for _, r := range results {
		status := "ok"
		if r.DryRun {
			status = "dry-run"
		}
		if r.Error != nil {
			status = fmt.Sprintf("error: %v", r.Error)
		}
		fmt.Printf("| %s | %s | %d | %s |\n", r.Artifact, r.OutputPath, r.Rows, status)
	}
}

func coalesceStr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func usage() {
	fmt.Fprintln(os.Stderr, `formula — deterministic compliance artifact generation pipeline

Usage:
  formula --program <slug> [flags]

Flags:
  --program string      Program slug (required)
  --framework string    Framework: iso42001, iso27001, iec62443
  --run-state string    Path to program state JSON (default: runs/[program]/latest.json)
  --output-dir string   Output directory (default: data/[program]/artifacts)
  --artifacts string    Comma-separated types: soa,risk,evidence-registry,dependency-map,context,collective-risk,system-card
  --dry-run             Print what would be written without writing
  --format string       Summary format: json (default), md
  --version             Print version and exit

Examples:
  formula --program iso42001 --framework iso42001 --dry-run
  formula --program iso42001 --framework iso42001 --output-dir artifacts/
  formula --program iso42001 --artifacts soa,risk --format md`)
}
