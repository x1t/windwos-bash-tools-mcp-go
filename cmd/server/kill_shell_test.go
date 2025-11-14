package main

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// KillShellHandlerTestSuite KillShell处理器测试套件
type KillShellHandlerTestSuite struct {
	suite.Suite
	server *MCPServer
}

// SetupSuite 测试套件初始化
func (suite *KillShellHandlerTestSuite) SetupSuite() {
	suite.server = NewMCPServer()
}

// TearDownSuite 测试套件清理
func (suite *KillShellHandlerTestSuite) TearDownSuite() {
	// 清理所有后台任务
	suite.server.mutex.Lock()
	defer suite.server.mutex.Unlock()
	
	// 清理临时文件
	for _, task := range suite.server.backgroundTasks {
		if task.TempFile != "" {
			// 在实际环境中会清理临时文件，这里暂时跳过
		}
	}
	suite.server.backgroundTasks = make(map[string]*BackgroundTask)
}

// TestKillShellHandler_EmptyShellID 测试空shell_id错误
func (suite *KillShellHandlerTestSuite) TestKillShellHandler_EmptyShellID() {
	args := KillShellArguments{
		ShellID: "",
	}

	// 根据官方MCP标准，工具错误返回nil + 结构化输出
	result, output, err := suite.server.KillShellHandler(context.Background(), &mcp.CallToolRequest{}, args)

	require.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "shell_id参数是必需的")
	
	// 工具错误时result为nil
	assert.Nil(suite.T(), result)
	assert.Equal(suite.T(), "", output.ShellID)
}

// TestKillShellHandler_TooLongShellID 测试过长的shell_id
func (suite *KillShellHandlerTestSuite) TestKillShellHandler_TooLongShellID() {
	longID := strings.Repeat("x", 101) // 101个字符
	args := KillShellArguments{
		ShellID: longID,
	}

	result, output, err := suite.server.KillShellHandler(context.Background(), &mcp.CallToolRequest{}, args)

	require.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "shell_id过长")
	
	// 工具错误时result为nil
	assert.Nil(suite.T(), result)
	assert.Equal(suite.T(), longID, output.ShellID)
}

// TestKillShellHandler_TaskNotFound 测试任务不存在
func (suite *KillShellHandlerTestSuite) TestKillShellHandler_TaskNotFound() {
	args := KillShellArguments{
		ShellID: "nonexistent_shell_id",
	}

	result, output, err := suite.server.KillShellHandler(context.Background(), &mcp.CallToolRequest{}, args)

	require.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "未找到后台任务")
	
	// 工具错误时result为nil
	assert.Nil(suite.T(), result)
	assert.Equal(suite.T(), "nonexistent_shell_id", output.ShellID)
}

// TestKillShellHandler_KillRunningTask 测试终止运行中的任务
func (suite *KillShellHandlerTestSuite) TestKillShellHandler_KillRunningTask() {
	// 创建一个运行中的任务
	taskID := "test_running_kill_12345"
	task := &BackgroundTask{
		ID:        taskID,
		Command:   "sleep 30",
		Output:    "Sleeping...\n",
		Status:    "running",
		StartTime: time.Now(),
	}

	suite.server.mutex.Lock()
	suite.server.backgroundTasks[taskID] = task
	suite.server.mutex.Unlock()

	// 验证任务存在
	suite.server.mutex.RLock()
	_, exists := suite.server.backgroundTasks[taskID]
	suite.server.mutex.RUnlock()
	assert.True(suite.T(), exists, "任务应该存在")

	args := KillShellArguments{
		ShellID: taskID,
	}

	// 根据官方MCP标准，成功操作返回nil
	result, output, err := suite.server.KillShellHandler(context.Background(), &mcp.CallToolRequest{}, args)

	require.NoError(suite.T(), err)
	// 成功操作返回nil
	assert.Nil(suite.T(), result)
	assert.Equal(suite.T(), "Background task "+taskID+" killed successfully", output.Message)
	assert.Equal(suite.T(), taskID, output.ShellID)

	// 验证任务已被删除
	suite.server.mutex.RLock()
	_, exists = suite.server.backgroundTasks[taskID]
	suite.server.mutex.RUnlock()
	assert.False(suite.T(), exists, "任务应该被删除")
}

