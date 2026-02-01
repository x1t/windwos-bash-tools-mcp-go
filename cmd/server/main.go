package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"mcp-bash-tools/internal/executor"
	"mcp-bash-tools/internal/security"
	"mcp-bash-tools/internal/windows"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// 常量定义
const (
	// 超时配置
	DefaultTimeoutMs = 30000  // 默认超时时间（毫秒）
	MinTimeoutMs     = 1000   // 最小超时时间（毫秒）
	MaxTimeoutMs     = 600000 // 最大超时时间（毫秒）

	// 任务配置
	MaxShellIDLength   = 100   // Shell ID 最大长度
	MaxBashIDLength    = 100   // Bash ID 最大长度
	MaxBackgroundTasks = 50    // 最大后台任务数
	MaxCommandLength   = 10000 // 最大命令长度（字符）

	// 超时等待配置
	DoneChannelTimeout = 5 * time.Second // done channel 等待超时
)

// NewShellExecutor 创建实际的ShellExecutor
func NewShellExecutor() ShellExecutorInterface {
	return executor.NewShellExecutor()
}

// BashArguments 定义Bash工具的输入参数 - 使用官方标准命名
type BashArguments struct {
	Command         string `json:"command" jsonschema:"要执行的PowerShell命令"`
	Timeout         int    `json:"timeout" jsonschema:"命令超时时间(毫秒),必填,范围1000-600000"`
	Description     string `json:"description,omitempty" jsonschema:"命令描述,用于日志记录"`
	RunInBackground bool   `json:"run_in_background,omitempty" jsonschema:"是否在后台执行命令"`
}

// BashResult 定义Bash工具的输出结果 - 使用官方标准命名
type BashResult struct {
	Output   string `json:"output" jsonschema:"命令执行输出内容"`
	ExitCode int    `json:"exitCode" jsonschema:"命令退出代码"`
	Killed   bool   `json:"killed,omitempty" jsonschema:"命令是否被强制终止"`
	ShellID  string `json:"shellId,omitempty" jsonschema:"后台任务的Shell ID"`
}

// BashOutputArguments 定义BashOutput工具的输入参数
type BashOutputArguments struct {
	BashID string `json:"bash_id" jsonschema:"后台任务的Bash ID"`
	Filter string `json:"filter,omitempty" jsonschema:"正则表达式过滤器,用于筛选输出内容"`
}

// BashOutputResult 定义BashOutput工具的输出结果
type BashOutputResult struct {
	Output   string `json:"output" jsonschema:"后台任务的输出内容"`
	Status   string `json:"status" jsonschema:"任务状态(running,completed,failed,killed)"`
	ExitCode *int   `json:"exitCode,omitempty" jsonschema:"任务退出代码(仅任务完成时有效)"`
}

// KillShellArguments 定义KillShell工具的输入参数
type KillShellArguments struct {
	ShellID string `json:"shell_id" jsonschema:"要终止的后台任务Shell ID"`
}

// KillShellResult 定义KillShell工具的输出结果
type KillShellResult struct {
	Message string `json:"message" jsonschema:"操作结果消息"`
	ShellID string `json:"shell_id" jsonschema:"被终止的任务Shell ID"`
}

// BackgroundTask 表示一个后台任务
type BackgroundTask struct {
	ID        string             `json:"id"`
	Command   string             `json:"command"`
	Output    string             `json:"output"`
	Status    string             `json:"status"` // running, completed, failed, killed
	StartTime time.Time          `json:"startTime"`
	Error     string             `json:"error,omitempty"`
	ExitCode  *int               `json:"exitCode,omitempty"`
	TempFile  string             `json:"tempFile,omitempty"` // 临时文件路径用于存储输出
	Process   *os.Process        `json:"-"`                  // 进程句柄，用于终止进程
	Cancel    context.CancelFunc `json:"-"`                  // Context取消函数，用于终止命令
	Job       *windows.JobObject `json:"-"`                  // Windows Job Object，用于管理进程树
}

// ShellExecutorInterface 定义Shell执行器接口
type ShellExecutorInterface interface {
	ExecuteCommand(command string, timeout int) (string, int, error)
	PrintShellInfo()
}

// MCPServer MCP服务器结构
type MCPServer struct {
	backgroundTasks map[string]*BackgroundTask
	mutex           sync.RWMutex
	shellExecutor   ShellExecutorInterface
}

