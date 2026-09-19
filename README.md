# formula

Deterministic compliance artifact generation pipeline.

```bash
go get github.com/Formulary-Labs/formula
```

## What it does

`formula` generates seven structured compliance artifacts from a `ProgramState` JSON and an optional gemara `ControlCatalog`. It fills every section where the content can be derived from the input data. Sections that require prose judgment — risk narratives, scope descriptions, management review content — are left as `[DATA NEEDED: narrative]` placeholders for the agent layer to complete.

Same inputs produce the same outputs on every run. No randomness, no inference, no generation of content that isn't traceable to the input.

## Usage

```go
import "github.com/Formulary-Labs/formula/generate"

pipeline, err := generate.New(generate.PipelineConfig{
    Program:   "my-program",
    Framework: "iso27001",
    OutputDir: "output/",
    DryRun:    false,
})

err = pipeline.Run(programStateJSON)
```

To generate a subset of artifacts:

```go
generate.PipelineConfig{
    Artifacts: []generate.ArtifactType{
        generate.SOA,
        generate.EvidenceRegistry,
        generate.RiskCSV,
    },
}
```

## Artifacts

| Artifact | Filename | Format | Contents |
|---|---|---|---|
| Statement of Applicability | `soa.csv` | CSV | All controls with inclusion/exclusion status, owner, evidence reference, justification |
| Risk Assessment | `risk.csv` | CSV | Risk register entries with severity, status, owner, remediation path |
| Evidence Registry | `evidence-registry.csv` | CSV | Evidence items linked to controls with collection status and due dates |
| Control Dependency Map | `dependency-map.csv` | CSV | Control-to-control dependency relationships from the catalog |
| Compliance Context Document | `context.md` | Markdown | Program scope, framework summary, key dates, `[DATA NEEDED]` for narrative sections |
| Collective Risk Register | `collective-risk.csv` | CSV | Aggregated risk view across all control families |
| Management System Card | `system-card.md` | Markdown | AI system card (ISO 42001 / AI governance programs); `[DATA NEEDED]` for system description sections |

### Framework support

`iso27001`, `iso42001`, `iec62443`. Framework determines which artifact fields are populated and which sections in the Markdown outputs are included.

## ProgramState input

```json
{
  "program": "my-program",
  "framework": "iso27001",
  "product_name": "My Product",
  "scope": "Production environment — SaaS platform",
  "owner": "security-team",
  "run_date": "2026-09-18",
  "controls": [
    {
      "id": "A.5.1",
      "status": "implemented",
      "owner": "security-team",
      "evidence_ref": "evidence/policy-review-2026.pdf",
      "determination": "satisfied"
    }
  ],
  "risks": [
    { "id": "RISK-001", "severity": "high", "status": "open" }
  ]
}
```

`controls` and `risks` are the primary data sources. If a `ControlCatalog` path is also provided, `formula` overlays catalog metadata (titles, objectives, families, dependencies) onto the control entries.

## The [DATA NEEDED] contract

Every `[DATA NEEDED: narrative]` placeholder in a `formula` output is a defined handoff point. It marks a section where deterministic generation stops and judgment is required — a scope statement, a management review narrative, a risk treatment rationale. These are not gaps; they are boundaries.

The agent layer (regimen) fills these placeholders using the program's memory, decisions log, and context. `formula` makes its scope explicit so the division of labor is auditable.

## Dry run

```go
generate.PipelineConfig{DryRun: true}
```

In dry-run mode, `formula` prints each artifact's file path and row count to stdout without writing any files. Useful for verifying the pipeline configuration before a run.

## License

Apache License 2.0
