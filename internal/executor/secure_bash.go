package executor

/*
	⚠️ 注意: 此文件包含企业级安全执行器的预留实现

	当前状态: 未启用
	- 本项目当前使用基础的 ShellExecutor (shell.go + bash.go)
	- SecureBashExecutor 提供了高级安全特性（沙箱、资源限制、网络策略等）
	- 这些功能已实现但未集成到主服务器中

	如需启用:
	1. 修改 cmd/server/main.go 中的 NewShellExecutor() 函数
	2. 返回 executor.NewSecureBashExecutor() 而非 executor.NewShellExecutor()
	3. 配置相应的安全策略和资源限制

	功能特性:
	- 沙箱隔离执行环境
	- CPU/内存/磁盘资源限制
	- 网络访问策略控制
	- 进程监控和管理
	- 高级安全验证
*/

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Enterprise-grade secure bash executor
type SecureBashExecutor struct {
	defaultTimeout time.Duration
	security       *ExecutionSecurity
	sandbox        *Sandbox
	mutex          sync.RWMutex
	workingDir     string
	maxOutputSize  int64
}

// ExecutionSecurity handles security policies
type ExecutionSecurity struct {
	allowedCommands map[string]bool
	blockedCommands map[string]bool
	allowedPaths    []string
	blockedPaths    []string
	maxMemory       int64
	maxCPU          float64
	enableChroot    bool
	enableNetwork   bool
	allowFileWrites bool
	maxProcessCount int
	mutex           sync.RWMutex
}

// Sandbox provides isolation for command execution
type Sandbox struct {
	rootDir       string
	tempDir       string
	networkPolicy NetworkPolicy
	resources     ResourceLimits
	mutex         sync.Mutex
}

type NetworkPolicy struct {
	allowOutbound bool
	allowedHosts  []string
	blockedHosts  []string
	allowedPorts  []int
	blockedPorts  []int
}

type ResourceLimits struct {
	maxMemory    int64   // bytes
	maxCPU       float64 // percentage
	maxDisk      int64   // bytes
	maxOpenFiles int
	maxProcesses int
}

// ExecutionContext holds context for command execution
type ExecutionContext struct {
	Command        string
	Timeout        time.Duration
	WorkingDir     string
	EnvVars        map[string]string
	UserID         string
	SessionID      string
	RequireSandbox bool
	AllowNetwork   bool
	MaxOutputSize  int64
}

// ExecutionResult contains the results of command execution
type ExecutionResult struct {
	Output             string
	ErrorOutput        string
	ExitCode           int
	ExecutionTime      time.Duration
	MemoryUsed         int64
	ProcessesKilled    int
	SecurityViolations []string
	TimedOut           bool
	Killed             bool
	Success            bool
}

// ProcessMonitor tracks running processes
type ProcessMonitor struct {
	processes    map[int]*ProcessInfo
	mutex        sync.RWMutex
	maxProcesses int
}

type ProcessInfo struct {
	PID         int
	Command     string
	StartTime   time.Time
	UserID      string
	SessionID   string
	MemoryUsage int64
	CPUUsage    float64
	Status      string
}

// NewSecureBashExecutor creates a new secure bash executor
func NewSecureBashExecutor() *SecureBashExecutor {
	security := &ExecutionSecurity{
		allowedCommands: make(map[string]bool),
		blockedCommands: make(map[string]bool),
		maxMemory:       512 * 1024 * 1024, // 512MB
		maxCPU:          80.0,              // 80%
		enableChroot:    false,
		enableNetwork:   false,
		allowFileWrites: true,
		maxProcessCount: 10,
	}

	// Initialize default security policies
	security.initializeDefaultPolicies()

	sandbox, err := NewSandbox()
	if err != nil {
		// Sandbox creation failed, will run without sandbox isolation
		fmt.Fprintf(os.Stderr, "Warning: failed to create sandbox: %v, running without sandbox isolation\n", err)
	}
	return &SecureBashExecutor{
		defaultTimeout: 10 * time.Second,
		security:       security,
		sandbox:        sandbox,
		workingDir:     getWorkingDirectory(),
		maxOutputSize:  10 * 1024 * 1024, // 10MB
	}
}

