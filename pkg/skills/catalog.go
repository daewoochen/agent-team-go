package skills

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type CatalogEntry struct {
	Name        string
	Description string
	Source      string
}

type InstalledSkill struct {
	Name        string
	Version     string
	Description string
	Path        string
}

func Catalog() []CatalogEntry {
	registry := builtinRegistry()
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)

	entries := make([]CatalogEntry, 0, len(names))
	for _, name := range names {
		entries = append(entries, CatalogEntry{
			Name:        name,
			Description: registry[name],
			Source:      "builtin",
		})
	}
	return entries
}

func Search(query string) []CatalogEntry {
	query = strings.ToLower(strings.TrimSpace(query))
	entries := Catalog()
	if query == "" {
		return entries
	}

	filtered := make([]CatalogEntry, 0, len(entries))
	for _, entry := range entries {
		if strings.Contains(strings.ToLower(entry.Name), query) || strings.Contains(strings.ToLower(entry.Description), query) {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

func (i *Installer) ListInstalled() ([]InstalledSkill, error) {
	entries, err := os.ReadDir(i.BaseDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	out := make([]InstalledSkill, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		dir := filepath.Join(i.BaseDir, entry.Name())
		manifestPath := filepath.Join(dir, manifestFile)
		content, err := os.ReadFile(manifestPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}

		var manifest Manifest
		if err := yaml.Unmarshal(content, &manifest); err != nil {
			return nil, err
		}

		out = append(out, InstalledSkill{
			Name:        manifest.Name,
			Version:     manifest.Version,
			Description: manifest.Description,
			Path:        dir,
		})
	}

	sort.Slice(out, func(a, b int) bool {
		return out[a].Name < out[b].Name
	})
	return out, nil
}
