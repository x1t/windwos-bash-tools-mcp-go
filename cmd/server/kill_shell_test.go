package main

import (
	"context"
	"os"
	"os/exec"
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
	suite.server.mutex.Lock()
	defer suite.server.mutex.Unlock()

	for _, task := range suite.server.backgroundTasks {
		if task.TempFile != "" {
			os.Remove(task.TempFile)
		}
	}
	suite.server.backgroundTasks = make(map[string]*BackgroundTask)
}

// TestKillShellHandler_EmptyShellID 测试空shell_id错误
func (suite *KillShellHandlerTestSuite) TestKillShellHandler_EmptyShellID() {
	args := KillShellArguments{
		ShellID: "",
	}

	_, output, err := suite.server.KillShellHandler(context.Background(), &mcp.CallToolRequest{}, args)

	require.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "shell_id is required")
	assert.Equal(suite.T(), "", output.ShellID)
}

// TestKillShellHandler_TooLongShellID 测试过长的shell_id
func (suite *KillShellHandlerTestSuite) TestKillShellHandler_TooLongShellID() {
	longID := strings.Repeat("a", 101)
	args := KillShellArguments{
		ShellID: longID,
	}

	_, output, err := suite.server.KillShellHandler(context.Background(), &mcp.CallToolRequest{}, args)

	require.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "shell_id is too long")
	assert.Equal(suite.T(), longID, output.ShellID)
}

// TestKillShellHandler_TaskNotFound 测试任务不存在
func (suite *KillShellHandlerTestSuite) TestKillShellHandler_TaskNotFound() {
	args := KillShellArguments{
		ShellID: "nonexistent_shell_id",
	}

	_, output, err := suite.server.KillShellHandler(context.Background(), &mcp.CallToolRequest{}, args)

	require.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "background task not found")
	assert.Equal(suite.T(), "nonexistent_shell_id", output.ShellID)
}

// TestKillShellHandler_KillRunningTask 测试终止运行中的任务
func (suite *KillShellHandlerTestSuite) TestKillShellHandler_KillRunningTask() {
	taskID := "test_running_kill_12345"

	// 创建一个模拟的进程（使用一个实际的短命令）
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, "powershell", "-Command", "Start-Sleep -Seconds 30")
	cmd.Start()

	task := &BackgroundTask{
		ID:        taskID,
		Command:   "Start-Sleep -Seconds 30",
		Output:    "Sleeping...\n",
		Status:    "running",
		StartTime: time.Now(),
		Process:   cmd.Process,
		Cancel:    cancel,
	}

	suite.server.mutex.Lock()
	suite.server.backgroundTasks[taskID] = task
	suite.server.mutex.Unlock()

	args := KillShellArguments{
		ShellID: taskID,
	}

	_, output, err := suite.server.KillShellHandler(context.Background(), &mcp.CallToolRequest{}, args)

	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), "Background task "+taskID+" killed successfully", output.Message)
	assert.Equal(suite.T(), taskID, output.ShellID)

	suite.server.mutex.RLock()
	_, exists := suite.server.backgroundTasks[taskID]
	suite.server.mutex.RUnlock()
	assert.False(suite.T(), exists, "任务应该被删除")

	// 等待一下确保进程被终止
	time.Sleep(100 * time.Millisecond)
}

// TestKillShellHandler_KillCompletedTask 测试终止已完成的任务
func (suite *KillShellHandlerTestSuite) TestKillShellHandler_KillCompletedTask() {
	taskID := "test_completed_kill_12345"
	exitCode := 0
	task := &BackgroundTask{
		ID:        taskID,
		Command:   "echo done",
		Output:    "done\n",
		Status:    "completed",
		StartTime: time.Now(),
		ExitCode:  &exitCode,
	}

	suite.server.mutex.Lock()
	suite.server.backgroundTasks[taskID] = task
	suite.server.mutex.Unlock()

	args := KillShellArguments{
		ShellID: taskID,
	}

	_, output, err := suite.server.KillShellHandler(context.Background(), &mcp.CallToolRequest{}, args)

	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), "Background task "+taskID+" killed successfully", output.Message)
	assert.Equal(suite.T(), taskID, output.ShellID)
}

// TestKillShellHandler_KillFailedTask 测试终止失败的任务
func (suite *KillShellHandlerTestSuite) TestKillShellHandler_KillFailedTask() {
	taskID := "test_failed_kill_12345"
	exitCode := 1
	task := &BackgroundTask{
		ID:        taskID,
		Command:   "false",
		Output:    "",
		Status:    "failed",
		StartTime: time.Now(),
		ExitCode:  &exitCode,
	}

	suite.server.mutex.Lock()
	suite.server.backgroundTasks[taskID] = task
	suite.server.mutex.Unlock()

	args := KillShellArguments{
		ShellID: taskID,
	}

	_, output, err := suite.server.KillShellHandler(context.Background(), &mcp.CallToolRequest{}, args)

	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), "Background task "+taskID+" killed successfully", output.Message)
	assert.Equal(suite.T(), taskID, output.ShellID)
}

