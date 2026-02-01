//go:build !windows

package windows

import (
	"fmt"
	"os"
	"syscall"
)

// JobObject 表示一个 Windows Job Object（非 Windows 平台的存根）
type JobObject struct {
	handle syscall.Handle
}

// CreateJobObject 创建一个新的 Job Object（非 Windows 平台返回错误）
func CreateJobObject(name string) (*JobObject, error) {
	return nil, fmt.Errorf("Job Objects are only supported on Windows")
}

// AddProcess 将进程添加到 Job Object（非 Windows 平台返回错误）
func (j *JobObject) AddProcess(process *os.Process) error {
	return fmt.Errorf("Job Objects are only supported on Windows")
}

// Terminate 终止 Job Object 中的所有进程（非 Windows 平台返回错误）
func (j *JobObject) Terminate(exitCode uint32) error {
	return fmt.Errorf("Job Objects are only supported on Windows")
}

// Close 关闭 Job Object（非 Windows 平台返回错误）
func (j *JobObject) Close() error {
	return nil
}

// Handle 返回 Job Object 的句柄（非 Windows 平台返回 0）
func (j *JobObject) Handle() syscall.Handle {
	return 0
}
