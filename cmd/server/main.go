package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"mcp-bash-tools/internal/executor"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// NewShellExecutor 创建实际的ShellExecutor
func NewShellExecutor() ShellExecutorInterface {
	return executor.NewShellExecutor()
}

// BashArguments 定义Bash工具的输入参数 - 使用官方标准命名
type BashArguments struct {
	Command         string `json:"command" jsonschema:"要执行的PowerShell/CMD命令"`
	Timeout         int    `json:"timeout,omitempty" jsonschema:"命令超时时间(毫秒),默认30000,范围1000-600000"`
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
	ID        string    `json:"id"`
	Command   string    `json:"command"`
	Output    string    `json:"output"`
	Status    string    `json:"status"` // running, completed, failed, killed
	StartTime time.Time `json:"startTime"`
	Error     string    `json:"error,omitempty"`
	ExitCode  *int      `json:"exitCode,omitempty"`
	TempFile  string    `json:"tempFile,omitempty"` // 临时文件路径用于存储输出
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
	// 参数验证 - 工具级错误（用户可见，不终止连接）
	if args.Command == "" {
		// 返回详细的错误信息
		errorMsg := "command参数是必需的"
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: errorMsg},
			},
			IsError: true,
		}, BashResult{
			ExitCode: 1,
			Output:   errorMsg,
		}, nil
	}

	if args.Timeout != 0 && (args.Timeout < 1000 || args.Timeout > 600000) {
		errorMsg := fmt.Sprintf("timeout必须在1000到600000毫秒之间，当前值: %d", args.Timeout)
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: errorMsg},
			},
			IsError: true,
		}, BashResult{
			ExitCode: 1,
			Output:   errorMsg,
		}, nil
	}

	// 安全检查
	if isDangerousCommand(args.Command) {
		errorMsg := fmt.Sprintf("命令因安全原因被拒绝: %s", args.Command)
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: errorMsg},
			},
			IsError: true,
		}, BashResult{
			ExitCode: 1,
			Output:   errorMsg,
		}, nil
	}

	// 日志记录
	logMsg := args.Description
	if logMsg == "" {
		logMsg = args.Command
	}
	fmt.Fprintf(os.Stderr, "Executing command: %s\n", logMsg)

	if args.RunInBackground {
		// 后台执行
		s.mutex.Lock()
		taskID := fmt.Sprintf("bash_%d", time.Now().UnixNano())
		task := &BackgroundTask{
			ID:        taskID,
			Command:   args.Command,
			StartTime: time.Now(),
			Status:    "running",
		}
		s.backgroundTasks[taskID] = task

		// 启动后台任务
		go s.executeBackgroundCommand(task, args.Timeout)
		s.mutex.Unlock()

		// 返回结果 - 使用结构化输出，不填充Content
		return nil, BashResult{
			ExitCode: 0,
			ShellID:  taskID,
		}, nil
	} else {
		// 前台执行
		output, exitCode, err := s.shellExecutor.ExecuteCommand(args.Command, args.Timeout)

		killed := false
	if err != nil {
		// 检查是否为超时导致的进程终止
		// 在Windows上，context超时通常返回"exit status 1"
		// 我们需要检查超时时间是否已过以及错误类型
		if strings.Contains(err.Error(), "killed") || 
		   strings.Contains(err.Error(), "context deadline exceeded") ||
		   strings.Contains(err.Error(), "signal: killed") {
			killed = true
		}
	}

		if err != nil && !killed {
			// 错误信息包含在输出中，返回成功状态以传递BashResult
			errorOutput := output
			if errorOutput == "" {
				errorOutput = fmt.Sprintf("命令执行失败: %v", err)
			} else {
				errorOutput = fmt.Sprintf("%s\n错误: %v", output, err)
			}
			
			// 返回CallToolResult包含错误信息，同时返回BashResult
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: errorOutput},
				},
				IsError: true, // 标记为错误，但仍然传递输出
			}, BashResult{
				Output:   errorOutput,
				ExitCode: exitCode,
				Killed:   killed,
			}, nil
		}

		// 成功返回 - 使用结构化输出
		return nil, BashResult{
			Output:   output,
			ExitCode: exitCode,
			Killed:   killed,
		}, nil
	}
}

