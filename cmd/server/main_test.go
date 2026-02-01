package main

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// BashHandlerTestSuite Bash工具处理器测试套件
type BashHandlerTestSuite struct {
	suite.Suite
	server *MCPServer
}

// SetupSuite 测试套件初始化
func (suite *BashHandlerTestSuite) SetupSuite() {
	suite.server = NewMCPServer()
	suite.server.shellExecutor = &MockShellExecutor{}
}

// TearDownSuite 测试套件清理
func (suite *BashHandlerTestSuite) TearDownSuite() {
	suite.server.mutex.Lock()
	defer suite.server.mutex.Unlock()
	suite.server.backgroundTasks = make(map[string]*BackgroundTask)
}

// TestBashHandler_EmptyCommand 测试空命令错误
func (suite *BashHandlerTestSuite) TestBashHandler_EmptyCommand() {
	args := BashArguments{
		Command: "",
		Timeout: 5000,
	}

	result, output, err := suite.server.BashHandler(context.Background(), &mcp.CallToolRequest{}, args)

	require.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "command is required")
	assert.Nil(suite.T(), result)
	assert.Equal(suite.T(), 1, output.ExitCode)
}

// TestBashHandler_InvalidTimeout 测试无效超时时间
func (suite *BashHandlerTestSuite) TestBashHandler_InvalidTimeout() {
	tests := []struct {
		name     string
		timeout  int
		expected string
	}{
		{"timeout too short", 500, "timeout must be between 1000 and 600000"},
		{"timeout too long", 700000, "timeout must be between 1000 and 600000"},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			args := BashArguments{
				Command: "echo test",
				Timeout: tt.timeout,
			}

			result, output, err := suite.server.BashHandler(context.Background(), &mcp.CallToolRequest{}, args)

			require.Error(suite.T(), err)
			assert.Contains(suite.T(), err.Error(), tt.expected)
			assert.Nil(suite.T(), result)
			assert.Equal(suite.T(), 1, output.ExitCode)
		})
	}
}

// TestBashHandler_DangerousCommand 测试危险命令拦截（Windows专用）
func (suite *BashHandlerTestSuite) TestBashHandler_DangerousCommand() {
	// 仅测试Windows危险命令（已移除Linux命令如 rm -rf /）
	dangerousCommands := []string{
		"del /s C:\\Windows",
		"format C:",
		"shutdown /s",
		"reboot",
		"rd /s /q C:\\",
		"diskpart",
	}

	for _, cmd := range dangerousCommands {
		suite.Run("dangerous: "+cmd, func() {
			args := BashArguments{
				Command: cmd,
				Timeout: 5000,
			}

			result, output, err := suite.server.BashHandler(context.Background(), &mcp.CallToolRequest{}, args)

			require.Error(suite.T(), err)
			assert.Contains(suite.T(), err.Error(), "command rejected for security reasons")
			assert.Nil(suite.T(), result)
			assert.Equal(suite.T(), 1, output.ExitCode)
		})
	}
}

// TestBashHandler_BackgroundExecution 测试后台命令执行
func (suite *BashHandlerTestSuite) TestBashHandler_BackgroundExecution() {
	args := BashArguments{
		Command:         "echo Hello Background",
		RunInBackground: true,
		Timeout:         5000,
	}

	result, output, err := suite.server.BashHandler(context.Background(), &mcp.CallToolRequest{}, args)

	require.NoError(suite.T(), err)
	assert.Nil(suite.T(), result)
	assert.Equal(suite.T(), 0, output.ExitCode)
	assert.NotEmpty(suite.T(), output.ShellID)
	assert.True(suite.T(), strings.HasPrefix(output.ShellID, "bash_"))

	suite.server.mutex.RLock()
	_, exists := suite.server.backgroundTasks[output.ShellID]
	suite.server.mutex.RUnlock()
	assert.True(suite.T(), exists, "background task should be created")

	suite.server.mutex.Lock()
	delete(suite.server.backgroundTasks, output.ShellID)
	suite.server.mutex.Unlock()
}

// TestBashHandler_ForegroundExecution 测试前台命令执行
func (suite *BashHandlerTestSuite) TestBashHandler_ForegroundExecution() {
	args := BashArguments{
		Command:         "echo Hello World",
		RunInBackground: false,
		Timeout:         5000,
	}

	result, output, err := suite.server.BashHandler(context.Background(), &mcp.CallToolRequest{}, args)

	require.NoError(suite.T(), err)
	assert.Nil(suite.T(), result)
	assert.Equal(suite.T(), 0, output.ExitCode)
	assert.False(suite.T(), output.Killed)
	assert.Empty(suite.T(), output.ShellID)
}

// TestBashHandler_ConcurrentBackgroundTasks 测试并发后台任务
func (suite *BashHandlerTestSuite) TestBashHandler_ConcurrentBackgroundTasks() {
	const numTasks = 10
	var wg sync.WaitGroup
	taskIDs := make([]string, 0, numTasks)
	var mu sync.Mutex

	for i := 0; i < numTasks; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			args := BashArguments{
				Command:         "echo Task " + string(rune('A'+index)),
				RunInBackground: true,
				Timeout:         5000,
			}

			result, output, err := suite.server.BashHandler(context.Background(), &mcp.CallToolRequest{}, args)

			assert.NoError(suite.T(), err)
			assert.Nil(suite.T(), result)
			assert.NotEmpty(suite.T(), output.ShellID)

			mu.Lock()
			taskIDs = append(taskIDs, output.ShellID)
			mu.Unlock()
		}(i)
	}

	wg.Wait()

	assert.Len(suite.T(), taskIDs, numTasks, "should create specified number of tasks")

	uniqueIDs := make(map[string]bool)
	for _, id := range taskIDs {
		assert.False(suite.T(), uniqueIDs[id], "task ID should be unique: %s", id)
		uniqueIDs[id] = true
	}

	suite.server.mutex.Lock()
	for _, id := range taskIDs {
		delete(suite.server.backgroundTasks, id)
	}
	suite.server.mutex.Unlock()
}

// TestBashHandler_DescriptionLogging 测试描述日志功能
func (suite *BashHandlerTestSuite) TestBashHandler_DescriptionLogging() {
	args := BashArguments{
		Command:     "echo test",
		Description: "test description",
		Timeout:     5000,
	}

	result, output, err := suite.server.BashHandler(context.Background(), &mcp.CallToolRequest{}, args)

	require.NoError(suite.T(), err)
	assert.Nil(suite.T(), result)
	assert.NotNil(suite.T(), output)
}

// 运行BashHandler测试套件
func TestBashHandlerTestSuite(t *testing.T) {
	suite.Run(t, new(BashHandlerTestSuite))
}

// MockShellExecutor 模拟Shell执行器
type MockShellExecutor struct{}

// ExecuteCommand 模拟命令执行
func (m *MockShellExecutor) ExecuteCommand(command string, timeout int) (string, int, error) {
	return "Mock output: " + command, 0, nil
}

// PrintShellInfo 模拟Shell信息打印
func (m *MockShellExecutor) PrintShellInfo() {}