// NewMCPServer 创建新的MCP服务器
func NewMCPServer() *MCPServer {
	return &MCPServer{
		backgroundTasks: make(map[string]*BackgroundTask),
		shellExecutor:   NewShellExecutor(), // 使用实际的ShellExecutor
	}
}

// BashHandler 处理Bash命令执行 - 使用官方标准Handler签名
func (s *MCPServer) BashHandler(ctx context.Context, req *mcp.CallToolRequest, args BashArguments) (*mcp.CallToolResult, BashResult, error) {
	// 参数验证
	if args.Command == "" {
		errorMsg := "command is required"
		return nil, BashResult{
			ExitCode: 1,
			Output:   errorMsg,
		}, fmt.Errorf("%s", errorMsg)
	}

	// 命令长度验证
	if len(args.Command) > MaxCommandLength {
		errorMsg := fmt.Sprintf("command too long (max %d characters), got: %d", MaxCommandLength, len(args.Command))
		return nil, BashResult{
			ExitCode: 1,
			Output:   errorMsg,
		}, fmt.Errorf("%s", errorMsg)
	}

	if args.Timeout == 0 {
		errorMsg := fmt.Sprintf("timeout is required and must be between %d and %d milliseconds", MinTimeoutMs, MaxTimeoutMs)
		return nil, BashResult{
			ExitCode: 1,
			Output:   errorMsg,
		}, fmt.Errorf("%s", errorMsg)
	}

	if args.Timeout < MinTimeoutMs || args.Timeout > MaxTimeoutMs {
		errorMsg := fmt.Sprintf("timeout must be between %d and %d milliseconds, got: %d", MinTimeoutMs, MaxTimeoutMs, args.Timeout)
		return nil, BashResult{
			ExitCode: 1,
			Output:   errorMsg,
		}, fmt.Errorf("%s", errorMsg)
	}

	// 安全检查
	if security.IsDangerousCommand(args.Command) {
		errorMsg := fmt.Sprintf("command rejected for security reasons: %s", args.Command)
		return nil, BashResult{
			ExitCode: 1,
			Output:   errorMsg,
		}, fmt.Errorf("%s", errorMsg)
	}

	// 日志记录
	logMsg := args.Description
	if logMsg == "" {
		logMsg = args.Command
	}
	fmt.Fprintf(os.Stderr, "Executing command: %s\n", logMsg)

	if args.RunInBackground {
		// 检查后台任务数量限制
		s.mutex.RLock()
		taskCount := len(s.backgroundTasks)
		s.mutex.RUnlock()

		if taskCount >= MaxBackgroundTasks {
			errorMsg := fmt.Sprintf("maximum background tasks limit reached (%d/%d)", taskCount, MaxBackgroundTasks)
			return nil, BashResult{
				ExitCode: 1,
				Output:   errorMsg,
			}, fmt.Errorf("%s", errorMsg)
		}

		// 后台执行 - 不设置超时限制
		s.mutex.Lock()
		// 使用UUID保证全局唯一性
		taskID := fmt.Sprintf("bash_%s", uuid.New().String())
		task := &BackgroundTask{
			ID:        taskID,
			Command:   args.Command,
			StartTime: time.Now(),
			Status:    "running",
		}
		s.backgroundTasks[taskID] = task

		// 启动后台任务（传入0表示无超时限制）
		go s.executeBackgroundCommand(task, 0)
		s.mutex.Unlock()

		// 返回结果
		return nil, BashResult{
			ExitCode: 0,
			ShellID:  taskID,
			Output:   fmt.Sprintf("Background task started with ID: %s", taskID),
		}, nil
	}

	// 前台执行 - 带超时，超时后自动转后台
	resultChan := make(chan struct {
		output   string
		exitCode int
		err      error
	}, 1)

	// 在goroutine中执行命令
	go func() {
		output, exitCode, err := s.shellExecutor.ExecuteCommand(args.Command, args.Timeout)
		resultChan <- struct {
			output   string
			exitCode int
			err      error
		}{output, exitCode, err}
	}()

	// 等待结果或超时
	select {
	case result := <-resultChan:
		// 命令在超时前完成
		killed := false
		if result.err != nil {
			errStr := result.err.Error()
			if strings.Contains(errStr, "killed") ||
				strings.Contains(errStr, "timed out") ||
				strings.Contains(errStr, "context deadline exceeded") {
				killed = true
			}
		}

		if result.err != nil && !killed {
			errorOutput := result.output
			if errorOutput == "" {
				errorOutput = fmt.Sprintf("command execution failed: %v", result.err)
			} else {
				errorOutput = fmt.Sprintf("%s\nError: %v", result.output, result.err)
			}

			return nil, BashResult{
				Output:   errorOutput,
				ExitCode: result.exitCode,
				Killed:   killed,
			}, nil
		}

		// 成功返回
		return nil, BashResult{
			Output:   result.output,
			ExitCode: result.exitCode,
			Killed:   killed,
		}, nil

	case <-time.After(time.Duration(args.Timeout) * time.Millisecond):
		// 超时！自动转为后台任务
		taskID := fmt.Sprintf("bash_%s", uuid.New().String())

		task := &BackgroundTask{
			ID:        taskID,
			Command:   args.Command,
			Status:    "running",
			StartTime: time.Now(),
			Output:    fmt.Sprintf("Task exceeded timeout (%dms), converted to background execution\n", args.Timeout),
		}

		s.mutex.Lock()
		s.backgroundTasks[taskID] = task
		s.mutex.Unlock()

		// 继续监控任务完成（任务实际上还在执行）
		go func() {
			result := <-resultChan

			s.mutex.Lock()
			if task, exists := s.backgroundTasks[taskID]; exists {
				task.Output += result.output
				task.ExitCode = &result.exitCode
				if result.err != nil {
					task.Status = "failed"
					task.Error = result.err.Error()
				} else {
					task.Status = "completed"
				}
			}
			s.mutex.Unlock()
		}()

		// 立即返回，告诉用户任务已转后台
		return nil, BashResult{
			Output:   fmt.Sprintf("⏱️ Command exceeded timeout (%dms), automatically converted to background task.\n\n✅ Task ID: %s\n\n💡 Use 'bash_output' tool with bash_id='%s' to check progress.\n💡 Use 'kill_shell' tool with shell_id='%s' to terminate if needed.", args.Timeout, taskID, taskID, taskID),
			ExitCode: 0,
			ShellID:  taskID,
			Killed:   false,
		}, nil
	}
}

