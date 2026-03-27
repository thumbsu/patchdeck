package navigation

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type JumpTarget struct {
	AbsPath     string
	OpenCommand string
}

func EditorCommand(worktreePath, filePath string) (*exec.Cmd, JumpTarget) {
	absPath := filepath.Join(worktreePath, filePath)
	editor := os.Getenv("EDITOR")
	if strings.TrimSpace(editor) == "" {
		editor = "vim"
	}

	parts := strings.Fields(editor)
	if len(parts) == 0 {
		parts = []string{"vim"}
	}
	parts = append(parts, absPath)

	return exec.Command(parts[0], parts[1:]...), JumpTarget{
		AbsPath:     absPath,
		OpenCommand: strings.Join(parts, " "),
	}
}

func ShellCommand(worktreePath string) (*exec.Cmd, JumpTarget) {
	shell := os.Getenv("SHELL")
	if strings.TrimSpace(shell) == "" {
		shell = "/bin/zsh"
	}

	quotedPath := quoteShell(worktreePath)
	quotedShell := quoteShell(shell)
	command := "cd " + quotedPath + "; exec " + quotedShell + " -l"

	return exec.Command(shell, "-lc", command), JumpTarget{
		AbsPath:     worktreePath,
		OpenCommand: command,
	}
}

func quoteShell(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
