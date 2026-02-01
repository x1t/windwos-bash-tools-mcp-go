package executor

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// BashExecutor PowerShell命令执行器
// 仅支持 PowerShell 7+ (pwsh) 和 Windows PowerShell 5.x (powershell)
type BashExecutor struct {
	defaultTimeout time.Duration
}

// NewBashExecutor 创建新的Bash执行器
func NewBashExecutor() *BashExecutor {
	return &BashExecutor{
		defaultTimeout: 10 * time.Second, // 默认10秒超时
	}
}

// Execute 执行PowerShell命令并返回输出、退出码和错误
func (be *BashExecutor) Execute(command string, timeoutMs int) (string, int, error) {
	ctx, cancel := context.WithTimeout(context.Background(),
		time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	// 使用PowerShell执行命令
	cmd := exec.CommandContext(ctx, "powershell", "-Command", command)

	output, err := cmd.CombinedOutput()

	exitCode := 0
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}

	outputStr := string(output)

	if ctx.Err() == context.DeadlineExceeded {
		return outputStr, -1, fmt.Errorf("command timed out after %dms", timeoutMs)
	}

	if err != nil {
		return outputStr, exitCode, fmt.Errorf("command execution failed: %w", err)
	}

	return outputStr, exitCode, nil
}

// ExecuteWithProcess 执行命令并返回输出、退出码、错误和进程对象
func (be *BashExecutor) ExecuteWithProcess(command string, timeoutMs int) (string, int, *os.Process, error) {
	ctx, cancel := context.WithTimeout(context.Background(),
		time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	// 使用PowerShell执行命令
	cmd := exec.CommandContext(ctx, "powershell", "-Command", command)

	output, err := cmd.CombinedOutput()

	exitCode := 0
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}

	outputStr := string(output)

	if ctx.Err() == context.DeadlineExceeded {
		return outputStr, -1, cmd.Process, fmt.Errorf("command timed out after %dms", timeoutMs)
	}

	if err != nil {
		return outputStr, exitCode, cmd.Process, fmt.Errorf("command execution failed: %w", err)
	}

	return outputStr, exitCode, cmd.Process, nil
}

// BackgroundCommandHandle 后台命令句柄，用于管理后台命令的生命周期
type BackgroundCommandHandle struct {
	Process *os.Process
	Cancel  context.CancelFunc
	Done    chan struct{}
}

// Kill 终止后台命令
func (h *BackgroundCommandHandle) Kill() error {
	if h.Cancel != nil {
		h.Cancel()
	}
	if h.Process != nil {
		return h.Process.Kill()
	}
	return nil
}

// StartBackgroundCommand 在后台启动命令并返回命令句柄
// 调用者负责在适当时机调用handle.Kill()或handle.Cancel()来清理资源
func (be *BashExecutor) StartBackgroundCommand(command string, timeoutMs int) (*BackgroundCommandHandle, error) {
	ctx, cancel := context.WithTimeout(context.Background(),
		time.Duration(timeoutMs)*time.Millisecond)

	// 使用PowerShell执行命令
	cmd := exec.CommandContext(ctx, "powershell", "-Command", command)

	if err := cmd.Start(); err != nil {
		cancel() // 启动失败时释放context
		return nil, fmt.Errorf("failed to start command: %w", err)
	}

	// 创建done channel用于通知goroutine退出
	done := make(chan struct{})

	// 启动一个goroutine来处理超时和命令完成
	go func() {
		// 等待命令完成
		cmdDone := make(chan error, 1)
		go func() {
			cmdDone <- cmd.Wait()
		}()

		select {
		case <-ctx.Done():
			// Context被取消或超时
			if cmd.Process != nil {
				cmd.Process.Kill()
			}
			// 等待cmd.Wait()返回
			<-cmdDone
		case <-cmdDone:
			// 命令正常完成
		}
		close(done)
	}()

	return &BackgroundCommandHandle{
		Process: cmd.Process,
		Cancel:  cancel,
		Done:    done,
	}, nil
}

// KillProcess 终止指定的进程
func (be *BashExecutor) KillProcess(process *os.Process) error {
	if process == nil {
		return fmt.Errorf("process is nil")
	}
	return process.Kill()
}

// ExecuteWithStreaming 流式执行命令
func (be *BashExecutor) ExecuteWithStreaming(command string, timeoutMs int,
	onOutput func(string)) (string, int, error) {
	ctx, cancel := context.WithTimeout(context.Background(),
		time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	// 使用PowerShell执行命令
	cmd := exec.CommandContext(ctx, "powershell", "-Command", command)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", -1, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		// 关闭已创建的stdout pipe防止资源泄漏
		stdout.Close()
		return "", -1, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		// 关闭已创建的pipes防止资源泄漏
		stdout.Close()
		stderr.Close()
		return "", -1, fmt.Errorf("failed to start command: %w", err)
	}

	var output strings.Builder
	outputChan := make(chan string, 100)

	// 读取stdout
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			outputChan <- line
			if onOutput != nil {
				onOutput(line)
			}
			output.WriteString(line + "\n")
		}
	}()

	// 读取stderr
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()
			outputChan <- "ERROR: " + line
			if onOutput != nil {
				onOutput("ERROR: " + line)
			}
			output.WriteString("ERROR: " + line + "\n")
		}
	}()

	// 等待命令完成
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		close(outputChan)

		exitCode := 0
		if cmd.ProcessState != nil {
			exitCode = cmd.ProcessState.ExitCode()
		}

		if err != nil {
			return output.String(), exitCode, fmt.Errorf("command execution failed: %w", err)
		}

		return output.String(), exitCode, nil

	case <-ctx.Done():
		// 超时，强制终止进程
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		return output.String(), -1, fmt.Errorf("command timed out after %dms", timeoutMs)
	}
}

// ValidateCommand 验证命令是否安全
func (be *BashExecutor) ValidateCommand(command string) error {
	// 基本的安全检查 - 仅针对PowerShell命令
	dangerousCommands := []string{
		"del ", "rmdir ", "rd ", "format ", "shutdown ", "reboot ",
		"reg delete", "reg add", "net user", "net localgroup",
		"powershell -enc", "powershell -encodedcommand",
	}

	cmdLower := strings.ToLower(command)
	for _, dangerous := range dangerousCommands {
		if strings.Contains(cmdLower, dangerous) {
			return fmt.Errorf("dangerous command detected: %s", dangerous)
		}
	}

	return nil
}
