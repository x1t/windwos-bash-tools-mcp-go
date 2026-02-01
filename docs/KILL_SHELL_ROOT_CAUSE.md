# Kill Shell 根本原因分析

## 测试结果

运行测试后发现：
```
taskkill failed: exit status 128
process.Kill() returned: invalid argument (may have already exited)
```

但是 **node.exe 进程仍在运行，端口 5173 仍在监听**。

## 根本原因

### 问题 1: PowerShell 命令的执行方式

当我们执行命令：
```powershell
cd K:\tailwind\react ; pnpm dev
```

PowerShell 的行为是：
1. 执行 `cd K:\tailwind\react` ✅
2. 执行 `pnpm dev` ✅
3. `pnpm` 启动 `node.exe` 作为**子进程** ✅
4. **PowerShell 命令立即返回**（因为 pnpm 已经启动了后台进程）❌
5. 我们的代码认为任务 "completed" ❌
6. `cmd.Process` 指向的 PowerShell 进程已经退出 ❌

### 问题 2: 进程句柄失效

```go
// 保存的是 PowerShell 进程
task.Process = cmd.Process  // PowerShell 进程

// 当 PowerShell 退出后
// task.Process 指向一个已经不存在的进程
// taskkill 失败: exit status 128 (进程不存在)
// process.Kill() 失败: invalid argument (进程已退出)
```

### 问题 3: 子进程成为孤儿进程

```
PowerShell (已退出)
  └─ pnpm (孤儿进程，继续运行)
      └─ node.exe (孤儿进程，继续运行)
          └─ vite (孤儿进程，继续运行) ← 端口 5173 仍在监听
```

## 为什么会这样？

### PowerShell 的进程管理

PowerShell 在执行命令时：
- 如果命令是**前台命令**（如 `Get-Process`），PowerShell 会等待完成
- 如果命令**启动了新进程**（如 `pnpm dev`），PowerShell 可能立即返回
- 特别是 `pnpm`、`npm`、`yarn` 这类包管理器，它们会：
  1. 启动一个守护进程
  2. 立即返回控制权给 PowerShell
  3. 守护进程继续运行

### 测试中的证据

```
状态: completed  ← PowerShell 命令已完成
端口 5173 正在监听  ← 但 Vite 服务器仍在运行
node.exe 进程仍存在  ← 子进程成为孤儿进程
```

## 解决方案

### 方案 1: 使用 Job Objects (Windows API)

Windows 提供了 Job Objects 机制，可以将进程组绑定在一起：

```go
// 创建 Job Object
job := CreateJobObject(nil, nil)

// 设置 Job 属性：终止 Job 时终止所有进程
info := JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
    BasicLimitInformation: JOBOBJECT_BASIC_LIMIT_INFORMATION{
        LimitFlags: JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
    },
}
SetInformationJobObject(job, JobObjectExtendedLimitInformation, &info)

// 将进程添加到 Job
AssignProcessToJobObject(job, cmd.Process.Handle)

// 终止时：关闭 Job，所有进程都会被终止
CloseHandle(job)
```

### 方案 2: 追踪子进程 PID

在启动命令时，获取所有子进程的 PID：

```powershell
# 启动命令并获取进程 ID
$process = Start-Process -FilePath "pnpm" -ArgumentList "dev" -WorkingDirectory "K:\tailwind\react" -PassThru
$processId = $process.Id

# 保存 PID，终止时使用
taskkill /F /T /PID $processId
```

### 方案 3: 修改命令执行方式

不使用 `cd ; pnpm dev`，而是：

```powershell
# 方式 1: 使用 Start-Process
Start-Process -FilePath "pnpm" -ArgumentList "dev" -WorkingDirectory "K:\tailwind\react" -NoNewWindow -Wait

# 方式 2: 使用 & 运算符
& { cd K:\tailwind\react ; pnpm dev }

# 方式 3: 使用 pwsh -Command
pwsh -Command "cd K:\tailwind\react ; pnpm dev"
```

### 方案 4: 定期扫描子进程

启动任务时，记录父进程 PID，定期扫描其子进程：

```go
// 启动时
task.ParentPID = cmd.Process.Pid

// 终止时
// 1. 获取所有子进程
children := GetChildProcesses(task.ParentPID)

// 2. 终止所有子进程
for _, childPID := range children {
    exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprintf("%d", childPID)).Run()
}
```

## 推荐方案

**方案 1 (Job Objects)** 是最可靠的，因为：
- ✅ Windows 原生支持
- ✅ 自动管理进程树
- ✅ 终止时保证所有子进程被清理
- ✅ 不需要定期扫描
- ✅ 性能开销小

但需要使用 Windows API，实现较复杂。

**方案 2 (追踪 PID)** 是最实用的，因为：
- ✅ 实现简单
- ✅ 可以立即实施
- ✅ 不需要 Windows API
- ⚠️ 需要修改命令执行方式

## 下一步

1. 实现 Job Objects 支持（长期方案）
2. 修改命令执行方式，使用 `Start-Process` 获取 PID（短期方案）
3. 添加子进程扫描功能（备用方案）
4. 更新文档和测试

## 相关资源

- [Windows Job Objects](https://docs.microsoft.com/en-us/windows/win32/procthread/job-objects)
- [Go syscall package](https://pkg.go.dev/syscall)
- [taskkill documentation](https://docs.microsoft.com/en-us/windows-server/administration/windows-commands/taskkill)
