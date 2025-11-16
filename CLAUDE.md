# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 项目概述

**MCP Bash Tools** - 专为Windows设计的MCP服务器，提供企业级安全的PowerShell/Bash命令执行能力。项目采用Go 1.23.0开发，具备六层安全防护体系和80%+的测试覆盖率。

## 常用命令

### 构建和运行

```powershell
# 构建（调试模式）
.\build.ps1

# 发布模式（优化二进制文件）
.\build.ps1 -Release

# 清理构建产物
.\build.ps1 -Clean

# 详细构建输出
.\build.ps1 -Verbose

# 运行服务器
.\dist\bash-tools.exe

# 验证Go环境
go version
go env
```

### 测试执行

```powershell
# 运行所有测试
go test ./...

# 运行特定测试文件
go test -v ./cmd/server/main_test.go
go test -v ./cmd/server/security_test.go
go test -v ./cmd/server/background_task_test.go
go test -v ./cmd/server/bash_output_test.go
go test -v ./cmd/server/kill_shell_test.go

# 运行特定包的测试
go test -v ./internal/security
go test -v ./internal/executor

# 生成覆盖率报告
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html

# 并发安全测试
go test -race ./...

# 跳过并发安全检查（快速测试）
go test ./... -skip=TestKillShellHandlerTestSuite -skip=TestBashHandlerTestSuite

# 运行单个测试
go test -v -run TestSpecificTestName ./...

# 性能基准测试
go test -bench=. -benchmem ./...

# 超时设置（长时间测试）
go test -timeout 10m ./...
```

## 核心架构设计

### 三层架构模式

```
┌─────────────────────────────────────┐
│          MCP接口层 (cmd/server)        │  ← 629行
│  • 工具注册与JSON-RPC通信             │
│  • 参数验证与错误处理                 │
└─────────────────────────────────────┘
                    ↓
┌─────────────────────────────────────┐
│        业务逻辑层 (internal)          │
│  • executor/  - Shell执行器 (3个)     │
│  • security/  - 安全验证 (1061行)    │
│  • core/      - 类型定义             │
└─────────────────────────────────────┘
                    ↓
┌─────────────────────────────────────┐
│      基础设施层 (pkg)                 │
│  • logger/  - 日志系统                │
│  • utils/   - 工具函数                │
└─────────────────────────────────────┘
```

### 核心模块职责

#### cmd/server/main.go (629行) - MCP服务器核心
- **MCP协议实现**: 工具注册、JSON-RPC通信
- **三个核心工具**:
  - `bash` - 命令执行（前台/后台）
  - `bash_output` - 获取后台任务实时输出
  - `kill_shell` - 终止后台任务
- **后台任务管理**: 使用`sync.RWMutex`保证并发安全
- **临时文件机制**: 实时存储后台任务输出

#### internal/executor/ - Shell执行器系统 (983行)
- **shell.go** (185行): 智能Shell检测，优先级：PowerShell 7 → Git Bash → PowerShell 5+ → CMD
- **bash.go** (240行): 基础命令执行，支持超时控制和进程管理
- **secure_bash.go** (558行): 企业级安全执行器
  - 沙箱隔离环境
  - 资源限制（CPU 50%、内存256MB、磁盘100MB）
  - 实时资源监控

#### internal/security/ - 安全防护体系 (1061行)
- **security.go** (561行):
  - JWT令牌认证
  - 基于角色的访问控制 (RBAC)
  - Token Bucket速率限制（默认10 RPS，突发20）
  - 结构化审计日志
- **validator.go** (213行): 命令安全验证
  - 70+危险模式识别（系统破坏、控制、权限提升等）
  - 引号内容检查
  - 命令长度限制（默认10000字符）

## MCP工具接口

### Bash工具 (cmd/server/main.go:108-189)
执行PowerShell/CMD命令，支持前台和后台模式。

