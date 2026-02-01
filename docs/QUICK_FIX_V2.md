# Kill Shell 快速修复方案 V2

## 问题

当前的 `kill_shell` 无法终止通过 `pnpm dev` 等命令启动的子进程，因为：
1. PowerShell 命令立即返回
2. 我们保存的进程句柄指向已退出的 PowerShell 进程
3. 真正的子进程（node.exe）成为孤儿进程继续运行

## 解决方案

### 方案 A: 使用 PowerShell 脚本包装

修改命令执行方式，使用一个包装脚本来保持进程活跃：

```powershell
# wrapper.ps1
param($Command, $WorkingDir)

if ($WorkingDir) {
    Set-Location $WorkingDir
}

# 启动进程并等待
$process = Start-Process -FilePath "powershell" -ArgumentList "-Command", $Command -NoNewWindow -PassThru
$process.WaitForExit()
```

### 方案 B: 记录子进程 PID

在启动命令时，使用 PowerShell 获取子进程 PID：

```powershell
# 启动命令
cd K:\tailwind\react
$process = Start-Process -FilePath "pnpm" -ArgumentList "dev" -NoNewWindow -PassThru
Write-Output "PROCESS_ID:$($process.Id)"
Wait-Process -Id $process.Id
```

然后解析输出获取 PID，保存到 task 中。

### 方案 C: 使用 Windows Job Objects (推荐)

这是最可靠的方案，需要使用 Windows API：

```go
import (
    "syscall"
    "unsafe"
)

// 创建 Job Object
func createJobObject() (syscall.Handle, error) {
    job, err := syscall.CreateJobObject(nil, nil)
    if err != nil {
        return 0, err
    }
    
    // 设置 Job 属性：关闭 Job 时终止所有进程
    info := JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
        BasicLimitInformation: JOBOBJECT_BASIC_LIMIT_INFORMATION{
            LimitFlags: JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
        },
    }
    
    _, err = syscall.SetInformationJobObject(
        job,
        JobObjectExtendedLimitInformation,
        uintptr(unsafe.Pointer(&info)),
        uint32(unsafe.Sizeof(info)),
    )
    
    return job, err
}

// 将进程添加到 Job
func assignProcessToJob(job syscall.Handle, process *os.Process) error {
    return syscall.AssignProcessToJobObject(job, syscall.Handle(process.Pid))
}

// 终止 Job（会终止所有子进程）
func terminateJob(job syscall.Handle) error {
    return syscall.CloseHandle(job)
}
```

## 实现步骤

### 短期方案（方案 B）

1. 修改 `executeBackgroundCommand` 函数
2. 解析命令，提取工作目录和实际命令
3. 使用 `Start-Process` 启动并获取 PID
4. 保存 PID 到 task
5. 终止时使用 `taskkill /F /T /PID`

### 长期方案（方案 C）

1. 创建 `internal/windows/job.go`
2. 实现 Job Objects 相关函数
3. 修改 `executeBackgroundCommand` 使用 Job
4. 修改 `KillShellHandler` 终止 Job
5. 添加测试

## 代码示例

### 方案 B 实现

```go
// 解析命令
func parseCommand(command string) (workingDir string, actualCommand string) {
    // 简单解析 "cd path ; command" 格式
    parts := strings.Split(command, ";")
    if len(parts) >= 2 {
        cdPart := strings.TrimSpace(parts[0])
        if strings.HasPrefix(strings.ToLower(cdPart), "cd ") {
            workingDir = strings.TrimSpace(cdPart[3:])
            actualCommand = strings.TrimSpace(strings.Join(parts[1:], ";"))
            return
        }
    }
    actualCommand = command
    return
}

// 启动进程并获取 PID
func startProcessWithPID(workingDir, command string) (int, *exec.Cmd, error) {
    script := fmt.Sprintf(`
        if ('%s' -ne '') { Set-Location '%s' }
        $process = Start-Process -FilePath 'powershell' -ArgumentList '-Command', '%s' -NoNewWindow -PassThru
        Write-Output "PID:$($process.Id)"
        Wait-Process -Id $process.Id
    `, workingDir, workingDir, command)
    
    cmd := exec.Command("powershell", "-Command", script)
    
    // 启动并读取输出
    output, err := cmd.StdoutPipe()
    if err != nil {
        return 0, nil, err
    }
    
    if err := cmd.Start(); err != nil {
        return 0, nil, err
    }
    
    // 读取 PID
    scanner := bufio.NewScanner(output)
    var pid int
    for scanner.Scan() {
        line := scanner.Text()
        if strings.HasPrefix(line, "PID:") {
            fmt.Sscanf(line, "PID:%d", &pid)
            break
        }
    }
    
    return pid, cmd, nil
}

// 终止进程树
func killProcessTree(pid int) error {
    cmd := exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprintf("%d", pid))
    return cmd.Run()
}
```

## 测试

```bash
# 1. 启动后台任务
bash tool: command="cd K:\tailwind\react ; pnpm dev", run_in_background=true

# 2. 检查进程
tasklist | findstr node

# 3. 终止任务
kill_shell tool: shell_id="<task_id>"

# 4. 验证进程已终止
tasklist | findstr node  # 应该没有输出
netstat -ano | findstr :5173  # 应该没有输出
```

## 优先级

1. **立即**: 文档化当前问题和限制
2. **本周**: 实现方案 B（PID 追踪）
3. **下周**: 实现方案 C（Job Objects）
4. **持续**: 改进测试和文档