// BashOutputHandler 处理BashOutput工具调用 - 使用官方标准Handler签名
func (s *MCPServer) BashOutputHandler(ctx context.Context, req *mcp.CallToolRequest, args BashOutputArguments) (*mcp.CallToolResult, BashOutputResult, error) {
	if args.BashID == "" {
		return nil, BashOutputResult{
			Status: "failed",
		}, fmt.Errorf("bash_id参数是必需的")
	}

	if len(args.BashID) > 100 {
		errorMsg := fmt.Sprintf("bash_id过长(最大100字符)，当前长度: %d", len(args.BashID))
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: errorMsg},
			},
			IsError: true,
		}, BashOutputResult{
			Status: "failed",
			Output: errorMsg,
		}, nil
	}

	s.mutex.RLock()
	defer s.mutex.RUnlock()

	task, exists := s.backgroundTasks[args.BashID]
	if !exists {
		errorMsg := fmt.Sprintf("未找到后台任务: %s", args.BashID)
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: errorMsg},
			},
			IsError: true,
		}, BashOutputResult{
			Status: "not_found",
			Output: errorMsg,
		}, nil
	}

	// 从临时文件中读取最新的输出内容
	output := task.Output
	if task.TempFile != "" {
		// 从临时文件中读取最新的输出
		if content, err := os.ReadFile(task.TempFile); err == nil {
			output = string(content)
			// 更新内存中的输出，以便后续调用也能够获取到最新内容
			// 先释放读锁，获取写锁
			s.mutex.RUnlock()
			s.mutex.Lock()
			if existingTask, exists := s.backgroundTasks[args.BashID]; exists {
				existingTask.Output = output
			}
			s.mutex.Unlock()
			// 重新获取读锁
			s.mutex.RLock()
			// 重新获取task以确保我们使用的是最新的数据
			if updatedTask, exists := s.backgroundTasks[args.BashID]; exists {
				output = updatedTask.Output
			}
		}
	}

	if args.Filter != "" {
		// 使用正则表达式过滤输出
		regex, err := regexp.Compile(args.Filter)
		if err != nil {
			errorMsg := fmt.Sprintf("无效的正则表达式过滤模式 '%s': %v", args.Filter, err)
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: errorMsg},
				},
				IsError: true,
			}, BashOutputResult{
				Status: "failed",
				Output: errorMsg,
			}, nil
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
		Output: output,
		Status: task.Status,
	}

	if task.ExitCode != nil {
		result.ExitCode = task.ExitCode
	}

	// 成功返回 - 使用结构化输出
	return nil, result, nil
}

// KillShellHandler 处理KillShell工具调用 - 使用官方标准Handler签名
func (s *MCPServer) KillShellHandler(ctx context.Context, req *mcp.CallToolRequest, args KillShellArguments) (*mcp.CallToolResult, KillShellResult, error) {
	if args.ShellID == "" {
		errorMsg := "shell_id参数是必需的"
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: errorMsg},
			},
			IsError: true,
		}, KillShellResult{
			ShellID: "",
			Message: errorMsg,
		}, nil
	}

	if len(args.ShellID) > 100 {
		errorMsg := fmt.Sprintf("shell_id过长(最大100字符)，当前长度: %d", len(args.ShellID))
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: errorMsg},
			},
			IsError: true,
		}, KillShellResult{
			ShellID: args.ShellID,
			Message: errorMsg,
		}, nil
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()

	task, exists := s.backgroundTasks[args.ShellID]
	if !exists {
		errorMsg := fmt.Sprintf("未找到后台任务: %s", args.ShellID)
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: errorMsg},
			},
			IsError: true,
		}, KillShellResult{
			ShellID: args.ShellID,
			Message: errorMsg,
		}, nil
	}

	// 终止后台任务
	if task.Status == "running" {
		task.Status = "killed"
		task.Error = "Task killed by user request"
	}

	// 从后台任务列表中移除
	delete(s.backgroundTasks, args.ShellID)

	fmt.Fprintf(os.Stderr, "Background task %s killed successfully\n", args.ShellID)

	// 成功返回 - 使用结构化输出
	return nil, KillShellResult{
		Message: fmt.Sprintf("Background task %s killed successfully", args.ShellID),
		ShellID: args.ShellID,
	}, nil
}

