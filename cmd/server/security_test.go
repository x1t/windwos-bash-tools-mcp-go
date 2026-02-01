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
func (suite *SecurityTestSuite) TestDangerousCommandDetection() {
	dangerousCommands := []string{
		"del /s /q C:\\Windows",
		"format C:",
		"shutdown /s",
		"shutdown /r",
		"rd /s /q C:\\Important",
		"rmdir /s /q C:\\Data",
		"diskpart",
		"Stop-Computer",
		"Restart-Computer",
		"takeown /f C:\\Windows",
		"net user hacker /add",
		"reg delete HKLM\\Software",
		"bitsadmin /transfer",
		"powershell -enc",
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
		{"shutdown", true},
		{";shutdown /s", true},
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
