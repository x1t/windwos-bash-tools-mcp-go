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
	// 创建一个简单的ShellExecutor来避免实际执行命令
	suite.server.shellExecutor = &MockShellExecutor{}
}

// TearDownSuite 测试套件清理
func (suite *BashHandlerTestSuite) TearDownSuite() {
	// 清理所有后台任务
	suite.server.mutex.Lock()
	defer suite.server.mutex.Unlock()
	suite.server.backgroundTasks = make(map[string]*BackgroundTask)
}

// TestBashHandler_EmptyCommand 测试空命令错误
func (suite *BashHandlerTestSuite) TestBashHandler_EmptyCommand() {
	// 准备测试数据
	args := BashArguments{
		Command: "",
	}

	// 执行测试 - 根据官方MCP标准，工具错误返回nil + 结构化输出
	result, output, err := suite.server.BashHandler(context.Background(), &mcp.CallToolRequest{}, args)

	// 验证结果
	require.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "command参数是必需的")
	
	// 工具错误时result为nil，输出结构体包含错误信息
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
		{"超时时间过短", 500, "timeout必须在1000到600000毫秒之间"},
		{"超时时间过长", 700000, "timeout必须在1000到600000毫秒之间"},
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
			
			// 根据官方MCP标准，工具错误时result为nil
			assert.Nil(suite.T(), result)
			assert.Equal(suite.T(), 1, output.ExitCode)
		})
	}
}

// TestBashHandler_DangerousCommand 测试危险命令拦截
func (suite *BashHandlerTestSuite) TestBashHandler_DangerousCommand() {
	dangerousCommands := []string{
		"rm -rf /",
		"del /f C:\\*.*",
		"format C:",
		"shutdown /s",
		"reboot",
		"sudo rm -rf",
		"> /dev/null",
	}

	for _, cmd := range dangerousCommands {
		suite.Run("危险命令: "+cmd, func() {
			args := BashArguments{
				Command: cmd,
				Timeout: 5000,
			}

			result, output, err := suite.server.BashHandler(context.Background(), &mcp.CallToolRequest{}, args)

			require.Error(suite.T(), err)
			assert.Contains(suite.T(), err.Error(), "命令因安全原因被拒绝")
			
			// 根据官方MCP标准，工具错误时result为nil
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

	// 根据官方MCP标准，成功操作返回nil作为CallToolResult
	result, output, err := suite.server.BashHandler(context.Background(), &mcp.CallToolRequest{}, args)

	require.NoError(suite.T(), err)
	// 成功操作返回nil，结构化输出在第二个返回值中
	assert.Nil(suite.T(), result)
	assert.Equal(suite.T(), 0, output.ExitCode)
	assert.NotEmpty(suite.T(), output.ShellID)
	assert.True(suite.T(), strings.HasPrefix(output.ShellID, "bash_"))

	// 验证后台任务已被创建
	suite.server.mutex.RLock()
	_, exists := suite.server.backgroundTasks[output.ShellID]
	suite.server.mutex.RUnlock()
	assert.True(suite.T(), exists, "后台任务应该被创建")

	// 清理
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

	// 根据官方MCP标准，成功操作返回nil作为CallToolResult
	result, output, err := suite.server.BashHandler(context.Background(), &mcp.CallToolRequest{}, args)

	require.NoError(suite.T(), err)
	// 成功操作返回nil，结构化输出在第二个返回值中
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

			// 根据官方MCP标准，成功操作返回nil
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

	// 验证所有任务都被创建
	assert.Len(suite.T(), taskIDs, numTasks, "应该创建指定数量的后台任务")

	// 验证所有任务ID都是唯一的
	uniqueIDs := make(map[string]bool)
	for _, id := range taskIDs {
		assert.False(suite.T(), uniqueIDs[id], "任务ID应该是唯一的: %s", id)
		uniqueIDs[id] = true
	}

	// 清理
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
		Description: "测试命令描述",
		Timeout:     5000,
	}

	// 这里我们无法直接测试日志输出，但可以验证参数被正确处理
	// 根据官方MCP标准，成功操作返回nil
	result, output, err := suite.server.BashHandler(context.Background(), &mcp.CallToolRequest{}, args)

	require.NoError(suite.T(), err)
	assert.Nil(suite.T(), result)
	assert.NotNil(suite.T(), output)
}

// MockShellExecutor 模拟Shell执行器
type MockShellExecutor struct{}

// ExecuteCommand 模拟命令执行
func (m *MockShellExecutor) ExecuteCommand(command string, timeout int) (string, int, error) {
	// 模拟成功执行
	return "Mock output: " + command, 0, nil
}

// PrintShellInfo 模拟Shell信息打印
func (m *MockShellExecutor) PrintShellInfo() {
	// 什么都不做
}

// 运行BashHandler测试套件
func TestBashHandlerTestSuite(t *testing.T) {
	suite.Run(t, new(BashHandlerTestSuite))
}

// 单独的基准测试
func BenchmarkBashHandler_ValidCommand(b *testing.B) {
	server := NewMCPServer()
	// 不设置模拟执行器，使用默认的
	args := BashArguments{
		Command: "echo benchmark test",
		Timeout: 5000,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := server.BashHandler(context.Background(), &mcp.CallToolRequest{}, args)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBashHandler_BackgroundTask(b *testing.B) {
	server := NewMCPServer()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		args := BashArguments{
			Command:         "echo background task " + string(rune(i)),
			RunInBackground: true,
			Timeout:         5000,
		}
		
		_, _, err := server.BashHandler(context.Background(), &mcp.CallToolRequest{}, args)
		if err != nil {
			b.Fatal(err)
		}
		
		// 清理任务以避免内存泄漏
		server.mutex.Lock()
		server.backgroundTasks = make(map[string]*BackgroundTask)
		server.mutex.Unlock()
	}
}