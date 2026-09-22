// Package generate — standards.go provides gemara-native framework
// configuration for formula. Framework definitions come from gemara
// ControlCatalog artifacts. Artifact selection sets are encoded in Go keyed
// by normalized versions of the catalog's metadata.id field.
package generate

import (
	"fmt"
	"strings"

	"github.com/Formulary-Labs/substrate/artifact"
)

// frameworkArtifactSets maps normalized gemara catalog IDs (metadata.id) to
// the artifact types formula should generate for that framework. Keyed by
// lower-cased, normalized versions of common catalog IDs.
//
// ISO 42001 includes system-card (AI management system standard).
// IEC 62443 omits system-card (OT/ICS, not AI context).
var frameworkArtifactSets = map[string][]ArtifactType{
	// ISO 27001 variants
	"iso27001":  {SOA, RiskCSV, EvidenceRegistry, DependencyMap, ContextDoc, CollectiveRisk, XLSX},
	"iso-27001": {SOA, RiskCSV, EvidenceRegistry, DependencyMap, ContextDoc, CollectiveRisk, XLSX},
	"iso_27001": {SOA, RiskCSV, EvidenceRegistry, DependencyMap, ContextDoc, CollectiveRisk, XLSX},
	// ISO 42001 variants
	"iso42001":  {SOA, RiskCSV, EvidenceRegistry, DependencyMap, ContextDoc, CollectiveRisk, SystemCard, XLSX},
	"iso-42001": {SOA, RiskCSV, EvidenceRegistry, DependencyMap, ContextDoc, CollectiveRisk, SystemCard, XLSX},
	"iso_42001": {SOA, RiskCSV, EvidenceRegistry, DependencyMap, ContextDoc, CollectiveRisk, SystemCard, XLSX},
	// IEC 62443 variants
	"iec62443":  {SOA, RiskCSV, EvidenceRegistry, ContextDoc, CollectiveRisk, XLSX},
	"iec-62443": {SOA, RiskCSV, EvidenceRegistry, ContextDoc, CollectiveRisk, XLSX},
	"iec_62443": {SOA, RiskCSV, EvidenceRegistry, ContextDoc, CollectiveRisk, XLSX},
	// SOC 2 variants
	"soc2":  {SOA, RiskCSV, EvidenceRegistry, ContextDoc, CollectiveRisk, XLSX},
	"soc-2": {SOA, RiskCSV, EvidenceRegistry, ContextDoc, CollectiveRisk, XLSX},
	"soc 2": {SOA, RiskCSV, EvidenceRegistry, ContextDoc, CollectiveRisk, XLSX},
	// NIST 800-53 variants
	"nist800-53":  {SOA, RiskCSV, EvidenceRegistry, DependencyMap, ContextDoc, CollectiveRisk, XLSX},
	"nist-800-53": {SOA, RiskCSV, EvidenceRegistry, DependencyMap, ContextDoc, CollectiveRisk, XLSX},
	"nist_800_53": {SOA, RiskCSV, EvidenceRegistry, DependencyMap, ContextDoc, CollectiveRisk, XLSX},
	// FedRAMP / FINOS CCC variants
	"fedramp":   {SOA, RiskCSV, EvidenceRegistry, DependencyMap, ContextDoc, CollectiveRisk, XLSX},
	"finos-ccc": {SOA, RiskCSV, EvidenceRegistry, DependencyMap, ContextDoc, CollectiveRisk, XLSX},
}

// normalizeFrameworkID lowercases and strips common separators to provide a
// consistent lookup key from arbitrary catalog metadata IDs.
func normalizeFrameworkID(id string) string {
	return strings.ToLower(strings.TrimSpace(id))
}

// ArtifactsFromCatalog loads a gemara ControlCatalog and returns:
//   - the recommended artifact set for its framework (from metadata.id)
//   - the parsed catalog for downstream use by ControlsFromCatalog
//
// Returns (nil, nil, nil) when catalogPath is empty.
// Unknown framework IDs fall back to AllArtifactTypes.
func ArtifactsFromCatalog(catalogPath string) ([]ArtifactType, *artifact.ControlCatalog, error) {
	if catalogPath == "" {
		return nil, nil, nil
	}

	cat, err := artifact.LoadControlCatalog(catalogPath)
	if err != nil {
		return nil, nil, fmt.Errorf("loading catalog %q: %w", catalogPath, err)
	}

	// Framework is identified by the catalog's metadata.id.
	// Metadata is a value type — always accessible.
	normalized := normalizeFrameworkID(cat.Metadata.Id)
	set, ok := frameworkArtifactSets[normalized]
	if !ok {
		// Unknown or missing framework ID — generate everything safe.
		return AllArtifactTypes, cat, nil
	}
	return set, cat, nil
}

// ControlsFromCatalog converts a gemara ControlCatalog into the ControlEntry
// slice that formula's pipeline generators consume.
//
// Catalog controls are the authoritative list. Each control's family is derived
// from its group reference resolved against the catalog's groups. Controls from
// the existing program state are merged in to provide determinations, owners,
// and evidence refs. Program state controls not present in the catalog are
// appended after catalog controls.
func ControlsFromCatalog(cat *artifact.ControlCatalog, existing []ControlEntry) []ControlEntry {
	if cat == nil {
		return existing
	}

	// Build a group ID → title lookup for family resolution.
	groupTitles := make(map[string]string, len(cat.Groups))
	for _, g := range cat.Groups {
		groupTitles[g.Id] = g.Title
	}

	// Index existing program state by control ID for overlay.
	lookup := make(map[string]ControlEntry, len(existing))
	for _, c := range existing {
		lookup[c.ID] = c
	}

	var out []ControlEntry
	for _, ctrl := range cat.Controls {
		entry := ControlEntry{
			ID:     ctrl.Id,
			Title:  ctrl.Title,
			Family: groupTitles[ctrl.Group], // resolve group ID → title
		}
		// Overlay program-specific data where available.
		if ex, ok := lookup[ctrl.Id]; ok {
			entry.Determination = ex.Determination
			entry.Implementation = ex.Implementation
			entry.Owner = ex.Owner
			entry.EvidenceRef = ex.EvidenceRef
			entry.Inherited = ex.Inherited
			entry.InheritedFrom = ex.InheritedFrom
			entry.Excluded = ex.Excluded
			entry.ExclusionJustification = ex.ExclusionJustification
			entry.RiskScore = ex.RiskScore
			entry.Dependencies = ex.Dependencies
			entry.ReviewCadence = ex.ReviewCadence
			delete(lookup, ctrl.Id)
		}
		out = append(out, entry)
	}

	// Append program state entries not present in the catalog.
	for id := range lookup {
		out = append(out, lookup[id])
	}

	return out
}