// BashOutputHandler 处理BashOutput工具调用 - 使用官方标准Handler签名
func (s *MCPServer) BashOutputHandler(ctx context.Context, req *mcp.CallToolRequest, args BashOutputArguments) (*mcp.CallToolResult, BashOutputResult, error) {
	if args.BashID == "" {
		return nil, BashOutputResult{
			Status: "failed",
		}, fmt.Errorf("bash_id is required")
	}

	if len(args.BashID) > MaxBashIDLength {
		errorMsg := fmt.Sprintf("bash_id is too long (max %d characters), got: %d", MaxBashIDLength, len(args.BashID))
		return nil, BashOutputResult{
			Status: "failed",
			Output: errorMsg,
		}, fmt.Errorf("bash_id is too long (max %d characters), got: %d", MaxBashIDLength, len(args.BashID))
	}

	// 先获取任务信息（短暂持锁），然后释放锁再进行文件I/O
	var taskOutput string
	var taskStatus string
	var taskExitCode *int
	var tempFilePath string

	s.mutex.RLock()
	task, exists := s.backgroundTasks[args.BashID]
	if !exists {
		s.mutex.RUnlock()
		errorMsg := fmt.Sprintf("background task not found: %s", args.BashID)
		return nil, BashOutputResult{
			Status: "not_found",
			Output: errorMsg,
		}, fmt.Errorf("background task not found: %s", args.BashID)
	}

	// 复制必要的信息，避免持锁进行I/O操作
	taskOutput = task.Output
	taskStatus = task.Status
	if task.ExitCode != nil {
		exitCode := *task.ExitCode
		taskExitCode = &exitCode
	}
	tempFilePath = task.TempFile
	s.mutex.RUnlock()

	// 在锁外部读取临时文件（避免持锁I/O导致的性能问题和潜在死锁）
	output := taskOutput
	if tempFilePath != "" {
		if content, err := os.ReadFile(tempFilePath); err == nil {
			output = string(content)
		}
		// 如果文件读取失败，使用内存中的输出
	}

	if args.Filter != "" {
		// 使用正则表达式过滤输出
		regex, err := regexp.Compile(args.Filter)
		if err != nil {
			errorMsg := fmt.Sprintf("invalid regex filter pattern '%s': %v", args.Filter, err)
			return nil, BashOutputResult{
				Status: "failed",
				Output: errorMsg,
			}, fmt.Errorf("invalid filter pattern '%s': %v", args.Filter, err)
		}

		lines := strings.Split(output, "\n")
		var filteredLines []string
		for _, line := range lines {
			if regex.MatchString(line) {
				filteredLines = append(filteredLines, line)
			}
		}
		output = strings.Join(filteredLines, "\n")
	}

	result := BashOutputResult{
		Output:   output,
		Status:   taskStatus,
		ExitCode: taskExitCode,
	}

	// 成功返回 - 使用结构化输出
	return nil, result, nil
}

