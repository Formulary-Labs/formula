package generate

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// StandardConfig is the schema for a per-framework artifact selection config.
// Config files are JSON, named {framework}.json, and live in StandardsDir.
// They are not embedded in the binary — they come from gemara layer 1 artifacts
// or operator-managed config directories.
//
// Example file at standards/my-framework.json:
//
//	{
//	  "id": "my-framework",
//	  "artifacts": ["soa", "risk", "evidence-registry", "xlsx"]
//	}
type StandardConfig struct {
	ID        string   `json:"id"`
	Artifacts []string `json:"artifacts"`
}

// LoadStandardConfig reads the artifact list for the given framework from dir.
// It looks for {dir}/{framework}.json (case-insensitive filename match).
//
// Returns (nil, nil) when no config file exists for the framework — the caller
// should fall back to AllArtifactTypes. Returns a non-nil error only when a
// config file is found but cannot be parsed.
func LoadStandardConfig(framework, dir string) ([]ArtifactType, error) {
	if framework == "" || dir == "" {
		return nil, nil
	}

	// Try exact filename first, then lowercase.
	candidates := []string{
		filepath.Join(dir, framework+".json"),
		filepath.Join(dir, strings.ToLower(framework)+".json"),
	}

	var data []byte
	var readErr error
	for _, path := range candidates {
		data, readErr = os.ReadFile(path)
		if readErr == nil {
			break
		}
		if !errors.Is(readErr, os.ErrNotExist) {
			return nil, fmt.Errorf("reading standards config %q: %w", path, readErr)
		}
	}
	if data == nil {
		return nil, nil // no config file found — not an error
	}

	var sc StandardConfig
	if err := json.Unmarshal(data, &sc); err != nil {
		return nil, fmt.Errorf("parsing standards config for %q: %w", framework, err)
	}

	artifacts := make([]ArtifactType, 0, len(sc.Artifacts))
	for _, a := range sc.Artifacts {
		artifacts = append(artifacts, ArtifactType(a))
	}
	return artifacts, nil
}