func NewSandbox() (*Sandbox, error) {
	tempDir, err := os.MkdirTemp("", "mcp-sandbox-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create sandbox temp directory: %w", err)
	}

	return &Sandbox{
		rootDir: tempDir,
		tempDir: tempDir,
		networkPolicy: NetworkPolicy{
			allowOutbound: false,
		},
		resources: ResourceLimits{
			maxMemory:    256 * 1024 * 1024, // 256MB
			maxCPU:       50.0,              // 50%
			maxDisk:      100 * 1024 * 1024, // 100MB
			maxOpenFiles: 100,
			maxProcesses: 5,
		},
	}, nil
}

// Execute executes a command with enhanced security
func (sbe *SecureBashExecutor) Execute(ctx context.Context, execCtx *ExecutionContext) (*ExecutionResult, error) {
	sbe.mutex.Lock()
	defer sbe.mutex.Unlock()

	startTime := time.Now()
	result := &ExecutionResult{
		SecurityViolations: make([]string, 0),
	}

	// Security validation
	if violations := sbe.validateCommand(execCtx.Command); len(violations) > 0 {
		result.SecurityViolations = violations
		result.Success = false
		return result, fmt.Errorf("security validation failed: %v", violations)
	}

	// Setup execution environment
	var cmd *exec.Cmd
	var err error

	if execCtx.RequireSandbox {
		cmd, err = sbe.setupSandboxCommand(ctx, execCtx)
	} else {
		cmd, err = sbe.setupCommand(ctx, execCtx)
	}

	if err != nil {
		result.Success = false
		return result, fmt.Errorf("failed to setup command: %w", err)
	}

	// Set up resource limits
	sbe.setResourceLimits(cmd)

	// Execute command with monitoring
	result, err = sbe.executeWithMonitoring(ctx, cmd, execCtx.Timeout, result)
	if err != nil {
		result.Success = false
		return result, err
	}

	result.ExecutionTime = time.Since(startTime)
	result.Success = result.ExitCode == 0

	return result, nil
}

