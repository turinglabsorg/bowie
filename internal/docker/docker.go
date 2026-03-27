package docker

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/client"
	"github.com/turinglabs/bobby/internal/config"
	"github.com/turinglabs/bobby/internal/task"
)

const ImageName = "bowie-agent:latest"

type Container struct {
	ID     string
	cli    *client.Client
	stdin  io.WriteCloser
	stdout io.Reader
}

func NewClient() (*client.Client, error) {
	return client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
}

func EnsureImage(ctx context.Context, cli *client.Client) error {
	result, err := cli.ImageList(ctx, client.ImageListOptions{
		Filters: make(client.Filters).Add("reference", ImageName),
	})
	if err != nil {
		return err
	}
	if len(result.Items) > 0 {
		return nil
	}
	return fmt.Errorf("image %s not found — run 'make agent-image' first", ImageName)
}

func Start(ctx context.Context, cli *client.Client, t *task.Task, llmCfg *config.LLMConfig, mcpCfg *config.MCPConfig) (*Container, error) {
	taskDir := task.Dir(t.ID)
	configPath := fmt.Sprintf("%s/%s.json", config.ConfigsDir(), t.Config)
	cacheDir := config.CacheDir()

	containerCfg := &container.Config{
		Image:        ImageName,
		OpenStdin:    true,
		StdinOnce:    false,
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          false,
		Labels: map[string]string{
			"bowie.task.id": t.ID,
		},
	}

	soulName := t.Soul
	if soulName == "" {
		soulName = "default"
	}
	soulPath := fmt.Sprintf("%s/%s.md", config.SoulsDir(), soulName)

	artifactsDir := filepath.Join(taskDir, "artifacts")

	mounts := []mount.Mount{
		{Type: mount.TypeBind, Source: taskDir, Target: "/bowie/task"},
		{Type: mount.TypeBind, Source: configPath, Target: "/bowie/config/config.json", ReadOnly: true},
		{Type: mount.TypeBind, Source: cacheDir, Target: "/bowie/cache"},
		{Type: mount.TypeBind, Source: soulPath, Target: "/bowie/soul/soul.md", ReadOnly: true},
		{Type: mount.TypeBind, Source: artifactsDir, Target: "/bowie/artifacts"},
	}
	if mcpCfg != nil {
		mcpPath := fmt.Sprintf("%s/%s.json", config.MCPsDir(), t.MCP)
		mounts = append(mounts, mount.Mount{Type: mount.TypeBind, Source: mcpPath, Target: "/bowie/mcp/mcp.json", ReadOnly: true})
	}

	hostCfg := &container.HostConfig{
		Mounts:     mounts,
		ExtraHosts: []string{"host.docker.internal:host-gateway"},
	}

	resp, err := cli.ContainerCreate(ctx, client.ContainerCreateOptions{
		Config:     containerCfg,
		HostConfig: hostCfg,
		Name:       "bowie-" + t.ID,
	})
	if err != nil {
		return nil, fmt.Errorf("create container: %w", err)
	}

	c := &Container{ID: resp.ID, cli: cli}

	if err := c.attach(ctx); err != nil {
		cli.ContainerRemove(ctx, c.ID, client.ContainerRemoveOptions{Force: true})
		return nil, err
	}

	if _, err := cli.ContainerStart(ctx, c.ID, client.ContainerStartOptions{}); err != nil {
		cli.ContainerRemove(ctx, c.ID, client.ContainerRemoveOptions{Force: true})
		return nil, fmt.Errorf("start container: %w", err)
	}

	return c, nil
}

func (c *Container) attach(ctx context.Context) error {
	resp, err := c.cli.ContainerAttach(ctx, c.ID, client.ContainerAttachOptions{
		Stream: true,
		Stdin:  true,
		Stdout: true,
		Stderr: true,
	})
	if err != nil {
		return fmt.Errorf("attach container: %w", err)
	}

	c.stdin = resp.Conn

	pr, pw := io.Pipe()
	c.stdout = pr

	go func() {
		stdcopy.StdCopy(pw, pw, resp.Reader)
		pw.Close()
	}()

	return nil
}

func Attach(ctx context.Context, cli *client.Client, containerID string) (*Container, error) {
	c := &Container{ID: containerID, cli: cli}
	if err := c.attach(ctx); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Container) Send(msg map[string]interface{}) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	_, err = c.stdin.Write(append(data, '\n'))
	return err
}

func (c *Container) SendMessage(content string) error {
	return c.Send(map[string]interface{}{
		"type":    "user_message",
		"content": content,
	})
}

func (c *Container) SendShutdown() error {
	return c.Send(map[string]interface{}{"type": "shutdown"})
}

func (c *Container) ReadMessages(ctx context.Context, ch chan<- map[string]interface{}) {
	scanner := bufio.NewScanner(c.stdout)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		var msg map[string]interface{}
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}
		select {
		case ch <- msg:
		case <-ctx.Done():
			return
		}
	}
	close(ch)
}

func (c *Container) Stop(ctx context.Context) error {
	c.SendShutdown()
	timeout := 10
	_, err := c.cli.ContainerStop(ctx, c.ID, client.ContainerStopOptions{Timeout: &timeout})
	return err
}

func (c *Container) Remove(ctx context.Context) error {
	_, err := c.cli.ContainerRemove(ctx, c.ID, client.ContainerRemoveOptions{Force: true})
	return err
}

func RemoveByID(ctx context.Context, cli *client.Client, containerID string) error {
	_, err := cli.ContainerRemove(ctx, containerID, client.ContainerRemoveOptions{Force: true})
	return err
}

func CleanAll(ctx context.Context, cli *client.Client) (int, error) {
	// Find all bowie containers (running or stopped)
	result, err := cli.ContainerList(ctx, client.ContainerListOptions{
		All:     true,
		Filters: make(client.Filters).Add("label", "bowie.task.id"),
	})
	if err != nil {
		return 0, err
	}

	count := 0
	for _, c := range result.Items {
		_, err := cli.ContainerRemove(ctx, c.ID, client.ContainerRemoveOptions{Force: true})
		if err == nil {
			count++
		}
	}

	// Remove the bowie-agent image
	_, _ = cli.ImageRemove(ctx, ImageName, client.ImageRemoveOptions{Force: true})

	return count, nil
}

func FindByTaskID(ctx context.Context, cli *client.Client, taskID string) (string, bool, error) {
	result, err := cli.ContainerList(ctx, client.ContainerListOptions{
		All:     true,
		Filters: make(client.Filters).Add("label", "bowie.task.id="+taskID),
	})
	if err != nil {
		return "", false, err
	}
	if len(result.Items) == 0 {
		return "", false, nil
	}
	c := result.Items[0]
	running := strings.HasPrefix(c.Status, "Up")
	return c.ID, running, nil
}
