package executor

/*
	Windows专用Shell执行器 - 仅支持PowerShell
	本模块专为Windows设计，仅支持以下Shell:
	- PowerShell 7+ (pwsh)
	- Windows PowerShell 5.x (powershell)

	不支持Git Bash、CMD或其他Shell。
*/

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ShellType 定义Shell类型
type ShellType int

const (
	PowerShell7 ShellType = iota
	PowerShell
	Unknown
)

// String 返回Shell类型的字符串表示
func (s ShellType) String() string {
	switch s {
	case PowerShell7:
		return "pwsh"
	case PowerShell:
		return "powershell"
	default:
		return "unknown"
	}
}

// ShellExecutor Shell执行器
type ShellExecutor struct {
	preferredShell ShellType
	shellPaths     map[ShellType]string
}

// NewShellExecutor 创建新的Shell执行器
func NewShellExecutor() *ShellExecutor {
	executor := &ShellExecutor{
		shellPaths: make(map[ShellType]string),
	}

	// 检测可用的Shell
	executor.detectShells()

	return executor
}

// detectShells 检测系统中可用的Shell
func (e *ShellExecutor) detectShells() {
	// Windows Shell detection - 仅支持PowerShell
	// 按优先级检测Shell
	shells := []struct {
		shellType ShellType
		commands  []string
	}{
		{PowerShell7, []string{"pwsh", "pwsh.exe"}},
		{PowerShell, []string{"powershell", "powershell.exe"}},
	}

	for _, shell := range shells {
		for _, cmd := range shell.commands {
			if path, err := exec.LookPath(strings.Trim(cmd, `"`)); err == nil {
				e.shellPaths[shell.shellType] = path
				e.preferredShell = shell.shellType
				return // 找到第一个可用的Shell就停止
			}
		}
	}
}

// GetPreferredShell 获取首选Shell
func (e *ShellExecutor) GetPreferredShell() ShellType {
	return e.preferredShell
}

// GetShellPath 获取指定Shell的路径
func (e *ShellExecutor) GetShellPath(shellType ShellType) string {
	if path, exists := e.shellPaths[shellType]; exists {
		return path
	}
	return ""
}

// ExecuteCommand 使用最佳Shell执行命令
func (e *ShellExecutor) ExecuteCommand(command string, timeout int) (string, int, error) {
	if e.preferredShell == Unknown {
		return "", -1, fmt.Errorf("no suitable shell found")
	}

	return e.ExecuteWithShell(e.preferredShell, command, timeout)
}

// ExecuteWithShell 使用指定Shell执行命令
func (e *ShellExecutor) ExecuteWithShell(shellType ShellType, command string, timeout int) (string, int, error) {
	shellPath, exists := e.shellPaths[shellType]
	if !exists {
		return "", -1, fmt.Errorf("shell %s not available", shellType.String())
	}

	// 准备命令参数
	var args []string
	switch shellType {
	case PowerShell7, PowerShell:
		// PowerShell执行
		args = []string{"-Command", command}
	default:
		return "", -1, fmt.Errorf("unsupported shell type: %s", shellType.String())
	}

	var cmd *exec.Cmd
	ctx := context.Background()
	var cancel context.CancelFunc

	// 设置超时 - 使用正确的 context 机制，便于超时后统一返回
	if timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, time.Duration(timeout)*time.Millisecond)
		defer func() {
			if cancel != nil {
				cancel()
			}
		}()
		cmd = exec.CommandContext(ctx, shellPath, args...)
	} else {
		cmd = exec.Command(shellPath, args...)
	}

	output, err := cmd.CombinedOutput()

	// 优先判断是否为超时：CommandContext 超时后会杀进程，Run 返回的 err 可能是 Wait 的退出错误
	if ctx.Err() == context.DeadlineExceeded {
		outStr := string(output)
		return outStr, -1, fmt.Errorf("command timed out after %dms: %w", timeout, context.DeadlineExceeded)
	}

	exitCode := 0
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			exitCode = exitError.ExitCode()
		} else if !errors.Is(err, context.DeadlineExceeded) {
			exitCode = -1
		}
	}

	return string(output), exitCode, err
}

// GetAvailableShells 获取所有可用的Shell
func (e *ShellExecutor) GetAvailableShells() []ShellType {
	var shells []ShellType
	for shellType := range e.shellPaths {
		shells = append(shells, shellType)
	}
	return shells
}

// PrintShellInfo 打印Shell信息
func (e *ShellExecutor) PrintShellInfo() {
	// MCP协议要求stdout只用于JSON-RPC通信，调试信息输出到stderr
	fmt.Fprintf(os.Stderr, "🔧 检测到的Shell环境 (仅支持PowerShell):\n")
	for i, shellType := range []ShellType{PowerShell7, PowerShell} {
		if path, exists := e.shellPaths[shellType]; exists {
			status := "✅"
			if shellType == e.preferredShell {
				status = "🎯 (首选)"
			}
			fmt.Fprintf(os.Stderr, "%d. %s: %s %s\n", i+1, shellType.String(), path, status)
		} else {
			fmt.Fprintf(os.Stderr, "%d. %s: ❌ 未找到\n", i+1, shellType.String())
		}
	}
}
