package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// ForegroundTimeoutTestSuite 前台超时转后台测试套件
type ForegroundTimeoutTestSuite struct {
	suite.Suite
	server *MCPServer
}

// SetupSuite 测试套件初始化
func (suite *ForegroundTimeoutTestSuite) SetupSuite() {
	suite.server = NewMCPServer()
}

// TearDownSuite 测试套件清理
func (suite *ForegroundTimeoutTestSuite) TearDownSuite() {
	suite.server.mutex.Lock()
	defer suite.server.mutex.Unlock()
	suite.server.backgroundTasks = make(map[string]*BackgroundTask)
}

// TestForeground_CompletesBeforeTimeout 测试前台命令在超时前完成
func (suite *ForegroundTimeoutTestSuite) TestForeground_CompletesBeforeTimeout() {
	args := BashArguments{
		Command:         "Write-Output 'Quick command'",
		Timeout:         5000, // 5秒超时
		RunInBackground: false,
	}

	start := time.Now()
	_, result, err := suite.server.BashHandler(context.Background(), &mcp.CallToolRequest{}, args)
	duration := time.Since(start)

	// 验证：命令应该快速完成
	require.NoError(suite.T(), err)
	assert.Less(suite.T(), duration, 5*time.Second, "命令应该在超时前完成")
	assert.Equal(suite.T(), 0, result.ExitCode)
	assert.Contains(suite.T(), result.Output, "Quick command")
	assert.Empty(suite.T(), result.ShellID, "前台完成的命令不应该有ShellID")
	assert.False(suite.T(), result.Killed)
}

// TestForeground_TimeoutConvertsToBackground 测试前台命令超时自动转后台
func (suite *ForegroundTimeoutTestSuite) TestForeground_TimeoutConvertsToBackground() {
	// 使用一个会运行较长时间的命令（使用PowerShell的-Milliseconds参数避免分号）
	args := BashArguments{
		Command:         "Start-Sleep -Milliseconds 10000",
		Timeout:         2000, // 2秒超时
		RunInBackground: false,
	}

	start := time.Now()
	_, result, err := suite.server.BashHandler(context.Background(), &mcp.CallToolRequest{}, args)
	duration := time.Since(start)

	// 验证：应该在2秒左右返回（不会等待10秒）
	require.NoError(suite.T(), err)
	assert.Less(suite.T(), duration, 3*time.Second, "应该在超时后立即返回，不阻塞Agent")
	assert.Greater(suite.T(), duration, 2*time.Second, "应该等待超时时间")

	// 验证：返回的是后台任务信息
	assert.NotEmpty(suite.T(), result.ShellID, "超时后应该返回ShellID")
	assert.True(suite.T(), strings.HasPrefix(result.ShellID, "bash_"), "ShellID应该以bash_开头")
	assert.Contains(suite.T(), result.Output, "converted to background task", "输出应该说明已转后台")
	assert.Contains(suite.T(), result.Output, result.ShellID, "输出应该包含任务ID")
	assert.Equal(suite.T(), 0, result.ExitCode, "转后台不是错误，exitCode应该是0")
	assert.False(suite.T(), result.Killed, "任务没有被kill，只是转后台")

	// 验证：任务已经在后台任务列表中
	suite.server.mutex.RLock()
	task, exists := suite.server.backgroundTasks[result.ShellID]
	suite.server.mutex.RUnlock()

	assert.True(suite.T(), exists, "任务应该在后台任务列表中")
	assert.Equal(suite.T(), "running", task.Status, "任务应该处于运行状态")
	assert.Equal(suite.T(), args.Command, task.Command)

	// 等待一段时间，验证任务仍在执行
	time.Sleep(3 * time.Second)

	suite.server.mutex.RLock()
	task, exists = suite.server.backgroundTasks[result.ShellID]
	suite.server.mutex.RUnlock()

	assert.True(suite.T(), exists, "任务应该仍然存在")
	// 任务可能还在运行、已完成或失败
	assert.Contains(suite.T(), []string{"running", "completed", "failed"}, task.Status)

	// 清理
	suite.server.mutex.Lock()
	delete(suite.server.backgroundTasks, result.ShellID)
	suite.server.mutex.Unlock()
}

// TestForeground_TimeoutThenCheckOutput 测试超时转后台后使用bash_output查看
func (suite *ForegroundTimeoutTestSuite) TestForeground_TimeoutThenCheckOutput() {
	// 1. 启动一个会超时的前台命令（简单命令避免安全检查）
	args := BashArguments{
		Command:         "Start-Sleep -Milliseconds 8000",
		Timeout:         2000, // 2秒超时
		RunInBackground: false,
	}

	_, bashResult, err := suite.server.BashHandler(context.Background(), &mcp.CallToolRequest{}, args)
	require.NoError(suite.T(), err)
	require.NotEmpty(suite.T(), bashResult.ShellID, "应该返回ShellID")

	taskID := bashResult.ShellID

	// 2. 立即使用bash_output查看任务状态
	outputArgs := BashOutputArguments{
		BashID: taskID,
	}

	_, outputResult, err := suite.server.BashOutputHandler(context.Background(), &mcp.CallToolRequest{}, outputArgs)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), "running", outputResult.Status, "任务应该正在运行")

	// 3. 等待任务完成
	time.Sleep(10 * time.Second)

	// 4. 再次查看输出，应该看到任务已完成
	_, outputResult2, err := suite.server.BashOutputHandler(context.Background(), &mcp.CallToolRequest{}, outputArgs)
	require.NoError(suite.T(), err)
	assert.Contains(suite.T(), []string{"completed", "failed"}, outputResult2.Status, "任务应该已完成或失败")

	// 清理
	suite.server.mutex.Lock()
	delete(suite.server.backgroundTasks, taskID)
	suite.server.mutex.Unlock()
}

