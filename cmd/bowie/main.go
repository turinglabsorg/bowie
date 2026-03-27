package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"github.com/turinglabs/bobby/internal/config"
	"github.com/turinglabs/bobby/internal/docker"
	"github.com/turinglabs/bobby/internal/onboard"
	"github.com/turinglabs/bobby/internal/task"
	"github.com/turinglabs/bobby/internal/tui"
)

func main() {
	if err := config.EnsureDirs(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if err := config.EnsureDefaultSoul(); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating default soul: %v\n", err)
		os.Exit(1)
	}

	root := &cobra.Command{
		Use:   "bowie",
		Short: "Bowie — turn and face the strange",
		RunE: func(cmd *cobra.Command, args []string) error {
			m := tui.NewListModel()
			p := tea.NewProgram(m, tea.WithAltScreen())
			_, err := p.Run()
			return err
		},
	}

	var cfgName, mcpName, soulName string
	var headless bool

	newCmd := &cobra.Command{
		Use:   "new [description]",
		Short: "Create a new task and start the agent",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			desc := strings.Join(args, " ")

			if _, err := config.LoadLLMConfig(cfgName); err != nil {
				return fmt.Errorf("config %q not found: %w", cfgName, err)
			}
			if mcpName != "" {
				if _, err := config.LoadMCPConfig(mcpName); err != nil {
					return fmt.Errorf("mcp %q not found: %w", mcpName, err)
				}
			}
			if soulName != "" {
				if !config.SoulExists(soulName) {
					return fmt.Errorf("soul %q not found", soulName)
				}
			}

			t, err := task.Create(cfgName, mcpName, soulName, desc)
			if err != nil {
				return fmt.Errorf("create task: %w", err)
			}

			if headless {
				return startHeadless(t)
			}

			fmt.Printf("Created task %s\n", t.ID)
			return startAndAttach(t)
		},
	}
	newCmd.Flags().StringVar(&cfgName, "config", "", "LLM config name (required)")
	newCmd.Flags().StringVar(&mcpName, "mcp", "", "MCP config name (optional)")
	newCmd.Flags().StringVar(&soulName, "soul", "", "Soul/persona name (default: default)")
	newCmd.Flags().BoolVar(&headless, "headless", false, "Start without TUI, print task ID and exit")
	newCmd.MarkFlagRequired("config")

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all tasks",
		RunE: func(cmd *cobra.Command, args []string) error {
			tasks, err := task.List()
			if err != nil {
				return err
			}
			if len(tasks) == 0 {
				fmt.Println("No tasks.")
				return nil
			}

			ctx := context.Background()
			cli, err := docker.NewClient()
			if err != nil {
				return err
			}
			defer cli.Close()

			for _, t := range tasks {
				_, running, _ := docker.FindByTaskID(ctx, cli, t.ID)
				status := "stopped"
				if running {
					status = "running"
				}
				ts := formatTaskTime(t.ID)
				fmt.Printf("  %s  %-14s  %-8s  %s\n", shortTaskID(t.ID), ts, status, truncate(t.Description, 60))
			}
			return nil
		},
	}

	attachCmd := &cobra.Command{
		Use:   "attach [task_id]",
		Short: "Attach to a running task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			taskID := args[0]
			t, err := task.Get(taskID)
			if err != nil {
				return fmt.Errorf("task not found: %w", err)
			}

			ctx := context.Background()
			cli, err := docker.NewClient()
			if err != nil {
				return err
			}
			defer cli.Close()

			containerID, running, err := docker.FindByTaskID(ctx, cli, taskID)
			if err != nil {
				return err
			}

			if !running {
				fmt.Println("Container not running, starting...")
				return startAndAttach(t)
			}

			c, err := docker.Attach(ctx, cli, containerID)
			if err != nil {
				return fmt.Errorf("attach: %w", err)
			}

			m := tui.NewChatModel(c, taskID, t.Description)
			p := tea.NewProgram(m, tea.WithAltScreen())
			_, err = p.Run()
			return err
		},
	}

	stopCmd := &cobra.Command{
		Use:   "stop [task_id]",
		Short: "Stop a running task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			cli, err := docker.NewClient()
			if err != nil {
				return err
			}
			defer cli.Close()

			containerID, running, err := docker.FindByTaskID(ctx, cli, args[0])
			if err != nil {
				return err
			}
			if !running {
				fmt.Println("Not running.")
				return nil
			}

			c, err := docker.Attach(ctx, cli, containerID)
			if err != nil {
				return err
			}
			return c.Stop(ctx)
		},
	}

	restartCmd := &cobra.Command{
		Use:   "restart [task_id]",
		Short: "Restart a task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			taskID := args[0]
			t, err := task.Get(taskID)
			if err != nil {
				return fmt.Errorf("task not found: %w", err)
			}

			ctx := context.Background()
			cli, err := docker.NewClient()
			if err != nil {
				return err
			}
			defer cli.Close()

			containerID, running, _ := docker.FindByTaskID(ctx, cli, taskID)
			if containerID != "" {
				if running {
					c, err := docker.Attach(ctx, cli, containerID)
					if err == nil {
						c.Stop(ctx)
						c.Remove(ctx)
					}
				} else {
					docker.RemoveByID(ctx, cli, containerID)
				}
			}

			return startAndAttach(t)
		},
	}

	logsCmd := &cobra.Command{
		Use:   "logs [task_id]",
		Short: "Show task logs",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			logs := task.ReadLogs(args[0])
			if logs == "" {
				fmt.Println("No logs.")
			} else {
				fmt.Print(logs)
			}
			return nil
		},
	}

	rmCmd := &cobra.Command{
		Use:   "rm [task_id]",
		Short: "Remove a task and its data",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			taskID := args[0]
			ctx := context.Background()
			cli, err := docker.NewClient()
			if err == nil {
				defer cli.Close()
				containerID, running, _ := docker.FindByTaskID(ctx, cli, taskID)
				if containerID != "" {
					if running {
						c, err := docker.Attach(ctx, cli, containerID)
						if err == nil {
							c.Stop(ctx)
							c.Remove(ctx)
						}
					} else {
						docker.RemoveByID(ctx, cli, containerID)
					}
				}
			}
			return task.Remove(taskID)
		},
	}

	onboardCmd := &cobra.Command{
		Use:   "onboard",
		Short: "Interactive setup wizard for LLM and MCP configs",
		RunE: func(cmd *cobra.Command, args []string) error {
			return onboard.Run()
		},
	}

	sendCmd := &cobra.Command{
		Use:   "send [task_id] [message]",
		Short: "Send a message to a running agent and print the response",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			taskID := args[0]
			message := strings.Join(args[1:], " ")

			ctx := context.Background()
			cli, err := docker.NewClient()
			if err != nil {
				return err
			}
			defer cli.Close()

			containerID, running, err := docker.FindByTaskID(ctx, cli, taskID)
			if err != nil {
				return err
			}
			if !running {
				return fmt.Errorf("task %s is not running", taskID)
			}

			c, err := docker.Attach(ctx, cli, containerID)
			if err != nil {
				return fmt.Errorf("attach: %w", err)
			}

			ch := make(chan map[string]interface{}, 64)
			readCtx, readCancel := context.WithCancel(ctx)
			defer readCancel()
			go c.ReadMessages(readCtx, ch)

			if err := c.SendMessage(message); err != nil {
				return fmt.Errorf("send: %w", err)
			}

			// Wait for response — collect until agent goes idle
			timeout := time.After(5 * time.Minute)
			for {
				select {
				case msg, ok := <-ch:
					if !ok {
						return nil
					}
					msgType, _ := msg["type"].(string)
					switch msgType {
					case "agent_response":
						content, _ := msg["content"].(string)
						fmt.Println(content)
					case "tool_call":
						tool, _ := msg["tool"].(string)
						fmt.Fprintf(os.Stderr, "[tool] %s\n", tool)
					case "error":
						content, _ := msg["content"].(string)
						fmt.Fprintf(os.Stderr, "Error: %s\n", content)
					case "status":
						state, _ := msg["state"].(string)
						if state == "idle" {
							return nil
						}
					}
				case <-timeout:
					return fmt.Errorf("timeout waiting for response")
				}
			}
		},
	}

	readCmd := &cobra.Command{
		Use:   "read [task_id] [file]",
		Short: "Read a task file (status, roadmap, memory, logs)",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			taskID := args[0]
			if _, err := task.Get(taskID); err != nil {
				return fmt.Errorf("task not found: %w", err)
			}

			file := ""
			if len(args) > 1 {
				file = args[1]
			}

			validFiles := map[string]string{
				"status":  "status.md",
				"roadmap": "roadmap.md",
				"memory":  "memory.md",
				"logs":    "logs.md",
			}

			if file == "" {
				// Print all files
				for label, fname := range validFiles {
					content := task.ReadFile(taskID, fname)
					fmt.Printf("=== %s ===\n%s\n\n", label, strings.TrimSpace(content))
				}
				return nil
			}

			fname, ok := validFiles[file]
			if !ok {
				return fmt.Errorf("unknown file %q — use: status, roadmap, memory, logs", file)
			}
			content := task.ReadFile(taskID, fname)
			fmt.Print(content)
			return nil
		},
	}

	cleanCmd := &cobra.Command{
		Use:   "clean",
		Short: "Remove all Bowie containers and the agent image",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			cli, err := docker.NewClient()
			if err != nil {
				return err
			}
			defer cli.Close()

			count, err := docker.CleanAll(ctx, cli)
			if err != nil {
				return err
			}
			if count > 0 {
				fmt.Printf("Removed %d container(s)\n", count)
			} else {
				fmt.Println("No containers to remove")
			}
			fmt.Println("Done")
			return nil
		},
	}

	root.AddCommand(newCmd, listCmd, attachCmd, stopCmd, restartCmd, logsCmd, rmCmd, onboardCmd, sendCmd, readCmd, cleanCmd)

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func startHeadless(t *task.Task) error {
	ctx := context.Background()
	cli, err := docker.NewClient()
	if err != nil {
		return err
	}
	defer cli.Close()

	if err := docker.EnsureImage(ctx, cli); err != nil {
		return err
	}

	llmCfg, err := config.LoadLLMConfig(t.Config)
	if err != nil {
		return err
	}
	var mcpCfg *config.MCPConfig
	if t.MCP != "" {
		mcpCfg, err = config.LoadMCPConfig(t.MCP)
		if err != nil {
			return err
		}
	}

	_, err = docker.Start(ctx, cli, t, llmCfg, mcpCfg)
	if err != nil {
		return fmt.Errorf("start container: %w", err)
	}

	fmt.Println(t.ID)
	return nil
}