**参数**:
- `command` (string, 必填): 要执行的命令
- `timeout` (int, 可选): 超时时间(毫秒)，范围1000-600000，默认30000
- `description` (string, 可选): 命令描述
- `run_in_background` (bool, 必填): 是否后台执行

**返回**:
- `output` (string): 命令输出
- `exitCode` (int): 退出代码
- `killed` (bool): 是否被强制终止
- `shellId` (string): 后台任务ID（仅后台模式）

### BashOutput工具 (cmd/server/main.go:191-255)
获取后台任务的实时输出内容，支持正则过滤。

**参数**:
- `bash_id` (string, 必填): 后台任务ID
- `filter` (string, 可选): 正则表达式过滤器

**返回**:
- `output` (string): 输出内容（过滤后）
- `status` (string): 任务状态（running, completed, failed, killed）
- `exitCode` (int, 可选): 退出代码

### KillShell工具 (cmd/server/main.go:257-287)
终止后台任务，释放系统资源。

**参数**:
- `shell_id` (string, 必填): 要终止的任务ID

**返回**:
- `message` (string): 操作结果消息
- `shell_id` (string): 被终止的任务ID

## 安全机制

### 六层安全防护体系

1. **认证授权**: JWT + RBAC权限控制
2. **速率限制**: Token Bucket算法，10 RPS
3. **输入验证**: 危险模式识别，70+种模式
4. **执行隔离**: 可选沙箱环境
5. **资源限制**: CPU/内存/磁盘/文件句柄限制
6. **审计日志**: 结构化安全事件记录

### 危险命令分类 (internal/security/validator.go:15-95)
- **系统破坏**: `rm -rf /`, `format`, `dd if=/dev/zero`
- **系统控制**: `shutdown`, `reboot`, `halt`
- **权限提升**: `sudo su`, `passwd`, `chmod 777`
- **网络攻击**: `iptables -F`, fork bomb
- **磁盘操作**: `fdisk`, `mkfs`, `dd`

## 后台任务机制

**BackgroundTask结构** (cmd/server/main.go:64-74):
```go
type BackgroundTask struct {
    ID        string    `json:"id"`
    Command   string    `json:"command"`
    Output    string    `json:"output"`
    Status    string    `json:"status"` // running, completed, failed, killed
    StartTime time.Time `json:"startTime"`
    Error     string    `json:"error,omitempty"`
    ExitCode  *int      `json:"exitCode,omitempty"`
    TempFile  string    `json:"tempFile,omitempty"` // 实时输出存储
}
```

**并发安全**: 使用`sync.RWMutex`保护后台任务存储，读锁查询，写锁更新。

## 项目依赖

**go.mod**:
- `github.com/modelcontextprotocol/go-sdk v1.1.0`: MCP官方SDK
- `github.com/sirupsen/logrus v1.9.3`: 结构化日志
- `github.com/stretchr/testify v1.11.1`: 测试框架

间接依赖包括：google/jsonschema-go、golang.org/x/oauth2、gopkg.in/yaml.v3等。

## 测试策略

**测试金字塔**: 70%单元测试 + 20%集成测试 + 10%E2E测试

**测试文件** (共2409行):
1. `main_test.go` (291行): 核心功能测试，使用MockShellExecutor
2. `security_test.go` (482行): 70+危险命令安全测试
3. `background_task_test.go` (566行): 任务管理与并发安全测试
4. `bash_output_test.go` (399行): 输出获取与正则过滤测试
5. `kill_shell_test.go` (435行): 任务终止与资源清理测试
6. `validator_test.go` (636行): 命令验证逻辑测试

**测试模式**:
- 表驱动测试 (Table-Driven Tests)
- Mock对象隔离依赖
- 并发安全测试 (`go test -race`)
- 断言库使用 `testify/assert` 和 `testify/suite`

## 构建配置

**build.ps1** (233行): PowerShell构建脚本，支持：
- 调试/发布模式切换
- GOOS/GOARCH环境变量设置
- CGO_ENABLED配置
- 构建产物优化和压缩