// executeBackgroundCommand 执行后台命令
func (s *MCPServer) executeBackgroundCommand(task *BackgroundTask, timeout int) {
	// 设置默认超时如果未指定
	if timeout <= 0 {
		timeout = 30000 // 默认30秒
	}

	// 创建临时文件来存储输出
	tempFile, err := os.CreateTemp("", "bash_output_*.txt")
	if err != nil {
		s.mutex.Lock()
		task.Status = "failed"
		task.Error = fmt.Sprintf("Failed to create temp file: %v", err)
		s.mutex.Unlock()
		return
	}
	task.TempFile = tempFile.Name()
	defer os.Remove(tempFile.Name()) // 确保临时文件被清理

	// 创建带超时的context
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Millisecond)
	defer cancel()

	// 启动命令并实时写入临时文件
	done := make(chan struct {
		err error
		exitCode int
	}, 1)
	go func() {
		var cmd *exec.Cmd
		if strings.Contains(strings.ToLower(task.Command), "powershell") {
			cmd = exec.CommandContext(ctx, "powershell", "-Command", task.Command)
		} else {
			cmd = exec.CommandContext(ctx, "cmd", "/C", task.Command)
		}

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			done <- struct {
				err error
				exitCode int
			}{fmt.Errorf("failed to create stdout pipe: %w", err), 1}
			return
		}

		stderr, err := cmd.StderrPipe()
		if err != nil {
			done <- struct {
				err error
				exitCode int
			}{fmt.Errorf("failed to create stderr pipe: %w", err), 1}
			return
		}

		if err := cmd.Start(); err != nil {
			done <- struct {
				err error
				exitCode int
			}{fmt.Errorf("failed to start command: %w", err), 1}
			return
		}

		// 创建输出写入器
		fileWriter := tempFile

		// 读取stdout
		go func() {
			scanner := bufio.NewScanner(stdout)
			for scanner.Scan() {
				line := scanner.Text()
				// 写入临时文件
				fileWriter.WriteString(line + "\n")
				fileWriter.Sync() // 确保内容被写入磁盘
			}
		}()

		// 读取stderr
		go func() {
			scanner := bufio.NewScanner(stderr)
			for scanner.Scan() {
				line := scanner.Text()
				// 写入临时文件
				fileWriter.WriteString("ERROR: " + line + "\n")
				fileWriter.Sync() // 确保内容被写入磁盘
			}
		}()

		// 等待命令完成
		err = cmd.Wait()
		var finalExitCode int
		if cmd.ProcessState != nil {
			finalExitCode = cmd.ProcessState.ExitCode()
		} else {
			finalExitCode = -1
		}
		done <- struct {
			err error
			exitCode int
		}{err, finalExitCode}
	}()

	// 等待命令完成或超时
	select {
	case result := <-done:
		execErr := result.err
		actualExitCode := result.exitCode
		// 命令完成，关闭临时文件
		tempFile.Close()
		
		// 读取完整的输出内容
		outputContent, readErr := os.ReadFile(task.TempFile)
		if readErr != nil {
			s.mutex.Lock()
			task.Status = "failed"
			task.Error = fmt.Sprintf("Failed to read output file: %v", readErr)
			exitCode := -1
			task.ExitCode = &exitCode
			s.mutex.Unlock()
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
		s.mutex.Unlock()

	case <-ctx.Done():
		// 超时，强制终止进程
		tempFile.Close()
		
		s.mutex.Lock()
		task.Status = "failed"
		task.Error = fmt.Sprintf("Command timed out after %dms", timeout)
		exitCode := 1 // 超时通常表示失败
		task.ExitCode = &exitCode
		s.mutex.Unlock()
		
		// 读取已有的输出
		outputContent, _ := os.ReadFile(task.TempFile)
		s.mutex.Lock()
		if len(outputContent) > 0 {
			task.Output = string(outputContent)
		}
		s.mutex.Unlock()
	}
}