// KillShellHandler 处理KillShell工具调用 - 使用官方标准Handler签名
func (s *MCPServer) KillShellHandler(ctx context.Context, req *mcp.CallToolRequest, args KillShellArguments) (*mcp.CallToolResult, KillShellResult, error) {
	if args.ShellID == "" {
		errorMsg := "shell_id is required"
		return nil, KillShellResult{
			ShellID: "",
			Message: errorMsg,
		}, fmt.Errorf("%s", errorMsg)
	}

	if len(args.ShellID) > MaxShellIDLength {
		errorMsg := fmt.Sprintf("shell_id is too long (max %d characters), got: %d", MaxShellIDLength, len(args.ShellID))
		return nil, KillShellResult{
			ShellID: args.ShellID,
			Message: errorMsg,
		}, fmt.Errorf("%s", errorMsg)
	}

	s.mutex.Lock()
	task, exists := s.backgroundTasks[args.ShellID]
	if !exists {
		s.mutex.Unlock()
		return nil, KillShellResult{
			ShellID: args.ShellID,
			Message: fmt.Sprintf("background task not found: %s", args.ShellID),
		}, fmt.Errorf("background task not found: %s", args.ShellID)
	}

	// 获取需要的信息，然后释放锁
	process := task.Process
	cancelFunc := task.Cancel
	tempFilePath := task.TempFile
	wasRunning := task.Status == "running"
	job := task.Job

	// 更新任务状态
	if wasRunning {
		task.Status = "killed"
		task.Error = "Task killed by user request"
	}

	// 从后台任务列表中移除
	delete(s.backgroundTasks, args.ShellID)
	s.mutex.Unlock()

	// 在锁外部执行实际的进程终止和资源清理
	// 优先使用 Job Object 终止整个进程树
	if job != nil && runtime.GOOS == "windows" {
		fmt.Fprintf(os.Stderr, "Terminating process tree using Job Object...\n")
		if err := job.Terminate(1); err != nil {
			fmt.Fprintf(os.Stderr, "Note: Job.Terminate failed: %v, trying other methods\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "Successfully terminated process tree using Job Object\n")
			// 关闭 Job Object
			job.Close()
			// 清理临时文件
			if tempFilePath != "" {
				if err := os.Remove(tempFilePath); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to remove temp file %s: %v\n", tempFilePath, err)
				}
			}
			fmt.Fprintf(os.Stderr, "Background task %s killed successfully\n", args.ShellID)
			return nil, KillShellResult{
				Message: fmt.Sprintf("Background task %s killed successfully", args.ShellID),
				ShellID: args.ShellID,
			}, nil
		}
	}

	// 回退方案：先调用Cancel函数取消Context
	if cancelFunc != nil {
		cancelFunc()
	}

	// 强制终止进程树（Windows需要特殊处理）
	if process != nil && runtime.GOOS == "windows" {
		// 在Windows上使用taskkill终止整个进程树
		// 这样可以确保所有子进程（如pnpm启动的node/vite）都被终止
		killCmd := exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprintf("%d", process.Pid))
		if err := killCmd.Run(); err != nil {
			// 如果taskkill失败，尝试使用Go的Kill方法
			fmt.Fprintf(os.Stderr, "Note: taskkill failed: %v, trying process.Kill()\n", err)
			if err := process.Kill(); err != nil {
				// 进程可能已经退出，忽略错误
				fmt.Fprintf(os.Stderr, "Note: process kill returned: %v (may have already exited)\n", err)
			}
		} else {
			fmt.Fprintf(os.Stderr, "Successfully killed process tree with PID %d\n", process.Pid)
		}
	} else if process != nil {
		// 非 Windows 系统，直接使用 Kill
		if err := process.Kill(); err != nil {
			fmt.Fprintf(os.Stderr, "Note: process kill returned: %v (may have already exited)\n", err)
		}
	}

	// 清理临时文件（无论任务状态如何）
	if tempFilePath != "" {
		if err := os.Remove(tempFilePath); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to remove temp file %s: %v\n", tempFilePath, err)
		}
	}

	fmt.Fprintf(os.Stderr, "Background task %s killed successfully\n", args.ShellID)

	// 成功返回 - 使用结构化输出
	return nil, KillShellResult{
		Message: fmt.Sprintf("Background task %s killed successfully", args.ShellID),
		ShellID: args.ShellID,
	}, nil
}

