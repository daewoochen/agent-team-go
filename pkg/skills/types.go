package skills

type Manifest struct {
	Name         string       `yaml:"name"`
	Version      string       `yaml:"version"`
	Description  string       `yaml:"description"`
	Permissions  []string     `yaml:"permissions"`
	Entrypoints  []string     `yaml:"entrypoints"`
	Dependencies []Dependency `yaml:"dependencies"`
	Source       SourceInfo   `yaml:"source"`
}

type Dependency struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version"`
}

type SourceInfo struct {
	Type string `yaml:"type"`
	Ref  string `yaml:"ref,omitempty"`
	URL  string `yaml:"url,omitempty"`
}

type InstallResult struct {
	Name       string
	InstallDir string
	Installed  bool
}
