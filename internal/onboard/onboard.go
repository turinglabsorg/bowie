package onboard

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/turinglabs/bobby/internal/config"
)

var reader *bufio.Reader

func Run() error {
	reader = bufio.NewReader(os.Stdin)

	fmt.Println()
	fmt.Println("=== Bowie — Setup Wizard ===")
	fmt.Println()
	fmt.Println("Bowie is an autonomous agent for testing MCP servers against LLM providers.")
	fmt.Println("This wizard will help you configure your LLM providers and MCP servers.")
	fmt.Println()

	// --- LLM config loop ---
	fmt.Println("--- LLM Configuration ---")
	fmt.Println()
	for {
		if err := setupLLMConfig(); err != nil {
			return err
		}
		fmt.Println()
		if !promptYesNo("Add another LLM config?") {
			break
		}
		fmt.Println()
	}

	// --- Soul config (optional) ---
	fmt.Println()
	fmt.Println("--- Soul Configuration ---")
	fmt.Println()
	fmt.Println("  Souls define the agent's personality and directives.")
	fmt.Println("  A 'default' soul is created automatically (autonomous, proactive).")
	fmt.Println()
	if promptYesNo("Create a custom soul?") {
		fmt.Println()
		for {
			if err := setupSoul(); err != nil {
				return err
			}
			fmt.Println()
			if !promptYesNo("Create another soul?") {
				break
			}
			fmt.Println()
		}
	}

	// --- MCP config loop (optional) ---
	fmt.Println()
	fmt.Println("--- MCP Configuration ---")
	fmt.Println()
	if promptYesNo("Do you want to configure an MCP server?") {
		fmt.Println()
		for {
			if err := setupMCPConfig(); err != nil {
				return err
			}
			fmt.Println()
			if !promptYesNo("Add another MCP?") {
				break
			}
			fmt.Println()
		}
	}

	// --- Summary ---
	printSummary()
	return nil
}

func setupLLMConfig() error {
	providers := []string{"anthropic", "openai", "openrouter", "ollama", "other"}
	provider := promptChoice("Provider", providers)

	cfg := config.LLMConfig{Provider: provider}

	switch provider {
	case "ollama":
		cfg.Endpoint = prompt("Endpoint", "http://host.docker.internal:11434")
		cfg.Model = prompt("Model", "llama3.1")
	case "other":
		cfg.Provider = promptRequired("Provider name (e.g. minimax)")
		cfg.APIKey = promptRequired("API key")
		cfg.BaseURL = promptRequired("Base URL (e.g. https://api.minimax.io/anthropic/v1)")
		cfg.Model = promptRequired("Model name")
	default:
		cfg.APIKey = promptRequired("API key")
		defaultModel := defaultModelFor(provider)
		cfg.Model = prompt("Model", defaultModel)
		baseURL := prompt("Base URL (leave empty for default)", "")
		if baseURL != "" {
			cfg.BaseURL = baseURL
		}
	}

	defaultName := cfg.Provider
	name := prompt("Config name", defaultName)

	if config.ConfigExists(name) {
		fmt.Printf("  Warning: config %q already exists.\n", name)
		if !promptYesNo("  Overwrite?") {
			name = promptRequired("  Choose a different name")
		}
	}

	if err := config.SaveLLMConfig(name, &cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}
	fmt.Printf("  Saved: ~/.bowie/configs/%s.json\n", name)
	return nil
}

func setupMCPConfig() error {
	fmt.Println("  Example: name=factor-mcp, command=npx, args=-y,@anthropic/factor-mcp")
	fmt.Println()

	name := promptRequired("MCP name")
	command := promptRequired("Command (e.g. npx, python, node)")
	argsStr := prompt("Args (comma-separated)", "")
	installCmd := prompt("Install command (e.g. pip install duckduckgo-mcp-server)", "")
	envStr := prompt("Env vars (KEY=VAL,KEY=VAL)", "")

	var args []string
	if argsStr != "" {
		for _, a := range strings.Split(argsStr, ",") {
			a = strings.TrimSpace(a)
			if a != "" {
				args = append(args, a)
			}
		}
	}

	env := make(map[string]string)
	if envStr != "" {
		for _, pair := range strings.Split(envStr, ",") {
			pair = strings.TrimSpace(pair)
			if k, v, ok := strings.Cut(pair, "="); ok {
				env[strings.TrimSpace(k)] = strings.TrimSpace(v)
			}
		}
	}

	cfg := config.MCPConfig{
		Name:    name,
		Command: command,
		Args:    args,
	}
	if len(env) > 0 {
		cfg.Env = env
	}
	if installCmd != "" {
		cfg.Install = installCmd
	}

	if config.MCPExists(name) {
		fmt.Printf("  Warning: MCP %q already exists.\n", name)
		if !promptYesNo("  Overwrite?") {
			name = promptRequired("  Choose a different name")
			cfg.Name = name
		}
	}

	if err := config.SaveMCPConfig(name, &cfg); err != nil {
		return fmt.Errorf("saving MCP config: %w", err)
	}
	fmt.Printf("  Saved: ~/.bowie/mcps/%s.json\n", name)
	return nil
}