// executeBackgroundCommand 执行后台命令
func (s *MCPServer) executeBackgroundCommand(task *BackgroundTask, timeout int) {
	// 后台任务不应该有超时限制（timeout参数保留用于兼容性，但设为0表示无限制）
	// 用户可以通过 kill_shell 工具手动终止任务

	// 创建可取消的context（不设置超时）
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // 确保在函数退出时释放资源

	// 创建临时文件来存储输出（使用更具描述性的前缀）
	tempFile, err := os.CreateTemp("", "mcp_bash_output_*.txt")
	if err != nil {
		s.mutex.Lock()
		task.Status = "failed"
		task.Error = fmt.Sprintf("Failed to create temp file: %v", err)
		s.mutex.Unlock()
		return
	}
	tempFilePath := tempFile.Name()
	// 立即关闭文件，后续写入时重新打开（避免Windows文件锁问题）
	tempFile.Close()

	// 创建 Job Object（仅 Windows）
	var job *windows.JobObject
	if runtime.GOOS == "windows" {
		jobName := fmt.Sprintf("mcp_bash_job_%s", task.ID)
		job, err = windows.CreateJobObject(jobName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to create Job Object: %v, will use fallback method\n", err)
			job = nil
		} else {
			fmt.Fprintf(os.Stderr, "Created Job Object: %s\n", jobName)
		}
	}

	// 获取Shell执行器的首选Shell路径
	shellPath := "powershell" // 默认值
	if shellExec, ok := s.shellExecutor.(*executor.ShellExecutor); ok {
		if path := shellExec.GetShellPath(shellExec.GetPreferredShell()); path != "" {
			shellPath = path
		}
	}

	// 在goroutine外部创建cmd，以便超时处理时能访问
	// 强制设置控制台输出编码为UTF-8 (CodePage 65001)
	cmdArgs := fmt.Sprintf("[Console]::OutputEncoding=[System.Text.Encoding]::UTF8; %s", task.Command)
	cmd := exec.CommandContext(ctx, shellPath, "-NoProfile", "-Command", cmdArgs)

	// 加锁保护任务字段赋值
	s.mutex.Lock()
	task.TempFile = tempFilePath
	task.Cancel = cancel
	task.Job = job
	s.mutex.Unlock()

	// 使用同步机制保护文件写入
	writeMutex := sync.Mutex{}

	// 启动命令并实时写入临时文件
	done := make(chan struct {
		err      error
		exitCode int
	}, 1)

	// 使用WaitGroup等待所有goroutine完成
	var wg sync.WaitGroup

	go s.executeCommandWithTask(cmd, task, tempFilePath, &writeMutex, &wg, done)

	// 等待命令完成（后台任务无超时限制）
	select {
	case result := <-done:
		cancel() // 命令完成后取消context
		s.handleCommandCompletion(task, result, tempFilePath)
	case <-ctx.Done():
		// Context被取消（通过kill_shell）
		s.handleCommandCancellation(task, cmd, tempFilePath, done, &wg)
	}
}