// TestKillShellHandler_KillCompletedTask 测试终止已完成的任务
func (suite *KillShellHandlerTestSuite) TestKillShellHandler_KillCompletedTask() {
	// 创建一个已完成的任务
	taskID := "test_completed_kill_12345"
	exitCode := 0
	task := &BackgroundTask{
		ID:        taskID,
		Command:   "echo completed",
		Output:    "completed\n",
		Status:    "completed",
		StartTime: time.Now().Add(-1 * time.Second), // 1秒前开始
		ExitCode:  &exitCode,
	}

	suite.server.mutex.Lock()
	suite.server.backgroundTasks[taskID] = task
	suite.server.mutex.Unlock()

	args := KillShellArguments{
		ShellID: taskID,
	}

	// 根据官方MCP标准，成功操作返回nil
	result, output, err := suite.server.KillShellHandler(context.Background(), &mcp.CallToolRequest{}, args)

	require.NoError(suite.T(), err)
	// 成功操作返回nil
	assert.Nil(suite.T(), result)
	assert.Equal(suite.T(), "Background task "+taskID+" killed successfully", output.Message)
	assert.Equal(suite.T(), taskID, output.ShellID)

	// 验证任务已被删除
	suite.server.mutex.RLock()
	_, exists := suite.server.backgroundTasks[taskID]
	suite.server.mutex.RUnlock()
	assert.False(suite.T(), exists, "任务应该被删除")
}

// TestKillShellHandler_KillFailedTask 测试终止失败的任务
func (suite *KillShellHandlerTestSuite) TestKillShellHandler_KillFailedTask() {
	// 创建一个失败的任务
	taskID := "test_failed_kill_12345"
	exitCode := 1
	task := &BackgroundTask{
		ID:        taskID,
		Command:   "invalid_command",
		Output:    "Error: command not found\n",
		Status:    "failed",
		StartTime: time.Now().Add(-2 * time.Second), // 2秒前开始
		Error:     "command not found",
		ExitCode:  &exitCode,
	}

	suite.server.mutex.Lock()
	suite.server.backgroundTasks[taskID] = task
	suite.server.mutex.Unlock()

	args := KillShellArguments{
		ShellID: taskID,
	}

	// 根据官方MCP标准，成功操作返回nil
	result, output, err := suite.server.KillShellHandler(context.Background(), &mcp.CallToolRequest{}, args)

	require.NoError(suite.T(), err)
	// 成功操作返回nil
	assert.Nil(suite.T(), result)
	assert.Equal(suite.T(), "Background task "+taskID+" killed successfully", output.Message)
	assert.Equal(suite.T(), taskID, output.ShellID)

	// 验证任务已被删除
	suite.server.mutex.RLock()
	_, exists := suite.server.backgroundTasks[taskID]
	suite.server.mutex.RUnlock()
	assert.False(suite.T(), exists, "任务应该被删除")
}

// TestKillShellHandler_KillKilledTask 测试终止已被终止的任务
func (suite *KillShellHandlerTestSuite) TestKillShellHandler_KillKilledTask() {
	// 创建一个已被终止的任务
	taskID := "test_killed_kill_12345"
	task := &BackgroundTask{
		ID:        taskID,
		Command:   "echo killed",
		Output:    "Partial output\n",
		Status:    "killed",
		StartTime: time.Now().Add(-3 * time.Second), // 3秒前开始
		Error:     "Task killed by previous request",
	}

	suite.server.mutex.Lock()
	suite.server.backgroundTasks[taskID] = task
	suite.server.mutex.Unlock()

	args := KillShellArguments{
		ShellID: taskID,
	}

	// 根据官方MCP标准，成功操作返回nil
	result, output, err := suite.server.KillShellHandler(context.Background(), &mcp.CallToolRequest{}, args)

	require.NoError(suite.T(), err)
	// 成功操作返回nil
	assert.Nil(suite.T(), result)
	assert.Equal(suite.T(), "Background task "+taskID+" killed successfully", output.Message)
	assert.Equal(suite.T(), taskID, output.ShellID)

	// 验证任务已被删除
	suite.server.mutex.RLock()
	_, exists := suite.server.backgroundTasks[taskID]
	suite.server.mutex.RUnlock()
	assert.False(suite.T(), exists, "任务应该被删除")
}