// TestKillShellHandler_KillKilledTask 测试终止已终止的任务
func (suite *KillShellHandlerTestSuite) TestKillShellHandler_KillKilledTask() {
	taskID := "test_killed_kill_12345"
	task := &BackgroundTask{
		ID:        taskID,
		Command:   "echo killed",
		Output:    "",
		Status:    "killed",
		StartTime: time.Now(),
	}

	suite.server.mutex.Lock()
	suite.server.backgroundTasks[taskID] = task
	suite.server.mutex.Unlock()

	args := KillShellArguments{
		ShellID: taskID,
	}

	_, output, err := suite.server.KillShellHandler(context.Background(), &mcp.CallToolRequest{}, args)

	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), "Background task "+taskID+" killed successfully", output.Message)
	assert.Equal(suite.T(), taskID, output.ShellID)
}

// TestKillShellHandler_MixedStatusTasks 测试混合状态的任务
func (suite *KillShellHandlerTestSuite) TestKillShellHandler_MixedStatusTasks() {
	tasks := map[string]*BackgroundTask{
		"running_task":   {ID: "running_task", Command: "sleep 60", Status: "running", StartTime: time.Now()},
		"completed_task": {ID: "completed_task", Command: "echo done", Status: "completed", StartTime: time.Now()},
		"failed_task":    {ID: "failed_task", Command: "false", Status: "failed", StartTime: time.Now()},
		"killed_task":    {ID: "killed_task", Command: "echo killed", Status: "killed", StartTime: time.Now()},
	}

	suite.server.mutex.Lock()
	for _, task := range tasks {
		suite.server.backgroundTasks[task.ID] = task
	}
	suite.server.mutex.Unlock()

	for id := range tasks {
		args := KillShellArguments{
			ShellID: id,
		}

		_, output, err := suite.server.KillShellHandler(context.Background(), &mcp.CallToolRequest{}, args)

		require.NoError(suite.T(), err)
		assert.Equal(suite.T(), "Background task "+id+" killed successfully", output.Message)
		assert.Equal(suite.T(), id, output.ShellID)
	}

	suite.server.mutex.RLock()
	_, exists := suite.server.backgroundTasks["running_task"]
	suite.server.mutex.RUnlock()
	assert.False(suite.T(), exists, "运行任务应该被删除")
}

// TestKillShellHandler_ConcurrentKills 测试并发终止
func (suite *KillShellHandlerTestSuite) TestKillShellHandler_ConcurrentKills() {
	numTasks := 10
	var wg sync.WaitGroup

	for i := 0; i < numTasks; i++ {
		taskID := "test_concurrent_kill_" + string(rune('A'+i))

		// 为每个任务创建一个实际的进程
		ctx, cancel := context.WithCancel(context.Background())
		cmd := exec.CommandContext(ctx, "powershell", "-Command", "Start-Sleep -Seconds 30")
		cmd.Start()

		task := &BackgroundTask{
			ID:        taskID,
			Command:   "Start-Sleep -Seconds 30",
			Status:    "running",
			StartTime: time.Now(),
			Process:   cmd.Process,
			Cancel:    cancel,
		}

		suite.server.mutex.Lock()
		suite.server.backgroundTasks[taskID] = task
		suite.server.mutex.Unlock()

		wg.Add(1)
		go func(id string) {
			defer wg.Done()

			args := KillShellArguments{
				ShellID: id,
			}

			_, _, err := suite.server.KillShellHandler(context.Background(), &mcp.CallToolRequest{}, args)
			assert.NoError(suite.T(), err)
		}(taskID)
	}

	wg.Wait()

	suite.server.mutex.RLock()
	remaining := len(suite.server.backgroundTasks)
	suite.server.mutex.RUnlock()
	assert.Equal(suite.T(), 0, remaining, "所有任务都应该被删除")

	// 等待一下确保所有进程被终止
	time.Sleep(200 * time.Millisecond)
}

// TestKillShellHandler_SpecialCharactersInShellID 测试特殊字符的shell_id
func (suite *KillShellHandlerTestSuite) TestKillShellHandler_SpecialCharactersInShellID() {
	taskID := "test_special_123_!@#$%^&*()_task"
	task := &BackgroundTask{
		ID:        taskID,
		Command:   "echo special",
		Status:    "running",
		StartTime: time.Now(),
	}

	suite.server.mutex.Lock()
	suite.server.backgroundTasks[taskID] = task
	suite.server.mutex.Unlock()

	args := KillShellArguments{
		ShellID: taskID,
	}

	_, output, err := suite.server.KillShellHandler(context.Background(), &mcp.CallToolRequest{}, args)

	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), "Background task "+taskID+" killed successfully", output.Message)
	assert.Equal(suite.T(), taskID, output.ShellID)
}

// 运行KillShellHandler测试套件
func TestKillShellHandlerTestSuite(t *testing.T) {
	suite.Run(t, new(KillShellHandlerTestSuite))
}
