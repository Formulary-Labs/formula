// formula runs the deterministic compliance artifact generation pipeline.
//
// Usage:
//
//	formula --program <slug> --catalog <gemara-control-catalog.yaml> [flags]
//
// The --catalog flag accepts a gemara ControlCatalog artifact and is the
// preferred way to specify framework context. framework, artifact sets, and
// control lists are all derived from the catalog. --framework is a fallback
// for when no catalog is available.
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
		catalogFlag   = flag.String("catalog", "", "Path to gemara ControlCatalog YAML (preferred over --framework)")
		frameworkFlag = flag.String("framework", "", "Framework fallback when no --catalog: iso42001, iso27001, iec62443, soc2, nist800-53")
		runStateFlag  = flag.String("run-state", "", "Path to program state JSON (default: runs/[program]/latest.json)")
		outputDirFlag = flag.String("output-dir", "", "Output directory (default: data/[program]/artifacts)")
		artifactsFlag = flag.String("artifacts", "", "Comma-separated artifact types (overrides catalog default set): soa,risk,evidence-registry,...")
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

	ps, stateErr := generate.Load(runStatePath)
	if stateErr != nil {
		fmt.Fprintf(os.Stderr, "warning: could not load run state %q: %v — generating with empty state\n", runStatePath, stateErr)
		ps = &generate.ProgramState{Program: *programFlag, Framework: *frameworkFlag}
	}
	if *frameworkFlag != "" {
		ps.Framework = *frameworkFlag
	}

	outputDir := *outputDirFlag
	if outputDir == "" {
		outputDir = filepath.Join("data", *programFlag, "artifacts")
	}

	// Resolve artifacts and controls from gemara catalog when provided.
	// This is the preferred path: catalog drives artifact set + control list.
	var artifacts []generate.ArtifactType
	if *catalogFlag != "" {
		catArtifacts, cat, catErr := generate.ArtifactsFromCatalog(*catalogFlag)
		if catErr != nil {
			fmt.Fprintf(os.Stderr, `{"error": "loading catalog: %v", "code": 2}`+"\n", catErr)
			os.Exit(exit.ToolError)
		}
		// Populate framework from catalog metadata.id if not already set.
		// Metadata is a value type — always safe to access directly.
		if ps.Framework == "" && cat != nil {
			ps.Framework = cat.Metadata.Id
		}
		// Merge catalog controls with existing program state.
		ps.Controls = generate.ControlsFromCatalog(cat, ps.Controls)
		// Use catalog-derived artifact set unless overridden by --artifacts.
		if *artifactsFlag == "" {
			artifacts = catArtifacts
		}
		fmt.Fprintf(os.Stderr, "[FORMULA] Loaded catalog: %s (%d controls)\n", *catalogFlag, len(ps.Controls))
	}

	// --artifacts overrides everything (catalog or framework defaults).
	if *artifactsFlag != "" {
		artifacts = nil
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
		stateErrStr := ""
		if stateErr != nil {
			stateErrStr = stateErr.Error()
		}
		enc.Encode(map[string]interface{}{ //nolint:errcheck
			"program":      *programFlag,
			"framework":    cfg.Framework,
			"output_dir":   outputDir,
			"results":      results,
			"state_loaded": stateErr == nil,
			"state_error":  stateErrStr,
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
		Purpose:     fmt.Sprintf("formula: %d artifacts generated (%d rows) for %s (%s)", len(results), totalRows, *programFlag, cfg.Framework),
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
  formula --program <slug> --catalog <gemara-catalog.yaml> [flags]

Flags:
  --program string      Program slug (required)
  --catalog string      Path to gemara ControlCatalog YAML (preferred — drives framework + control list)
  --framework string    Framework fallback when no --catalog: iso42001, iso27001, iec62443, soc2, nist800-53
  --run-state string    Path to program state JSON (default: runs/[program]/latest.json)
  --output-dir string   Output directory (default: data/[program]/artifacts)
  --artifacts string    Override artifact set: soa,risk,evidence-registry,dependency-map,context,collective-risk,system-card
  --dry-run             Print what would be written without writing
  --format string       Summary format: json (default), md
  --version             Print version and exit

The --catalog flag is the preferred way to configure formula. It loads a gemara
ControlCatalog YAML (Layer 1 artifact), extracts the framework, control list,
and selects the appropriate artifact set automatically. The --framework flag is
a fallback for when no catalog is available.

Framework artifact sets (built-in):
  iso27001   soa, risk, evidence-registry, dependency-map, context, collective-risk, xlsx
  iso42001   soa, risk, evidence-registry, dependency-map, context, collective-risk, system-card, xlsx
  iec62443   soa, risk, evidence-registry, context, collective-risk, xlsx
  soc2       soa, risk, evidence-registry, context, collective-risk, xlsx
  nist800-53 soa, risk, evidence-registry, dependency-map, context, collective-risk, xlsx

Examples:
  formula --program myprogram --catalog data/iso42001/gemara/iso42001-catalog.yaml
  formula --program myprogram --catalog data/iso27001/gemara/iso27001-catalog.yaml --dry-run
  formula --program myprogram --catalog catalog.yaml --artifacts soa,risk --format md`)
}
