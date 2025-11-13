package main

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"mcp-bash-tools/internal/executor"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// BashInput 定义Bash工具的输入参数
type BashInput struct {
	Command         string `json:"command"`
	Timeout         int    `json:"timeout,omitempty"`
	Description     string `json:"description,omitempty"`
	RunInBackground bool   `json:"run_in_background,omitempty"`
}

// BashOutput 定义Bash工具的输出
type BashOutput struct {
	Output   string `json:"output"`
	ExitCode int    `json:"exitCode"`
	Killed   bool   `json:"killed,omitempty"`
	ShellID  string `json:"shellId,omitempty"`
}

// BashOutputInput 定义BashOutput工具的输入参数
type BashOutputInput struct {
	BashID string `json:"bash_id"`
	Filter string `json:"filter,omitempty"`
}

// BashOutputToolOutput 定义BashOutput工具的输出
type BashOutputToolOutput struct {
	Output   string `json:"output"`
	Status   string `json:"status"`
	ExitCode *int   `json:"exitCode,omitempty"`
}

// KillShellInput 定义KillShell工具的输入参数
type KillShellInput struct {
	ShellID string `json:"shell_id"`
}

// KillBashOutput 定义KillShell工具的输出
type KillBashOutput struct {
	Message string `json:"message"`
	ShellID string `json:"shell_id"`
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
}

// MCPServer MCP服务器结构
type MCPServer struct {
	backgroundTasks map[string]*BackgroundTask
	mutex           sync.RWMutex
	shellExecutor   *executor.ShellExecutor
}

// NewMCPServer 创建新的MCP服务器
func NewMCPServer() *MCPServer {
	return &MCPServer{
		backgroundTasks: make(map[string]*BackgroundTask),
		shellExecutor:   executor.NewShellExecutor(),
	}
}

// BashHandler 处理Bash命令执行
func (s *MCPServer) BashHandler(ctx context.Context, req *mcp.CallToolRequest, input BashInput) (*mcp.CallToolResult, BashOutput, error) {
	// 参数验证
	if input.Command == "" {
		return nil, BashOutput{}, fmt.Errorf("command is required")
	}

	if input.Timeout != 0 && (input.Timeout < 1000 || input.Timeout > 600000) {
		return nil, BashOutput{}, fmt.Errorf("timeout must be between 1000 and 600000 milliseconds")
	}

	// 安全检查
	if isDangerousCommand(input.Command) {
		return nil, BashOutput{}, fmt.Errorf("command rejected for security reasons")
	}

	// 日志记录
	logMsg := input.Description
	if logMsg == "" {
		logMsg = input.Command
	}
	fmt.Printf("Executing command: %s\n", logMsg)

	if input.RunInBackground {
		// 后台执行
		s.mutex.Lock()
		taskID := fmt.Sprintf("bash_%d", time.Now().UnixNano())
		task := &BackgroundTask{
			ID:        taskID,
			Command:   input.Command,
			StartTime: time.Now(),
			Status:    "running",
		}
		s.backgroundTasks[taskID] = task

		// 启动后台任务
		go s.executeBackgroundCommand(task, input.Timeout)
		s.mutex.Unlock()

		output := BashOutput{
			Output:   fmt.Sprintf("Command started in background with ID: %s", taskID),
			ExitCode: 0,
			Killed:   false,
			ShellID:  taskID,
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "Command started in background"},
			},
		}, output, nil
	} else {
		// 前台执行
		output, exitCode, err := s.shellExecutor.ExecuteCommand(input.Command, input.Timeout)

		killed := false
		if err != nil && strings.Contains(err.Error(), "killed") {
			killed = true
		}

		if err != nil && !killed {
			return nil, BashOutput{}, err
		}

		result := BashOutput{
			Output:   output,
			ExitCode: exitCode,
			Killed:   killed,
			ShellID:  "", // 前台执行的shellId为空
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: output},
			},
		}, result, nil
	}
}

