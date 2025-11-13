package windows

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

// Windows API 相关常量
const (
	STD_OUTPUT_HANDLE = ^uint32(0) - 11
	STD_ERROR_HANDLE  = ^uint32(0) - 12
	
	ENABLE_VIRTUAL_TERMINAL_PROCESSING = 0x0004
	
	CP_UTF8 = 65001
)

var (
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	
	procGetConsoleMode      = kernel32.NewProc("GetConsoleMode")
	procSetConsoleMode      = kernel32.NewProc("SetConsoleMode")
	procGetStdHandle        = kernel32.NewProc("GetStdHandle")
	procSetConsoleOutputCP  = kernel32.NewProc("SetConsoleOutputCP")
	procGetConsoleOutputCP  = kernel32.NewProc("GetConsoleOutputCP")
)

// OptimizedCommandExecutor Windows优化命令执行器
type OptimizedCommandExecutor struct {
	enableVTProcessing bool
	utf8Enabled        bool
}

func NewOptimizedCommandExecutor() *OptimizedCommandExecutor {
	executor := &OptimizedCommandExecutor{}
	executor.initializeConsole()
	return executor
}

// initializeConsole 初始化Windows控制台
func (oce *OptimizedCommandExecutor) initializeConsole() {
	// 设置UTF-8编码
	oce.setUTF8Encoding()
	
	// 启用虚拟终端处理
	oce.enableVirtualTerminal()
}

// setUTF8Encoding 设置控制台UTF-8编码
func (oce *OptimizedCommandExecutor) setUTF8Encoding() {
	procSetConsoleOutputCP.Call(uintptr(CP_UTF8))
	oce.utf8Enabled = true
}

// enableVirtualTerminal 启用虚拟终端处理（ANSI颜色支持）
func (oce *OptimizedCommandExecutor) enableVirtualTerminal() {
	stdOutHandle, _, _ := procGetStdHandle.Call(uintptr(STD_OUTPUT_HANDLE))
	if stdOutHandle == 0 {
		return
	}

	var mode uint32
	ret, _, _ := procGetConsoleMode.Call(stdOutHandle, uintptr(unsafe.Pointer(&mode)))
	if ret == 0 {
		return
	}

	mode |= ENABLE_VIRTUAL_TERMINAL_PROCESSING
	procSetConsoleMode.Call(stdOutHandle, uintptr(mode))
	oce.enableVTProcessing = true
}

// ExecuteCommandWithOptimization 执行Windows优化命令
func (oce *OptimizedCommandExecutor) ExecuteCommandWithOptimization(command string, workDir string) (*exec.Cmd, error) {
	// 检测命令类型并选择最佳执行方式
	var cmd *exec.Cmd
	
	commandLower := strings.ToLower(command)
	
	switch {
	case strings.Contains(commandLower, "powershell"):
		// PowerShell命令优化
		cmd = exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", command)
		
	case strings.Contains(commandLower, "git"):
		// Git命令优化
		if oce.isGitBashAvailable() {
			cmd = exec.Command("bash", "-c", command)
		} else {
			cmd = exec.Command("cmd", "/C", command)
		}
		
	case strings.HasPrefix(commandLower, "dir ") || strings.HasPrefix(commandLower, "type "):
		// Windows原生命令
		cmd = exec.Command("cmd", "/C", command)
		
	default:
		// 默认使用cmd
		cmd = exec.Command("cmd", "/C", command)
	}
	
	// 设置工作目录
	if workDir != "" {
		if absPath, err := filepath.Abs(workDir); err == nil {
			cmd.Dir = absPath
		}
	}
	
	// 设置环境变量优化
	oce.setupEnvironment(cmd)
	
	return cmd, nil
}

// setupEnvironment 设置执行环境
func (oce *OptimizedCommandExecutor) setupEnvironment(cmd *exec.Cmd) {
	cmd.Env = append(cmd.Env, os.Environ()...)
	
	// Windows特定环境变量
	cmd.Env = append(cmd.Env,
		"PROMPT=$P$G",
		"TERM=xterm-256color",
	)
	
	if oce.utf8Enabled {
		cmd.Env = append(cmd.Env, "PYTHONIOENCODING=utf-8")
	}
}

// isGitBashAvailable 检查Git Bash是否可用
func (oce *OptimizedCommandExecutor) isGitBashAvailable() bool {
	// 检查常见的Git Bash安装路径
	gitBashPaths := []string{
		"C:\\Program Files\\Git\\bin\\bash.exe",
		"C:\\Program Files (x86)\\Git\\bin\\bash.exe",
		"C:\\msys64\\usr\\bin\\bash.exe",
	}
	
	for _, path := range gitBashPaths {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	
	// 检查PATH中是否有bash
	if _, err := exec.LookPath("bash"); err == nil {
		return true
	}
	
	return false
}

// GetOptimalShell 获取最优的Shell环境
func (oce *OptimizedCommandExecutor) GetOptimalShell() string {
	if oce.isGitBashAvailable() {
		return "bash"
	}
	return "cmd"
}

// OptimizePath 优化Windows路径
func OptimizePath(path string) string {
	// 转换为绝对路径
	if absPath, err := filepath.Abs(path); err == nil {
		path = absPath
	}
	
	// 转换正斜杠为反斜杠
	path = filepath.FromSlash(path)
	
	// 处理长路径
	if len(path) > 260 {
		if !strings.HasPrefix(path, "\\\\?\\") {
			path = "\\\\?\\" + path
		}
	}
	
	return path
}

// IsWindows 检测是否为Windows系统
func IsWindows() bool {
	return strings.ToLower(os.Getenv("OS")) == "windows_NT" ||
		strings.Contains(strings.ToLower(os.Getenv("PATH")), "windows")
}

// GetWindowsInfo 获取Windows系统信息
type WindowsInfo struct {
	Version       string
	Build         string
	Architecture  string
	ProcessorInfo string
}

func GetWindowsInfo() WindowsInfo {
	info := WindowsInfo{}
	
	// 使用ver命令获取Windows版本
	if cmd := exec.Command("cmd", "/C", "ver"); cmd != nil {
		if output, err := cmd.CombinedOutput(); err == nil {
			info.Version = strings.TrimSpace(string(output))
		}
	}
	
	// 获取系统架构
	info.Architecture = os.Getenv("PROCESSOR_ARCHITECTURE")
	info.ProcessorInfo = os.Getenv("PROCESSOR_IDENTIFIER")
	
	return info
}

// OptimizeConcurrentExecution Windows并发执行优化
func (oce *OptimizedCommandExecutor) OptimizeConcurrentExecution() {
	// 在Windows上调整并发参数
	// 这里可以根据系统资源动态调整GOMAXPROCS等参数
}