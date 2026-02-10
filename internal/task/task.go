package task

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/turinglabs/bobby/internal/config"
)

type Task struct {
	ID          string `json:"id"`
	Config      string `json:"config"`
	MCP         string `json:"mcp,omitempty"`
	Soul        string `json:"soul,omitempty"`
	Description string `json:"description"`
	CreatedAt   string `json:"created_at"`
}

func Create(cfgName, mcpName, soulName, description string) (*Task, error) {
	now := time.Now().UTC()
	id := fmt.Sprintf("%d_%s", now.Unix(), uuid.New().String()[:8])
	t := &Task{
		ID:          id,
		Config:      cfgName,
		MCP:         mcpName,
		Soul:        soulName,
		Description: description,
		CreatedAt:   now.Format(time.RFC3339),
	}

	dir := Dir(id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create task dir: %w", err)
	}

	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, "task.json"), data, 0o644); err != nil {
		return nil, err
	}

	// Create artifacts directory for MCP servers and tools to store files
	if err := os.MkdirAll(filepath.Join(dir, "artifacts"), 0o755); err != nil {
		return nil, fmt.Errorf("create artifacts dir: %w", err)
	}

	for _, f := range []string{"roadmap.md", "status.md", "memory.md", "logs.md"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte(""), 0o644); err != nil {
			return nil, err
		}
	}

	return t, nil
}

func Dir(id string) string {
	return filepath.Join(config.TasksDir(), "task_"+id)
}

func Get(id string) (*Task, error) {
	data, err := os.ReadFile(filepath.Join(Dir(id), "task.json"))
	if err != nil {
		return nil, err
	}
	var t Task
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

func List() ([]*Task, error) {
	entries, err := os.ReadDir(config.TasksDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var tasks []*Task
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "task_") {
			continue
		}
		id := strings.TrimPrefix(e.Name(), "task_")
		t, err := Get(id)
		if err != nil {
			continue
		}
		tasks = append(tasks, t)
	}

	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].CreatedAt > tasks[j].CreatedAt
	})
	return tasks, nil
}

func Remove(id string) error {
	return os.RemoveAll(Dir(id))
}

func ReadStatus(id string) string {
	data, _ := os.ReadFile(filepath.Join(Dir(id), "status.md"))
	s := strings.TrimSpace(string(data))
	if s == "" {
		return "new"
	}
	return s
}

func ReadLogs(id string) string {
	data, _ := os.ReadFile(filepath.Join(Dir(id), "logs.md"))
	return string(data)
}

func ReadFile(id, name string) string {
	data, _ := os.ReadFile(filepath.Join(Dir(id), name))
	return string(data)
}