// BashOutputHandler 处理BashOutput工具调用
func (s *MCPServer) BashOutputHandler(ctx context.Context, req *mcp.CallToolRequest, input BashOutputInput) (*mcp.CallToolResult, BashOutputToolOutput, error) {
	if input.BashID == "" {
		return nil, BashOutputToolOutput{}, fmt.Errorf("bash_id is required")
	}

	if len(input.BashID) > 100 {
		return nil, BashOutputToolOutput{}, fmt.Errorf("bash_id too long (max 100 characters)")
	}

	s.mutex.RLock()
	defer s.mutex.RUnlock()

	task, exists := s.backgroundTasks[input.BashID]
	if !exists {
		return nil, BashOutputToolOutput{}, fmt.Errorf("background task not found: %s", input.BashID)
	}

	output := task.Output
	if input.Filter != "" {
		// 使用正则表达式过滤输出
		regex, err := regexp.Compile(input.Filter)
		if err != nil {
			return nil, BashOutputToolOutput{}, fmt.Errorf("invalid filter pattern: %v", err)
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

	result := BashOutputToolOutput{
		Output: output,
		Status: task.Status,
	}

	if task.ExitCode != nil {
		result.ExitCode = task.ExitCode
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: output},
		},
	}, result, nil
}

// KillShellHandler 处理KillShell工具调用
func (s *MCPServer) KillShellHandler(ctx context.Context, req *mcp.CallToolRequest, input KillShellInput) (*mcp.CallToolResult, KillBashOutput, error) {
	if input.ShellID == "" {
		return nil, KillBashOutput{}, fmt.Errorf("shell_id is required")
	}

	if len(input.ShellID) > 100 {
		return nil, KillBashOutput{}, fmt.Errorf("shell_id too long (max 100 characters)")
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()

	task, exists := s.backgroundTasks[input.ShellID]
	if !exists {
		return nil, KillBashOutput{}, fmt.Errorf("background task not found: %s", input.ShellID)
	}

	// 终止后台任务
	if task.Status == "running" {
		task.Status = "killed"
		task.Error = "Task killed by user request"
	}

	// 从后台任务列表中移除
	delete(s.backgroundTasks, input.ShellID)

	fmt.Printf("Background task %s killed successfully\n", input.ShellID)

	result := KillBashOutput{
		Message: fmt.Sprintf("Background task %s killed successfully", input.ShellID),
		ShellID: input.ShellID,
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: result.Message},
		},
	}, result, nil
}

// executeBackgroundCommand 执行后台命令
func (s *MCPServer) executeBackgroundCommand(task *BackgroundTask, timeout int) {
	// 设置默认超时如果未指定
	if timeout <= 0 {
		timeout = 30000 // 默认30秒
	}
	
	// 创建带超时的context
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Millisecond)
	defer cancel()
	
	// 使用BashExecutor而不是ShellExecutor，因为它有更好的超时处理
	bashExecutor := executor.NewBashExecutor()
	output, exitCode, err := bashExecutor.Execute(task.Command, timeout)

	s.mutex.Lock()
	defer s.mutex.Unlock()

	// 检查是否因为超时而失败
	if err != nil && ctx.Err() == context.DeadlineExceeded {
		task.Status = "failed"
		task.Error = fmt.Sprintf("Command timed out after %dms", timeout)
		task.Output = output // 即使超时也返回部分输出
		task.ExitCode = &exitCode
	} else if err != nil {
		task.Status = "failed"
		task.Error = err.Error()
		task.Output = output
		task.ExitCode = &exitCode
	} else {
		task.Status = "completed"
		task.Output = output
		task.ExitCode = &exitCode
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

func main() {
	// 创建MCP服务器
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "mcp-bash-tools",
		Version: "1.0.0",
	}, nil)

	// 创建我们的服务器实例
	bashServer := NewMCPServer()

	// 打印Shell信息
	fmt.Println("🚀 MCP Bash Tools Server starting...")
	bashServer.shellExecutor.PrintShellInfo()
	fmt.Println()

	// 添加Bash工具
	mcp.AddTool(server, &mcp.Tool{
		Name:        "Bash",
		Description: "Executes a given Pwsh7(Powershell) command in a persistent PowerShell session with optional timeout, ensuring proper handling and security safeguards",
	}, bashServer.BashHandler)

	// 添加BashOutput工具
	mcp.AddTool(server, &mcp.Tool{
		Name:        "BashOutput",
		Description: "Executes a given Pwsh7(Powershell) command.Retrieves output from a running or completed background bash shell",
	}, bashServer.BashOutputHandler)

	// 添加KillShell工具
	mcp.AddTool(server, &mcp.Tool{
		Name:        "KillShell",
		Description: "Executes a given Pwsh7(Powershell) command.Kill a running background bash shell",
	}, bashServer.KillShellHandler)

	// 启动服务器并运行在stdio上
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		fmt.Printf("Server failed: %v\n", err)
	}
}
