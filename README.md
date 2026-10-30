# formula

Seven compliance artifacts from one input. Same data in, same files out, every run. Sections requiring judgment are marked `[DATA NEEDED]` — that boundary is the product.

```bash
go get github.com/Formulary-Labs/formula
```

## In the ecosystem

Formulary is a **pick-and-choose** suite of small, single-purpose CLIs. Install only the tools your workflow needs; each runs standalone. Shared contracts live in [`substrate`](https://github.com/Formulary-Labs/substrate). Optional Make pipelines (or an agent such as `regimen`) compose tools into a run — plug-and-play, not all-or-nothing.

**This tool:** deterministic packet artifacts: SoA, risk CSVs, system card, impact.  
**Works with:** `compound` cites packet paths; `impact` / `titer` complement.  
**Pipeline stage:** packet.


## What it does

`formula` generates seven structured compliance artifacts from a `ProgramState` JSON and an optional OSCAL catalog or gemara `ControlCatalog`. It fills every section where the content can be derived from the input data. Sections that require prose judgment — risk narratives, scope descriptions, management review content — are left as `[DATA NEEDED: narrative]` placeholders for the agent layer to complete.

No randomness, no inference, no content that isn't traceable to the input.

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

`iso27001`, `iso42001`, `iec62443`, `soc2`, `nist800-53`. Framework determines which artifact fields are populated and which sections in the Markdown outputs are included. When both `--catalog` and `--framework` are given, the catalog wins and framework metadata is derived from catalog content.

Use `--artifacts` to generate a subset (comma-separated, e.g. `soa,evidence-registry`); by default all artifacts supported for the framework are generated. `--dry-run` prints the planned writes without writing.

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

## Catalogs

`formula` accepts an OSCAL catalog or a gemara `ControlCatalog` via `--catalog` to derive framework metadata and control titles for artifact generation. For OSCAL, set `metadata.document-ids[].identifier` to the framework slug. See [CATALOGS.md](https://github.com/Formulary-Labs/.github/blob/main/CATALOGS.md) for known upstream catalog sources.

## Dry run

```go
generate.PipelineConfig{DryRun: true}
```

In dry-run mode, `formula` prints each artifact's file path and row count to stdout without writing any files. Useful for verifying the pipeline configuration before a run.

## License

Apache License 2.0
