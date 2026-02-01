//go:build windows

// Windows Job Object 实现 - 用于管理进程树终止
package windows

import (
	"fmt"
	"os"
	"reflect"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// 常量定义
const (
	sizeofJobobjectExtendedLimitInformation = 144 // binary.size cannot handle uintptr
)

// JobObject 表示一个 Windows Job Object
type JobObject struct {
	handle windows.Handle
	name   string
}

// CreateJobObject 创建一个新的 Job Object
// 设置 JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE 标志，确保 Job 关闭时自动终止所有子进程
func CreateJobObject(name string) (*JobObject, error) {
	var namePtr *uint16
	var err error

	if name != "" {
		namePtr, err = syscall.UTF16PtrFromString(name)
		if err != nil {
			return nil, fmt.Errorf("failed to convert job name: %w", err)
		}
	}

	handle, err := windows.CreateJobObject(nil, namePtr)
	if err != nil {
		return nil, fmt.Errorf("failed to create job object: %w", err)
	}

	// 设置 Job Object 限制：当 Job 关闭时终止所有进程
	extendedInfo := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}

	_, err = windows.SetInformationJobObject(
		handle,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&extendedInfo)),
		sizeofJobobjectExtendedLimitInformation,
	)
	if err != nil {
		windows.CloseHandle(handle)
		return nil, fmt.Errorf("failed to set job object information: %w", err)
	}

	return &JobObject{
		handle: handle,
		name:   name,
	}, nil
}

// AddProcess 将进程添加到 Job Object
// 添加后，该进程及其所有子进程都将被 Job Object 管理
func (j *JobObject) AddProcess(process *os.Process) error {
	if j.handle == 0 {
		return fmt.Errorf("job object handle is invalid")
	}
	if process == nil {
		return fmt.Errorf("process is nil")
	}

	// 通过反射获取 os.Process 内部的 handle 字段
	fv := reflect.ValueOf(process).Elem().FieldByName("handle")
	if !fv.IsValid() {
		return fmt.Errorf("failed to get process handle via reflection")
	}

	var processHandle windows.Handle

	// 根据字段类型获取句柄值
	// 注意: os.Process 的 handle 字段类型在不同 Go 版本可能不同（uintptr 或 pointer）
	// fmt.Fprintf(os.Stderr, "Debug: handle field type: %s, kind: %s\n", fv.Type(), fv.Kind())

	switch fv.Kind() {
	case reflect.Uintptr, reflect.Uint, reflect.Uint64:
		processHandle = windows.Handle(fv.Uint())
	case reflect.Ptr:
		// 如果是指针，这可能是一个指向实际句柄的指针
		// 在 Go 1.23+ on Windows, os.Process handle is *os.processHandle
		ptrElem := fv.Elem()
		if ptrElem.Kind() == reflect.Uintptr {
			processHandle = windows.Handle(ptrElem.Uint())
		} else if ptrElem.Kind() == reflect.Struct && ptrElem.NumField() > 0 {
			// 如果是结构体 (type processHandle struct { handle uintptr }), 尝试获取第一个字段
			f0 := ptrElem.Field(0)
			if f0.Kind() == reflect.Uintptr {
				processHandle = windows.Handle(f0.Uint())
			} else {
				// Fallback: 尝试直接使用指针值 (旧行为，但通常不正确)
				ptr := fv.Pointer()
				processHandle = windows.Handle(ptr)
				fmt.Fprintf(os.Stderr, "Warning: handle field is pointer to struct/unknown, using pointer addr: %x\n", ptr)
			}
		} else {
			// Fallback
			ptr := fv.Pointer()
			processHandle = windows.Handle(ptr)
			fmt.Fprintf(os.Stderr, "Warning: handle field is pointer (type: %s), using value: %x\n", fv.Type(), ptr)
		}
	default:
		return fmt.Errorf("unsupported handle field type: %s", fv.Kind())
	}

	return windows.AssignProcessToJobObject(j.handle, processHandle)
}

// Terminate 终止 Job Object 中的所有进程
// 这是终止进程树的关键方法
func (j *JobObject) Terminate(exitCode uint32) error {
	if j.handle == 0 {
		return fmt.Errorf("job object handle is invalid")
	}

	// 使用 golang.org/x/sys/windows 的 TerminateJobObject API
	return windows.TerminateJobObject(j.handle, exitCode)
}

// Close 关闭 Job Object
// 如果设置了 JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE 标志，关闭时会自动终止所有进程
func (j *JobObject) Close() error {
	if j.handle == 0 {
		return nil
	}
	err := windows.CloseHandle(j.handle)
	j.handle = 0
	return err
}

// Handle 返回 Job Object 的句柄
func (j *JobObject) Handle() syscall.Handle {
	return syscall.Handle(j.handle)
}
