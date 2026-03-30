package skills

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/daewoochen/agent-team-go/pkg/spec"
)

const manifestFile = "skill.yaml"

type Installer struct {
	BaseDir string
}

func NewInstaller(baseDir string) *Installer {
	return &Installer{BaseDir: baseDir}
}

func (i *Installer) EnsureFromTeam(team *spec.TeamSpec) ([]InstallResult, error) {
	requirements := team.RequiredSkillRequirements()
	sort.Slice(requirements, func(a, b int) bool {
		return requirements[a].Name < requirements[b].Name
	})

	results := make([]InstallResult, 0, len(requirements))
	for _, req := range requirements {
		result, err := i.Install(req, team.BaseDir)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

func (i *Installer) Install(req spec.SkillRequirement, teamBaseDir string) (InstallResult, error) {
	if err := os.MkdirAll(i.BaseDir, 0o755); err != nil {
		return InstallResult{}, err
	}

	targetDir := filepath.Join(i.BaseDir, req.Name)
	manifestPath := filepath.Join(targetDir, manifestFile)
	if _, err := os.Stat(manifestPath); err == nil {
		return InstallResult{Name: req.Name, InstallDir: targetDir, Installed: false}, nil
	}

	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return InstallResult{}, err
	}

	switch req.Source.Type {
	case "local":
		resolvedPath := req.Source.Path
		if !filepath.IsAbs(resolvedPath) {
			resolvedPath = filepath.Join(teamBaseDir, resolvedPath)
		}
		if err := copyDir(resolvedPath, targetDir); err != nil {
			return InstallResult{}, err
		}
	case "registry":
		if err := i.installFromRegistry(req, targetDir); err != nil {
			return InstallResult{}, err
		}
	case "git":
		if err := i.installPlaceholder(req, targetDir, fmt.Sprintf("Installed from %s", req.Source.URL)); err != nil {
			return InstallResult{}, err
		}
	default:
		return InstallResult{}, fmt.Errorf("unsupported skill source %q", req.Source.Type)
	}

	return InstallResult{Name: req.Name, InstallDir: targetDir, Installed: true}, nil
}

func (i *Installer) installFromRegistry(req spec.SkillRequirement, targetDir string) error {
	if builtin, ok := builtinRegistry()[req.Name]; ok {
		return i.installPlaceholder(req, targetDir, builtin)
	}
	return i.installPlaceholder(req, targetDir, "Installed from registry placeholder")
}

func (i *Installer) installPlaceholder(req spec.SkillRequirement, targetDir, description string) error {
	manifest := Manifest{
		Name:        req.Name,
		Version:     defaultVersion(req.Version),
		Description: description,
		Permissions: []string{"read:workspace"},
		Entrypoints: []string{"prompt.md"},
		Source: SourceInfo{
			Type: req.Source.Type,
			Ref:  req.Source.Ref,
			URL:  req.Source.URL,
		},
	}
	if manifest.Source.Type == "registry" {
		manifest.Source.Ref = req.Source.Registry
	}

	content, err := yaml.Marshal(manifest)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(targetDir, manifestFile), content, 0o644); err != nil {
		return err
	}

	prompt := fmt.Sprintf("# %s\n\n%s.\n", req.Name, description)
	return os.WriteFile(filepath.Join(targetDir, "prompt.md"), []byte(prompt), 0o644)
}

func builtinRegistry() map[string]string {
	return map[string]string{
		"github":             "Git and GitHub coordination skill",
		"research-kit":       "Structured research and fact collection skill",
		"telegram-messenger": "Telegram delivery and notification skill",
		"feishu-messenger":   "Feishu delivery and notification skill",
	}
}

func defaultVersion(version string) string {
	if strings.TrimSpace(version) == "" || version == "latest" {
		return "0.1.0"
	}
	return version
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target, info.Mode())
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return os.Chmod(dst, mode)
}
