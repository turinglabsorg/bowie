package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

func Dir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".bowie")
}

func ConfigsDir() string { return filepath.Join(Dir(), "configs") }
func MCPsDir() string    { return filepath.Join(Dir(), "mcps") }
func TasksDir() string   { return filepath.Join(Dir(), "tasks") }
func CacheDir() string   { return filepath.Join(Dir(), "cache") }
func SoulsDir() string   { return filepath.Join(Dir(), "souls") }

func EnsureDirs() error {
	for _, d := range []string{ConfigsDir(), MCPsDir(), TasksDir(), CacheDir(), SoulsDir()} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	return nil
}

type LLMConfig struct {
	Provider string `json:"provider"`
	APIKey   string `json:"api_key,omitempty"`
	Endpoint string `json:"endpoint,omitempty"`
	BaseURL  string `json:"base_url,omitempty"`
	Model    string `json:"model"`
}

type MCPConfig struct {
	Name    string            `json:"name"`
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env,omitempty"`
	Install string            `json:"install,omitempty"`
}

func LoadLLMConfig(name string) (*LLMConfig, error) {
	path := filepath.Join(ConfigsDir(), name+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg LLMConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func LoadMCPConfig(name string) (*MCPConfig, error) {
	path := filepath.Join(MCPsDir(), name+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg MCPConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func ListConfigs() ([]string, error) {
	return listJSONFiles(ConfigsDir())
}

func ListMCPs() ([]string, error) {
	return listJSONFiles(MCPsDir())
}

func SaveLLMConfig(name string, cfg *LLMConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(ConfigsDir(), name+".json"), data, 0o644)
}

func SaveMCPConfig(name string, cfg *MCPConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(MCPsDir(), name+".json"), data, 0o644)
}

func ConfigExists(name string) bool {
	_, err := os.Stat(filepath.Join(ConfigsDir(), name+".json"))
	return err == nil
}

func MCPExists(name string) bool {
	_, err := os.Stat(filepath.Join(MCPsDir(), name+".json"))
	return err == nil
}

func SaveSoul(name string, content string) error {
	return os.WriteFile(filepath.Join(SoulsDir(), name+".md"), []byte(content), 0o644)
}

func LoadSoul(name string) (string, error) {
	data, err := os.ReadFile(filepath.Join(SoulsDir(), name+".md"))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func SoulExists(name string) bool {
	_, err := os.Stat(filepath.Join(SoulsDir(), name+".md"))
	return err == nil
}

func ListSouls() ([]string, error) {
	entries, err := os.ReadDir(SoulsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".md" {
			names = append(names, e.Name()[:len(e.Name())-3])
		}
	}
	return names, nil
}

func EnsureDefaultSoul() error {
	if SoulExists("default") {
		return nil
	}
	return SaveSoul("default", DefaultSoulContent)
}

const DefaultSoulContent = `## Directives

- Be maximally autonomous. Do not ask the user for clarification — make reasonable assumptions and proceed.
- If something is ambiguous, pick the most sensible option and move forward. Document your assumptions in memory.md.
- Only ask the user a question if you are completely blocked and cannot make progress any other way.
- Execute tools proactively. If you have the tools to accomplish a step, just do it.
- When you hit an error, try to recover on your own: retry with different parameters, try an alternative approach, or skip and move to the next step.
- Keep the user informed with short progress updates, not questions.
`

func listJSONFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
			names = append(names, e.Name()[:len(e.Name())-5])
		}
	}
	return names, nil
}