// TestKillShellHandler_ConcurrentKills 测试并发终止任务
func (suite *KillShellHandlerTestSuite) TestKillShellHandler_ConcurrentKills() {
	// 创建多个任务
	const numTasks = 10
	taskIDs := make([]string, numTasks)
	
	for i := 0; i < numTasks; i++ {
		taskID := "test_concurrent_kill_" + string(rune('A'+i))
		taskIDs[i] = taskID
		
		task := &BackgroundTask{
			ID:        taskID,
			Command:   "sleep 100",
			Output:    "Running task " + string(rune('A'+i)) + "\n",
			Status:    "running",
			StartTime: time.Now(),
		}
		
		suite.server.mutex.Lock()
		suite.server.backgroundTasks[taskID] = task
		suite.server.mutex.Unlock()
	}

	// 验证所有任务都被创建
	suite.server.mutex.RLock()
	assert.Len(suite.T(), suite.server.backgroundTasks, numTasks, "应该创建指定数量的任务")
	suite.server.mutex.RUnlock()

	// 并发终止所有任务
	var wg sync.WaitGroup
	for _, taskID := range taskIDs {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			
			args := KillShellArguments{
				ShellID: id,
			}

			// 根据官方MCP标准，成功操作返回nil
			result, output, err := suite.server.KillShellHandler(context.Background(), &mcp.CallToolRequest{}, args)

			assert.NoError(suite.T(), err)
			// 成功操作返回nil
			assert.Nil(suite.T(), result)
			assert.Equal(suite.T(), "Background task "+id+" killed successfully", output.Message)
			assert.Equal(suite.T(), id, output.ShellID)
		}(taskID)
	}

	wg.Wait()

	// 验证所有任务都已被删除
	suite.server.mutex.RLock()
	assert.Len(suite.T(), suite.server.backgroundTasks, 0, "所有任务都应该被删除")
	suite.server.mutex.RUnlock()
}

// TestKillShellHandler_MixedStatusTasks 测试终止不同状态的任务
func (suite *KillShellHandlerTestSuite) TestKillShellHandler_MixedStatusTasks() {
	// 创建不同状态的任务
	tasks := map[string]string{
		"running_task":  "running",
		"completed_task": "completed",
		"failed_task":    "failed",
		"killed_task":    "killed",
	}

	for taskID, status := range tasks {
		task := &BackgroundTask{
			ID:        taskID,
			Command:   "echo " + taskID,
			Output:    "Output for " + taskID + "\n",
			Status:    status,
			StartTime: time.Now().Add(-time.Duration(len(taskID)) * time.Second),
		}

		if status == "completed" {
			exitCode := 0
			task.ExitCode = &exitCode
		} else if status == "failed" {
			exitCode := 1
			task.ExitCode = &exitCode
			task.Error = "Task failed"
		} else if status == "killed" {
			task.Error = "Task was killed"
		}

		suite.server.mutex.Lock()
		suite.server.backgroundTasks[taskID] = task
		suite.server.mutex.Unlock()
	}

	// 逐一终止每个任务
	for taskID := range tasks {
		suite.Run("终止任务_"+taskID, func() {
			args := KillShellArguments{
				ShellID: taskID,
			}

			// 根据官方MCP标准，成功操作返回nil
			result, output, err := suite.server.KillShellHandler(context.Background(), &mcp.CallToolRequest{}, args)

			require.NoError(suite.T(), err)
			// 成功操作返回nil
			assert.Nil(suite.T(), result)
			assert.Equal(suite.T(), "Background task "+taskID+" killed successfully", output.Message)
			assert.Equal(suite.T(), taskID, output.ShellID)

			// 验证任务已被删除
			suite.server.mutex.RLock()
			_, exists := suite.server.backgroundTasks[taskID]
			suite.server.mutex.RUnlock()
			assert.False(suite.T(), exists, "任务 "+taskID+" 应该被删除")
		})
	}
}