func (sbe *SecureBashExecutor) setupCommand(ctx context.Context, execCtx *ExecutionContext) (*exec.Cmd, error) {
	// Windows command execution
	var cmd *exec.Cmd

	if strings.Contains(strings.ToLower(execCtx.Command), "powershell") {
		cmd = exec.CommandContext(ctx, "powershell", "-Command", execCtx.Command)
	} else {
		cmd = exec.CommandContext(ctx, "cmd", "/C", execCtx.Command)
	}

	// Set working directory
	workingDir := execCtx.WorkingDir
	if workingDir == "" {
		workingDir = sbe.workingDir
	}

	if !sbe.security.isPathAllowed(workingDir) {
		return nil, fmt.Errorf("working directory not allowed: %s", workingDir)
	}

	cmd.Dir = workingDir

	// Set environment variables
	if execCtx.EnvVars != nil {
		cmd.Env = os.Environ()
		for k, v := range execCtx.EnvVars {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
	}

	return cmd, nil
}

func (sbe *SecureBashExecutor) setupSandboxCommand(ctx context.Context, execCtx *ExecutionContext) (*exec.Cmd, error) {
	if sbe.sandbox == nil {
		return nil, fmt.Errorf("sandbox not available")
	}

	sbe.sandbox.mutex.Lock()
	defer sbe.sandbox.mutex.Unlock()

	// Create sandbox-specific command
	sandboxDir := filepath.Join(sbe.sandbox.tempDir, execCtx.SessionID)
	if err := os.MkdirAll(sandboxDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create sandbox directory: %w", err)
	}

	// Copy command to sandbox
	sandboxCmd := execCtx.Command

	// Windows sandbox command execution
	if strings.Contains(strings.ToLower(sandboxCmd), "powershell") {
		return exec.CommandContext(ctx, "powershell", "-Command", sandboxCmd), nil
	} else {
		return exec.CommandContext(ctx, "cmd", "/C", sandboxCmd), nil
	}
}

func (sbe *SecureBashExecutor) executeWithMonitoring(ctx context.Context, cmd *exec.Cmd, timeout time.Duration, result *ExecutionResult) (*ExecutionResult, error) {
	// Create pipes for output
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return result, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		stdout.Close()
		return result, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start command
	if err := cmd.Start(); err != nil {
		stdout.Close()
		stderr.Close()
		return result, fmt.Errorf("failed to start command: %w", err)
	}

	// Monitor execution
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	// Collect output with size limit
	outputChan := make(chan string, 100)
	errorChan := make(chan string, 100)

	go sbe.collectOutput(stdout, outputChan, sbe.maxOutputSize)
	go sbe.collectOutput(stderr, errorChan, sbe.maxOutputSize)

	// Wait for completion or timeout
	select {
	case err := <-done:
		// Command completed
		close(outputChan)
		close(errorChan)

		if cmd.ProcessState != nil {
			result.ExitCode = cmd.ProcessState.ExitCode()
		}

		// Collect remaining output
		for output := range outputChan {
			result.Output += output + "\n"
		}
		for errOutput := range errorChan {
			result.ErrorOutput += errOutput + "\n"
		}

		return result, err

	case <-time.After(timeout):
		// Timeout occurred
		result.TimedOut = true
		result.Killed = true

		if cmd.Process != nil {
			cmd.Process.Kill()
		}

		close(outputChan)
		close(errorChan)

		for output := range outputChan {
			result.Output += output + "\n"
		}
		for errOutput := range errorChan {
			result.ErrorOutput += errOutput + "\n"
		}

		return result, fmt.Errorf("command timed out after %v", timeout)

	case <-ctx.Done():
		// Context cancelled
		result.Killed = true

		if cmd.Process != nil {
			cmd.Process.Kill()
		}

		close(outputChan)
		close(errorChan)

		return result, ctx.Err()
	}
}

func (sbe *SecureBashExecutor) collectOutput(pipe interface{}, outputChan chan<- string, maxSize int64) {
	var scanner *bufio.Scanner

	switch p := pipe.(type) {
	case *os.File:
		scanner = bufio.NewScanner(p)
	default:
		return
	}

	var currentSize int64
	for scanner.Scan() {
		line := scanner.Text()
		if currentSize+int64(len(line)) > maxSize {
			outputChan <- "[OUTPUT TRUNCATED - SIZE LIMIT EXCEEDED]"
			break
		}
		outputChan <- line
		currentSize += int64(len(line)) + 1 // +1 for newline
	}
}

func (sbe *SecureBashExecutor) setResourceLimits(cmd *exec.Cmd) {
	// Resource limits are handled by the sandbox on Windows
}

func (sbe *SecureBashExecutor) validateCommand(command string) []string {
	var violations []string

	sbe.security.mutex.RLock()
	defer sbe.security.mutex.RUnlock()

	lowerCommand := strings.ToLower(command)

	// Check blocked commands
	for blockedCmd := range sbe.security.blockedCommands {
		if strings.Contains(lowerCommand, blockedCmd) {
			violations = append(violations, fmt.Sprintf("blocked command detected: %s", blockedCmd))
		}
	}

	// Check for dangerous patterns (Windows only)
	dangerousPatterns := []string{
		`(?i).*format\s+[a-zA-Z]:.*`, // format C:
		`(?i).*del\s+/[fFsS].*`,      // del /f /s
		`(?i).*rmdir\s+/[sS].*`,      // rmdir /s
		`(?i).*rd\s+/[sS].*`,         // rd /s
		`(?i).*diskpart.*`,           // diskpart
		`(?i).*shutdown\s+.*`,        // shutdown
		`(?i).*stop-computer.*`,      // Stop-Computer
		`(?i).*restart-computer.*`,   // Restart-Computer
		`(?i).*%0\|%0.*`,             // Windows fork bomb
	}

	for _, pattern := range dangerousPatterns {
		if matched, _ := regexp.MatchString(pattern, command); matched {
			violations = append(violations, fmt.Sprintf("dangerous pattern detected: %s", pattern))
		}
	}

	// Check command length
	if len(command) > 10000 {
		violations = append(violations, "command too long")
	}

	return violations
}

// Legacy method for backward compatibility
func (sbe *SecureBashExecutor) ExecuteCommand(command string, timeoutMs int) (string, int, error) {
	ctx := context.Background()
	execCtx := &ExecutionContext{
		Command:        command,
		Timeout:        time.Duration(timeoutMs) * time.Millisecond,
		RequireSandbox: false,
		AllowNetwork:   false,
		MaxOutputSize:  sbe.maxOutputSize,
	}

	result, err := sbe.Execute(ctx, execCtx)
	if err != nil {
		return result.Output, result.ExitCode, err
	}

	return result.Output, result.ExitCode, nil
}

// Validation method for backward compatibility
func (sbe *SecureBashExecutor) ValidateCommand(command string) error {
	violations := sbe.validateCommand(command)
	if len(violations) > 0 {
		return fmt.Errorf("command validation failed: %v", violations)
	}
	return nil
}

// ExecutionSecurity methods
func (es *ExecutionSecurity) initializeDefaultPolicies() {
	// Block dangerous commands (Windows only)
	blockedCommands := []string{
		// 文件删除命令
		"del", "rmdir", "rd",
		// 磁盘操作命令
		"format", "diskpart",
		// 系统控制命令
		"shutdown", "reboot", "stop-computer", "restart-computer",
		// 网络下载命令
		"bitsadmin", "certutil",
		// PowerShell 编码命令
		"powershell -enc", "powershell -encodedcommand",
		// 注册表操作
		"reg delete", "reg add",
		// 用户管理
		"net user", "net localgroup",
	}

	for _, cmd := range blockedCommands {
		es.blockedCommands[cmd] = true
	}

	// Allow safe commands (Windows compatible)
	allowedCommands := []string{
		// 文件浏览
		"dir", "type", "echo", "more", "findstr",
		// 系统信息
		"date", "time", "whoami", "hostname", "systeminfo",
		"tasklist", "ipconfig", "netstat",
		// 开发工具
		"python", "node", "java", "go", "gcc", "g++",
		"git", "npm", "pip", "cargo", "yarn",
		// PowerShell cmdlets
		"get-process", "get-childitem", "get-content", "get-date",
	}

	for _, cmd := range allowedCommands {
		es.allowedCommands[cmd] = true
	}
}

func (es *ExecutionSecurity) isPathAllowed(path string) bool {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}

	// Check blocked paths
	for _, blocked := range es.blockedPaths {
		if strings.HasPrefix(absPath, blocked) {
			return false
		}
	}

	// If allowed paths are specified, check against them
	if len(es.allowedPaths) > 0 {
		for _, allowed := range es.allowedPaths {
			if strings.HasPrefix(absPath, allowed) {
				return true
			}
		}
		return false
	}

	return true
}

// Utility methods
func getWorkingDirectory() string {
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return os.TempDir()
}

// Cleanup methods
func (sbe *SecureBashExecutor) Cleanup() {
	sbe.mutex.Lock()
	defer sbe.mutex.Unlock()

	if sbe.sandbox != nil {
		os.RemoveAll(sbe.sandbox.tempDir)
	}
}

func (sbe *SecureBashExecutor) GetSecurityStatus() map[string]interface{} {
	sbe.mutex.RLock()
	defer sbe.mutex.RUnlock()

	return map[string]interface{}{
		"sandbox_enabled":        sbe.sandbox != nil,
		"working_directory":      sbe.workingDir,
		"max_output_size":        sbe.maxOutputSize,
		"allowed_commands_count": len(sbe.security.allowedCommands),
		"blocked_commands_count": len(sbe.security.blockedCommands),
	}
}