func startAndAttach(t *task.Task) error {
	ctx := context.Background()
	cli, err := docker.NewClient()
	if err != nil {
		return err
	}
	defer cli.Close()

	if err := docker.EnsureImage(ctx, cli); err != nil {
		return err
	}

	llmCfg, err := config.LoadLLMConfig(t.Config)
	if err != nil {
		return err
	}
	var mcpCfg *config.MCPConfig
	if t.MCP != "" {
		mcpCfg, err = config.LoadMCPConfig(t.MCP)
		if err != nil {
			return err
		}
	}

	c, err := docker.Start(ctx, cli, t, llmCfg, mcpCfg)
	if err != nil {
		return fmt.Errorf("start container: %w", err)
	}

	m := tui.NewChatModel(c, t.ID, t.Description)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err = p.Run()
	return err
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}

func shortTaskID(id string) string {
	parts := strings.SplitN(id, "_", 2)
	if len(parts) == 2 {
		return parts[1]
	}
	return id
}

func formatTaskTime(id string) string {
	parts := strings.SplitN(id, "_", 2)
	if len(parts) < 2 {
		return ""
	}
	ts, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return ""
	}
	t := time.Unix(ts, 0)
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		return fmt.Sprintf("%d min ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		return fmt.Sprintf("%d hours ago", h)
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "yesterday"
		}
		return fmt.Sprintf("%d days ago", days)
	}
}