// TestKillShellHandler_KillNonExistentTask 测试终止不存在的任务
func (suite *KillShellHandlerTestSuite) TestKillShellHandler_KillNonExistentTask() {
	// 确保任务列表为空
	suite.server.mutex.Lock()
	suite.server.backgroundTasks = make(map[string]*BackgroundTask)
	suite.server.mutex.Unlock()

	args := KillShellArguments{
		ShellID: "definitely_nonexistent_task",
	}

	result, output, err := suite.server.KillShellHandler(context.Background(), &mcp.CallToolRequest{}, args)

	require.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "未找到后台任务")
	
	// 工具错误时result为nil
	assert.Nil(suite.T(), result)
	assert.Equal(suite.T(), "definitely_nonexistent_task", output.ShellID)
}

// TestKillShellHandler_SpecialCharactersInShellID 测试包含特殊字符的ShellID
func (suite *KillShellHandlerTestSuite) TestKillShellHandler_SpecialCharactersInShellID() {
	// 创建包含特殊字符的任务ID
	taskID := "test_special_123_!@#$%^&*()_task"
	task := &BackgroundTask{
		ID:        taskID,
		Command:   "echo special chars",
		Output:    "Special output\n",
		Status:    "running",
		StartTime: time.Now(),
	}

	suite.server.mutex.Lock()
	suite.server.backgroundTasks[taskID] = task
	suite.server.mutex.Unlock()

	args := KillShellArguments{
		ShellID: taskID,
	}

	// 根据官方MCP标准，成功操作返回nil
	result, output, err := suite.server.KillShellHandler(context.Background(), &mcp.CallToolRequest{}, args)

	require.NoError(suite.T(), err)
	// 成功操作返回nil
	assert.Nil(suite.T(), result)
	assert.Equal(suite.T(), "Background task "+taskID+" killed successfully", output.Message)
	assert.Equal(suite.T(), taskID, output.ShellID)

	// 验证任务已被删除
	suite.server.mutex.RLock()
	_, exists := suite.server.backgroundTasks[taskID]
	suite.server.mutex.RUnlock()
	assert.False(suite.T(), exists, "任务应该被删除")
}

// 运行KillShellHandler测试套件
func TestKillShellHandlerTestSuite(t *testing.T) {
	suite.Run(t, new(KillShellHandlerTestSuite))
}

// 基准测试
func BenchmarkKillShellHandler_ValidTask(b *testing.B) {
	server := NewMCPServer()
	
	// 创建测试任务
	taskID := "benchmark_kill_task"
	task := &BackgroundTask{
		ID:        taskID,
		Command:   "echo benchmark",
		Output:    "benchmark output",
		Status:    "running",
		StartTime: time.Now(),
	}
	
	server.mutex.Lock()
	server.backgroundTasks[taskID] = task
	server.mutex.Unlock()

	args := KillShellArguments{
		ShellID: taskID,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// 每次测试后重新创建任务
		if i > 0 {
			server.mutex.Lock()
			server.backgroundTasks[taskID] = task
			server.mutex.Unlock()
		}
		
		_, _, err := server.KillShellHandler(context.Background(), &mcp.CallToolRequest{}, args)
		if err != nil {
			b.Fatal(err)
		}
	}
}