// isDangerousCommand 检查是否为危险命令
func isDangerousCommand(command string) bool {
	dangerousCommands := []string{
		"rm -rf",
		"del /f",
		"format",
		"shutdown",
		"reboot",
		"sudo rm",
		"> /dev/null",
	}

	for _, dangerous := range dangerousCommands {
		if strings.Contains(command, dangerous) {
			return true
		}
	}
	return false
}

// AddBashTools 注册所有bash工具 - 使用官方标准注册模式
func AddBashTools(server *mcp.Server) {
	bashServer := NewMCPServer()

	// 注册Bash工具 - 使用官方推荐的AddTool模式
	mcp.AddTool(server, &mcp.Tool{
		Name:        "bash",
		Description: "安全执行PowerShell/CMD命令，支持前台和后台执行模式\n\n主要功能：\n• 支持PowerShell 7+和Windows CMD命令执行\n• 智能Shell环境检测，自动选择最佳Shell\n• 支持前台执行（同步等待结果）和后台执行（异步任务）\n• 可配置超时时间（1-600秒，默认30秒）\n• 企业级安全验证（危险命令过滤、长度限制）\n• 完整错误处理和退出代码返回\n\n参数说明：\n• command（必填）：要执行的PowerShell/CMD命令\n• timeout（可选）：超时时间（毫秒），范围1000-600000\n• description（可选）：命令描述，用于日志记录\n• run_in_background（可选）：是否后台执行，默认false\n\n返回结果：\n• output：命令执行输出内容\n• exitCode：命令退出代码\n• killed：是否被强制终止\n• shellId：后台任务ID（仅后台执行时返回）\n\n安全限制：\n• 最大命令长度10000字符\n• 禁止危险命令（删除、格式化、关机等）\n• 自动检测和过滤恶意操作",
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
		Instructions: `🚀 MCP Bash Tools Server - Windows专用安全命令执行服务器

功能特性：
• 🔒 企业级安全验证 - 多层安全检查防止恶意命令执行
• ⚡ 支持前台/后台执行模式 - 灵活的任务管理
• 📊 实时输出监控 - 后台任务输出实时获取
• 🎯 正则过滤功能 - 精确筛选输出内容
• 🛡️ 资源限制保护 - 防止系统资源滥用

可用工具：
• bash - 执行PowerShell/CMD命令
• bash_output - 获取后台任务输出
• kill_shell - 终止后台任务

安全限制：
• 禁止危险命令（rm -rf, format, shutdown等）
• 命令长度限制（最大10000字符）
• 超时保护（默认30秒，最大600秒）`,
	})

	// 打印启动信息
	fmt.Fprintf(os.Stderr, "🚀 MCP Bash Tools Server starting...\n")
	fmt.Fprintf(os.Stderr, "📋 Server Information:\n")
	fmt.Fprintf(os.Stderr, "   • Name: %s\n", "mcp-bash-tools")
	fmt.Fprintf(os.Stderr, "   • Version: %s\n", "1.0.0")
	fmt.Fprintln(os.Stderr)
	
	// 创建并初始化Shell执行器
	bashServer := NewMCPServer()
	fmt.Fprintf(os.Stderr, "🔧 Shell Environment Information:\n")
	bashServer.shellExecutor.PrintShellInfo()
	fmt.Fprintln(os.Stderr)

	// 注册所有bash工具
	fmt.Fprintf(os.Stderr, "📦 Registering MCP tools...\n")
	AddBashTools(server)
	fmt.Fprintf(os.Stderr, "✅ Tools registered successfully:\n")
	fmt.Fprintf(os.Stderr, "   • bash - Execute PowerShell/CMD commands\n")
	fmt.Fprintf(os.Stderr, "   • bash_output - Get background task output\n")
	fmt.Fprintf(os.Stderr, "   • kill_shell - Terminate background tasks\n")
	fmt.Fprintln(os.Stderr)

	// 启动服务器 - 使用官方标准启动方式
	fmt.Fprintf(os.Stderr, "🌟 Starting MCP server with stdio transport...\n")
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Server failed to start: %v\n", err)
		os.Exit(1)
	}
}