func setupSoul() error {
	fmt.Println("  Write the directives for this soul. This is markdown that tells the agent")
	fmt.Println("  how to behave. Use multiple lines — enter an empty line to finish.")
	fmt.Println()

	name := promptRequired("Soul name")

	if config.SoulExists(name) {
		fmt.Printf("  Warning: soul %q already exists.\n", name)
		if !promptYesNo("  Overwrite?") {
			name = promptRequired("  Choose a different name")
		}
	}

	fmt.Println("  Enter directives (empty line to finish):")
	var lines []string
	for {
		line, _ := reader.ReadString('\n')
		line = strings.TrimRight(line, "\n\r")
		if line == "" && len(lines) > 0 {
			break
		}
		lines = append(lines, line)
	}
	content := strings.Join(lines, "\n")

	if err := config.SaveSoul(name, content); err != nil {
		return fmt.Errorf("saving soul: %w", err)
	}
	fmt.Printf("  Saved: ~/.bowie/souls/%s.md\n", name)
	return nil
}

func printSummary() {
	fmt.Println()
	fmt.Println("=== Setup Complete ===")
	fmt.Println()

	configs, _ := config.ListConfigs()
	mcps, _ := config.ListMCPs()
	souls, _ := config.ListSouls()

	if len(configs) > 0 {
		fmt.Println("LLM configs:")
		for _, c := range configs {
			fmt.Printf("  - %s\n", c)
		}
	}
	if len(souls) > 0 {
		fmt.Println("Souls:")
		for _, s := range souls {
			fmt.Printf("  - %s\n", s)
		}
	}
	if len(mcps) > 0 {
		fmt.Println("MCP configs:")
		for _, m := range mcps {
			fmt.Printf("  - %s\n", m)
		}
	}

	fmt.Println()
	if len(configs) > 0 && len(mcps) > 0 {
		fmt.Printf("Example: bowie new --config %s --mcp %s \"test the MCP tools\"\n", configs[0], mcps[0])
	} else if len(configs) > 0 {
		fmt.Printf("Example: bowie new --config %s \"describe your task here\"\n", configs[0])
	}
	fmt.Println()
}

// --- prompt helpers ---

func prompt(label, defaultVal string) string {
	if defaultVal != "" {
		fmt.Printf("  %s [%s]: ", label, defaultVal)
	} else {
		fmt.Printf("  %s: ", label)
	}
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return defaultVal
	}
	return line
}

func promptRequired(label string) string {
	for {
		fmt.Printf("  %s: ", label)
		line, _ := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
		fmt.Println("  (required)")
	}
}

func promptChoice(label string, choices []string) string {
	fmt.Printf("  %s:\n", label)
	for i, c := range choices {
		fmt.Printf("    %d) %s\n", i+1, c)
	}
	for {
		fmt.Print("  Choice: ")
		line, _ := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		n, err := strconv.Atoi(line)
		if err == nil && n >= 1 && n <= len(choices) {
			return choices[n-1]
		}
		// also accept by name
		for _, c := range choices {
			if strings.EqualFold(line, c) {
				return c
			}
		}
		fmt.Printf("  Please enter 1-%d\n", len(choices))
	}
}

func promptYesNo(label string) bool {
	fmt.Printf("  %s (y/n): ", label)
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))
	return line == "y" || line == "yes"
}

func defaultModelFor(provider string) string {
	switch provider {
	case "anthropic":
		return "claude-sonnet-4-5-20250929"
	case "openai":
		return "gpt-4o"
	case "openrouter":
		return "anthropic/claude-sonnet-4-5-20250929"
	default:
		return ""
	}
}