// executeCommand 执行命令并处理输出
func (s *MCPServer) executeCommandWithTask(cmd *exec.Cmd, task *BackgroundTask, tempFilePath string, writeMutex *sync.Mutex, wg *sync.WaitGroup, done chan<- struct {
	err      error
	exitCode int
}) {
	if cmd == nil {
		done <- struct {
			err      error
			exitCode int
		}{fmt.Errorf("failed to create command"), 1}
		return
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		done <- struct {
			err      error
			exitCode int
		}{fmt.Errorf("failed to create stdout pipe: %w", err), 1}
		return
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		done <- struct {
			err      error
			exitCode int
		}{fmt.Errorf("failed to create stderr pipe: %w", err), 1}
		return
	}

	if err := cmd.Start(); err != nil {
		done <- struct {
			err      error
			exitCode int
		}{fmt.Errorf("failed to start command: %w", err), 1}
		return
	}

	// 保存进程句柄到task，以便外部可以终止进程
	s.mutex.Lock()
	task.Process = cmd.Process

	// 将进程添加到 Job Object（仅 Windows）
	if task.Job != nil && runtime.GOOS == "windows" {
		if err := task.Job.AddProcess(cmd.Process); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to add process to Job Object: %v\n", err)
			// 不是致命错误，继续执行
		} else {
			fmt.Fprintf(os.Stderr, "Added process %d to Job Object\n", cmd.Process.Pid)
		}
	}
	s.mutex.Unlock()

	// 启动输出读取goroutine
	wg.Add(2)
	go s.readOutputPipe(stdout, tempFilePath, writeMutex, wg)
	go s.readErrorPipe(stderr, tempFilePath, writeMutex, wg)

	// 等待命令完成
	cmdErr := cmd.Wait()
	finalExitCode := -1
	if cmd.ProcessState != nil {
		finalExitCode = cmd.ProcessState.ExitCode()
	}

	wg.Wait()
	done <- struct {
		err      error
		exitCode int
	}{cmdErr, finalExitCode}
}

// readOutputPipe 读取stdout并写入临时文件
func (s *MCPServer) readOutputPipe(stdout io.ReadCloser, tempFilePath string, writeMutex *sync.Mutex, wg *sync.WaitGroup) {
	defer wg.Done()
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		writeMutex.Lock()
		// 每次写入都重新打开文件，以避免长时间持有文件锁
		f, err := os.OpenFile(tempFilePath, os.O_APPEND|os.O_WRONLY, 0644)
		if err == nil {
			if _, err := f.WriteString(line + "\n"); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to write to temp file: %v\n", err)
			}
			f.Close()
		} else {
			fmt.Fprintf(os.Stderr, "Failed to open temp file for writing: %v\n", err)
		}
		writeMutex.Unlock()
	}
}

// readErrorPipe 读取stderr并写入临时文件
func (s *MCPServer) readErrorPipe(stderr io.ReadCloser, tempFilePath string, writeMutex *sync.Mutex, wg *sync.WaitGroup) {
	defer wg.Done()
	scanner := bufio.NewScanner(stderr)
	for scanner.Scan() {
		line := scanner.Text()
		writeMutex.Lock()
		// 每次写入都重新打开文件，以避免长时间持有文件锁
		f, err := os.OpenFile(tempFilePath, os.O_APPEND|os.O_WRONLY, 0644)
		if err == nil {
			if _, err := f.WriteString("ERROR: " + line + "\n"); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to write to temp file: %v\n", err)
			}
			f.Close()
		} else {
			fmt.Fprintf(os.Stderr, "Failed to open temp file for writing: %v\n", err)
		}
		writeMutex.Unlock()
	}
}

// handleCommandCompletion 处理命令正常完成
func (s *MCPServer) handleCommandCompletion(task *BackgroundTask, result struct {
	err      error
	exitCode int
}, tempFilePath string) {
	execErr := result.err
	actualExitCode := result.exitCode

	// 读取完整的输出内容
	// 使用 OpenFile 和 ReadAll 确保在读取时不被写入锁阻塞（最好加个重试机制，但目前先简单处理）
	outputContent, readErr := os.ReadFile(tempFilePath)
	if readErr != nil {
		s.mutex.Lock()
		task.Status = "failed"
		task.Error = fmt.Sprintf("Failed to read output file: %v", readErr)
		exitCode := -1
		task.ExitCode = &exitCode
		task.TempFile = "" // 清除临时文件路径
		s.mutex.Unlock()
		// 尝试删除临时文件
		if tempFilePath != "" {
			os.Remove(tempFilePath)
		}
		return
	}

	s.mutex.Lock()
	task.Output = string(outputContent)
	if execErr != nil {
		task.Status = "failed"
		task.Error = execErr.Error()
	} else {
		task.Status = "completed"
	}
	task.ExitCode = &actualExitCode
	task.TempFile = "" // 清除临时文件路径，表示内容已加载到内存
	s.mutex.Unlock()

	// 删除临时文件（内容已保存到task.Output）
	if tempFilePath != "" {
		if err := os.Remove(tempFilePath); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to remove temp file %s: %v\n", tempFilePath, err)
		}
	}
}

