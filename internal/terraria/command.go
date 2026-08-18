package terraria

import (
	"context"
	"fmt"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/nickheyer/discopanel/internal/db"
)

// SendCommand sends a command to the Terraria server container
func SendCommand(ctx context.Context, dockerClient *client.Client, server *db.Server, cmd string) (string, error) {
	if server.ContainerID == "" {
		return "", fmt.Errorf("server is not running")
	}

	// For Terraria servers in docker (especially beardedio), commands are often sent to the tmux session or via inject script
	// Let's try docker exec with inject first if available, otherwise just writing to stdin or similar

	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return "", nil
	}

	// We use "inject" command if it exists in the image (common in beardedio/terraria)
	execConfig := container.ExecOptions{
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          []string{"inject", cmd},
	}

	execID, err := dockerClient.ContainerExecCreate(ctx, server.ContainerID, execConfig)
	if err != nil {
		// Fallback to sending to stdin directly if inject isn't used
		return fallbackStdinCommand(ctx, dockerClient, server.ContainerID, cmd)
	}

	err = dockerClient.ContainerExecStart(ctx, execID.ID, container.ExecStartOptions{})
	if err != nil {
		return "", err
	}

	return "Command sent", nil
}

func fallbackStdinCommand(ctx context.Context, dockerClient *client.Client, containerID, cmd string) (string, error) {
	// Normally we would attach and write to stdin
	// This is a simplified placeholder
	attach, err := dockerClient.ContainerAttach(ctx, containerID, container.AttachOptions{
		Stream: true,
		Stdin:  true,
	})
	if err != nil {
		return "", err
	}
	defer attach.Close()

	_, err = attach.Conn.Write([]byte(cmd + "\n"))
	if err != nil {
		return "", err
	}

	return "Command sent to stdin", nil
}