// TestForeground_MultipleTimeouts 测试多个前台命令同时超时
func (suite *ForegroundTimeoutTestSuite) TestForeground_MultipleTimeouts() {
	numCommands := 5
	taskIDs := make([]string, 0, numCommands)

	// 启动多个会超时的命令
	for i := 0; i < numCommands; i++ {
		args := BashArguments{
			Command:         "Start-Sleep -Milliseconds 5000",
			Timeout:         1000, // 1秒超时
			RunInBackground: false,
		}

		_, result, err := suite.server.BashHandler(context.Background(), &mcp.CallToolRequest{}, args)
		require.NoError(suite.T(), err)
		require.NotEmpty(suite.T(), result.ShellID)
		taskIDs = append(taskIDs, result.ShellID)
	}

	// 验证所有任务都在后台列表中
	suite.server.mutex.RLock()
	assert.Len(suite.T(), suite.server.backgroundTasks, numCommands, "应该有指定数量的后台任务")
	suite.server.mutex.RUnlock()

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

// TestForeground_TimeoutWithDescription 测试带描述的超时命令
func (suite *ForegroundTimeoutTestSuite) TestForeground_TimeoutWithDescription() {
	args := BashArguments{
		Command:         "Start-Sleep -Seconds 5",
		Timeout:         1000,
		Description:     "Long running compilation task",
		RunInBackground: false,
	}

	_, result, err := suite.server.BashHandler(context.Background(), &mcp.CallToolRequest{}, args)
	require.NoError(suite.T(), err)
	assert.NotEmpty(suite.T(), result.ShellID)

	// 验证任务信息
	suite.server.mutex.RLock()
	task, exists := suite.server.backgroundTasks[result.ShellID]
	suite.server.mutex.RUnlock()

	assert.True(suite.T(), exists)
	assert.Equal(suite.T(), args.Command, task.Command)

	// 清理
	suite.server.mutex.Lock()
	delete(suite.server.backgroundTasks, result.ShellID)
	suite.server.mutex.Unlock()
}

// TestForeground_VeryShortTimeout 测试极短超时（边界条件）
func (suite *ForegroundTimeoutTestSuite) TestForeground_VeryShortTimeout() {
	args := BashArguments{
		Command:         "Start-Sleep -Seconds 3",
		Timeout:         1000, // 最小超时1秒
		RunInBackground: false,
	}

	start := time.Now()
	_, result, err := suite.server.BashHandler(context.Background(), &mcp.CallToolRequest{}, args)
	duration := time.Since(start)

	require.NoError(suite.T(), err)
	assert.Less(suite.T(), duration, 2*time.Second, "应该快速返回")
	assert.NotEmpty(suite.T(), result.ShellID, "应该转为后台任务")

	// 清理
	suite.server.mutex.Lock()
	delete(suite.server.backgroundTasks, result.ShellID)
	suite.server.mutex.Unlock()
}

// TestForeground_TimeoutThenKill 测试超时转后台后可以被kill
func (suite *ForegroundTimeoutTestSuite) TestForeground_TimeoutThenKill() {
	// 1. 启动会超时的命令
	args := BashArguments{
		Command:         "Start-Sleep -Seconds 30",
		Timeout:         1000,
		RunInBackground: false,
	}

	_, bashResult, err := suite.server.BashHandler(context.Background(), &mcp.CallToolRequest{}, args)
	require.NoError(suite.T(), err)
	require.NotEmpty(suite.T(), bashResult.ShellID)

	taskID := bashResult.ShellID

	// 2. 验证任务在运行
	suite.server.mutex.RLock()
	task, exists := suite.server.backgroundTasks[taskID]
	suite.server.mutex.RUnlock()
	assert.True(suite.T(), exists)
	assert.Equal(suite.T(), "running", task.Status)

	// 3. 终止任务
	killArgs := KillShellArguments{
		ShellID: taskID,
	}

	_, killResult, err := suite.server.KillShellHandler(context.Background(), &mcp.CallToolRequest{}, killArgs)
	require.NoError(suite.T(), err)
	assert.Contains(suite.T(), killResult.Message, "killed successfully")

	// 4. 验证任务已被删除
	suite.server.mutex.RLock()
	_, exists = suite.server.backgroundTasks[taskID]
	suite.server.mutex.RUnlock()
	assert.False(suite.T(), exists, "任务应该已被删除")
}

// 运行前台超时测试套件
func TestForegroundTimeoutTestSuite(t *testing.T) {
	suite.Run(t, new(ForegroundTimeoutTestSuite))
}