// handleCommandCancellation 处理命令被取消（通过kill_shell）
func (s *MCPServer) handleCommandCancellation(task *BackgroundTask, cmd *exec.Cmd, tempFilePath string, done chan struct {
	err      error
	exitCode int
}, wg *sync.WaitGroup) {
	// 被取消，强制终止进程树（Windows需要特殊处理）
	if cmd.Process != nil {
		// 优先使用 Job Object
		s.mutex.RLock()
		job := task.Job
		s.mutex.RUnlock()

		if job != nil && runtime.GOOS == "windows" {
			fmt.Fprintf(os.Stderr, "Terminating process tree using Job Object in cancellation...\n")
			if err := job.Terminate(1); err != nil {
				fmt.Fprintf(os.Stderr, "Note: Job.Terminate failed in cancellation: %v\n", err)
			}
			job.Close()
		} else if runtime.GOOS == "windows" {
			// 回退到 taskkill
			killCmd := exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprintf("%d", cmd.Process.Pid))
			if err := killCmd.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "Note: taskkill failed in cancellation: %v, trying process.Kill()\n", err)
				cmd.Process.Kill()
			}
		} else {
			cmd.Process.Kill()
		}
	}
	// 等待输出 goroutine 完成后再关闭文件
	wg.Wait()
	// 接收 done 结果，避免 executeCommand 的发送长期占用（带短超时防止永久阻塞）
	select {
	case <-done:
	case <-time.After(DoneChannelTimeout):
	}

	// 读取已有的输出
	var outputStr string
	if tempFilePath != "" {
		outputContent, _ := os.ReadFile(tempFilePath)
		if len(outputContent) > 0 {
			outputStr = string(outputContent)
		}
	}

	s.mutex.Lock()
	task.Status = "killed"
	task.Error = "Task was cancelled by user"
	exitCode := -1
	task.ExitCode = &exitCode
	task.Output = outputStr
	task.TempFile = "" // 清除临时文件路径，表示内容已加载到内存
	s.mutex.Unlock()

	// 删除临时文件（内容已保存到task.Output）
	if tempFilePath != "" {
		if err := os.Remove(tempFilePath); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to remove temp file %s: %v\n", tempFilePath, err)
		}
	}
}