**目标平台**: Windows x64 (`GOOS=windows GOARCH=amd64 CGO_ENABLED=0`)

## 开发指南

### 添加新功能流程
1. 在`cmd/server/main.go`定义工具输入输出类型（遵循BashArguments命名规范）
2. 在`internal/executor/`实现ShellExecutorInterface接口
3. 在main函数中注册新工具（参考BashHandler实现）
4. 编写对应测试文件（表驱动模式+Mock）
5. 运行`go test -v`验证功能

### 并发安全要求
- 后台任务管理必须使用`sync.RWMutex`
- 共享资源访问需要锁保护
- 锁升级降级优化（读锁→写锁→读锁）

### 安全检查清单
- 新增命令需通过`validator.ValidateCommand()`验证
- 危险模式检查（参考validator.go:15-95）
- 超时控制（默认30秒，范围1-600秒）
- 资源限制（CPU/内存/磁盘）
- 审计日志记录

### 代码规范
- 文件行数控制: 200-500行/文件
- 包结构按职责分离
- 接口抽象（ShellExecutorInterface）
- 依赖倒置：面向接口编程
- 错误处理：多层检查+详细错误信息

### 项目依赖管理
```powershell
# 更新依赖
go get -u ./...
go mod tidy

# 检查依赖漏洞
go list -m -u all

# 下载依赖
go mod download

# 验证依赖
go mod verify
```

### 开发工具推荐
- **IDE**: VS Code + Go扩展
- **调试**: Delve debugger (`dlv debug`)
- **格式化**: `go fmt ./...`
- **静态分析**: `go vet ./...`
- **代码检查**: golangci-lint

### 提交前检查清单
```powershell
# 1. 格式化代码
go fmt ./...

# 2. 静态分析
go vet ./...

# 3. 运行测试（跳过已知问题）
go test ./... -timeout 5m

# 4. 并发安全检查
go test -race ./internal/security ./internal/executor

# 5. 构建验证
.\build.ps1 -Release
```

## 故障排除

### 常见问题
1. **后台任务输出为空**: 检查`TempFile`路径和goroutine写入逻辑
2. **并发安全问题**: 验证锁的使用（读锁查询/写锁更新）
3. **安全验证误报**: 检查validator.go中的危险模式规则
4. **测试失败**: 清理临时文件，检查Mock对象初始化
5. **并发任务ID重复**: 检查时间戳生成逻辑，确保纳秒级精度
6. **KillShell测试失败**: 验证错误处理逻辑和任务状态检查

### 当前已知测试问题
- 部分并发测试存在任务ID冲突问题
- KillShell错误处理测试需要更新
- 安全验证边界情况需要改进

### 调试命令
```powershell
# 详细测试输出
go test -v -run TestBashHandler

# 竞争检测
go test -race ./...

# 性能分析
go test -cpuprofile=cpu.prof -memprofile=mem.prof ./...

# 构建详细输出
.\build.ps1 -Verbose

# 跳过已知问题的测试
go test -v ./... -skip="TestKillShellHandlerTestSuite" -skip="TestBashHandler_ConcurrentBackgroundTasks"

# 运行单个测试套件
go test -v ./cmd/server -run TestSecurityTestSuite

# 清理临时测试文件
Get-ChildItem -Path . -Recurse -Name "temp_*" -File | Remove-Item
```

### 开发环境检查
```powershell
# 检查Go版本和模块
go version
go mod verify
go mod tidy

# 检查PowerShell执行策略
Get-ExecutionPolicy
if ((Get-ExecutionPolicy) -eq "Restricted") {
    Set-ExecutionPolicy -ExecutionPolicy RemoteSigned -Scope CurrentUser
}

# 检查文件编码（避免中文乱码）
Get-Content build.ps1 -Encoding UTF8 | Set-Content build.ps1 -Encoding UTF8
```