// AddBashTools 注册所有bash工具 - 使用官方标准注册模式
func AddBashTools(server *mcp.Server) {
	bashServer := NewMCPServer()

	// 注册Bash工具 - 使用官方推荐的AddTool模式
	mcp.AddTool(server, &mcp.Tool{
		Name:        "bash",
		Description: "安全执行PowerShell命令，支持前台和后台执行模式\n\n主要功能：\n• 仅支持PowerShell 7+和Windows PowerShell 5.x命令执行\n• 智能Shell环境检测，自动选择最佳Shell\n• 支持前台执行（同步等待结果）和后台执行（异步任务）\n• 必填超时时间（1-600秒）防止无限等待\n• 企业级安全验证（危险命令过滤、长度限制）\n• 完整错误处理和退出代码返回\n\n参数说明：\n• command（必填）：要执行的PowerShell命令\n• timeout（必填）：超时时间（毫秒），范围1000-600000\n• description（可选）：命令描述，用于日志记录\n• run_in_background（可选）：是否后台执行，默认false\n\n返回结果：\n• output：命令执行输出内容\n• exitCode：命令退出代码\n• killed：是否被强制终止\n• shellId：后台任务ID（仅后台执行时返回）\n\n安全限制：\n• 最大命令长度10000字符\n• 禁止危险命令（删除、格式化、关机等）\n• 自动检测和过滤恶意操作\n• timeout参数为必填项，确保命令执行时间可控",
	}, bashServer.BashHandler)

	// 注册BashOutput工具
	mcp.AddTool(server, &mcp.Tool{
		Name:        "bash_output",
		Description: "获取后台任务的实时输出内容，支持正则表达式过滤\n\n主要功能：\n• 实时读取后台命令执行输出\n• 从临时文件实时获取最新内容\n• 支持正则表达式过滤输出行\n• 精确的任务状态追踪\n• 自动清理完成的任务\n\n参数说明：\n• bash_id（必填）：后台任务的Bash ID（由bash工具返回）\n• filter（可选）：正则表达式过滤器，用于筛选输出内容\n\n返回结果：\n• output：后台任务的输出内容（过滤后）\n• status：任务状态（running, completed, failed, killed, not_found）\n• exitCode：任务退出代码（仅任务完成时返回）\n\n使用说明：\n• 与bash工具的run_in_background参数配合使用\n• 适用于长时间运行的任务（编译、部署、下载等）\n• 可通过正则表达式精确筛选日志内容\n• 建议定期轮询获取最新输出\n• 任务完成后自动更新状态",
	}, bashServer.BashOutputHandler)

	// 注册KillShell工具
	mcp.AddTool(server, &mcp.Tool{
		Name:        "kill_shell",
		Description: "终止正在运行的后台任务，释放系统资源\n\n主要功能：\n• 强制终止指定的后台命令\n• 自动清理任务相关资源\n• 更新任务状态为killed\n• 防止资源泄漏和僵尸进程\n\n参数说明：\n• shell_id（必填）：要终止的后台任务Shell ID\n\n返回结果：\n• message：操作结果消息\n• shell_id：被终止的任务Shell ID\n\n使用场景：\n• 长时间运行的任务需要手动中断\n• 发现任务异常或卡死时强制终止\n• 系统维护和资源清理\n• 测试和开发环境中的任务管理\n\n注意事项：\n• 仅能终止通过bash工具创建的后台任务\n• 被终止的任务无法恢复\n• 建议确认任务确实需要终止后再调用\n• 终止操作会立即生效",
	}, bashServer.KillShellHandler)
}

func main() {
	// 创建MCP服务器实例 - 使用官方标准配置
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "mcp-bash-tools",
		Version: "1.0.0",
	}, &mcp.ServerOptions{
		Instructions: `MCP Bash Tools Server - Windows专用安全命令执行服务器

功能特性：
- 企业级安全验证 - 多层安全检查防止恶意命令执行
- 支持前台/后台执行模式 - 灵活的任务管理
- 实时输出监控 - 后台任务输出实时获取
- 正则过滤功能 - 精确筛选输出内容
- 资源限制保护 - 防止系统资源滥用

可用工具：
- bash - 执行PowerShell命令
- bash_output - 获取后台任务输出
- kill_shell - 终止后台任务

安全限制：
- 禁止危险命令（rm -rf, format, shutdown等）
- 命令长度限制（最大10000字符）
- 超时保护（默认30秒，最大600秒）`,
	})

	// 打印启动信息
	fmt.Fprintf(os.Stderr, "MCP Bash Tools Server starting...\n")
	fmt.Fprintf(os.Stderr, "Server Information:\n")
	fmt.Fprintf(os.Stderr, "   Name: %s\n", "mcp-bash-tools")
	fmt.Fprintf(os.Stderr, "   Version: %s\n", "1.0.0")
	fmt.Fprintln(os.Stderr)

	// 创建并初始化Shell执行器
	bashServer := NewMCPServer()
	fmt.Fprintf(os.Stderr, "Shell Environment Information:\n")
	bashServer.shellExecutor.PrintShellInfo()
	fmt.Fprintln(os.Stderr)

	// 注册所有bash工具
	fmt.Fprintf(os.Stderr, "Registering MCP tools...\n")
	AddBashTools(server)
	fmt.Fprintf(os.Stderr, "Tools registered successfully:\n")
	fmt.Fprintf(os.Stderr, "   - bash - Execute PowerShell commands\n")
	fmt.Fprintf(os.Stderr, "   - bash_output - Get background task output\n")
	fmt.Fprintf(os.Stderr, "   - kill_shell - Terminate background tasks\n")
	fmt.Fprintln(os.Stderr)

	// 启动服务器 - 使用官方标准启动方式
	fmt.Fprintf(os.Stderr, "Starting MCP server with stdio transport...\n")
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		fmt.Fprintf(os.Stderr, "Server failed to start: %v\n", err)
		os.Exit(1)
	}
}
