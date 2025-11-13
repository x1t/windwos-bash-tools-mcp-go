package core

import (
	"encoding/json"
	"os"
	"time"
)

// MCP JSON-RPC 基础结构
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type JSONRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   *JSONRPCError `json:"error,omitempty"`
}

type JSONRPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// MCP 协议特定结构
type InitializeParams struct {
	ProtocolVersion      string            `json:"protocolVersion"`
	Capabilities         ClientCapabilities `json:"capabilities"`
	ClientInfo           Info              `json:"clientInfo"`
	ProcessID            *int              `json:"processId,omitempty"`
	Root                 *Root             `json:"root,omitempty"`
	TraceLevel           string            `json:"traceLevel,omitempty"`
}

type ClientCapabilities struct {
	Sampling *map[string]interface{} `json:"sampling,omitempty"`
}

type Info struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type Root struct {
	URI       string `json:"uri"`
	Name      string `json:"name,omitempty"`
}

type InitializeResult struct {
	ProtocolVersion string            `json:"protocolVersion"`
	Capabilities    ServerCapabilities `json:"capabilities"`
	ServerInfo      Info              `json:"serverInfo"`
}

type ServerCapabilities struct {
	Tools *map[string]interface{} `json:"tools,omitempty"`
}

// 工具相关结构
type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema interface{} `json:"inputSchema"`
}

type CallToolParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// Agent工具结构
type AgentInput struct {
	Description string `json:"description" validate:"required,max=100"`
	Prompt      string `json:"prompt" validate:"required"`
	SubagentType string `json:"subagent_type" validate:"required,oneof=general-purpose statusline-setup Explore Plan"`
	Model       string `json:"model,omitempty" validate:"omitempty,oneof=sonnet opus haiku"`
	Resume      string `json:"resume,omitempty"`
}

// Bash工具结构
type BashInput struct {
	Command        string `json:"command" validate:"required"`
	Timeout        *int   `json:"timeout,omitempty" validate:"omitempty,min=1000,max=600000"`
	Description    string `json:"description,omitempty" validate:"omitempty,max=100"`
	RunInBackground bool  `json:"run_in_background" validate:"required"`
}

type BashOutputInput struct {
	BashID string `json:"bash_id" validate:"required"`
	Filter string `json:"filter,omitempty"`
}


type Content struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// 后台任务管理
type BackgroundTask struct {
	ID       string    `json:"id"`
	Command  string    `json:"command"`
	StartTime time.Time `json:"start_time"`
	Status   string    `json:"status"` // "running", "completed", "failed", "killed"
	Output   string    `json:"output"`
	ExitCode *int      `json:"exit_code,omitempty"`
	Error    string    `json:"error,omitempty"`
	Process  *os.Process `json:"-"` // 不序列化进程对象
}

// 配置结构
type Config struct {
	Server      ServerConfig      `mapstructure:"server"`
	Execution   ExecutionConfig   `mapstructure:"execution"`
	Security    SecurityConfig    `mapstructure:"security"`
	Logging     LoggingConfig     `mapstructure:"logging"`
}

type ServerConfig struct {
	Host         string `mapstructure:"host" default:"localhost"`
	Port         int    `mapstructure:"port" default:"8080"`
	MaxConns     int    `mapstructure:"max_conns" default:"100"`
	ReadTimeout  int    `mapstructure:"read_timeout" default:"30s"`
	WriteTimeout int    `mapstructure:"write_timeout" default:"30s"`
}

type ExecutionConfig struct {
	DefaultTimeout    int      `mapstructure:"default_timeout" default:"10000"` // 10秒
	MaxTimeout        int      `mapstructure:"max_timeout" default:"600000"`    // 10分钟
	MaxConcurrentJobs int      `mapstructure:"max_concurrent_jobs" default:"50"`
	AllowedCommands   []string `mapstructure:"allowed_commands"`
	BlockedCommands   []string `mapstructure:"blocked_commands"`
	WorkingDir        string   `mapstructure:"working_dir"`
}

type SecurityConfig struct {
	EnableValidation bool     `mapstructure:"enable_validation" default:"true"`
	AllowedPaths     []string `mapstructure:"allowed_paths"`
	BlockedPaths     []string `mapstructure:"blocked_paths"`
	MaxFileSize      int64    `mapstructure:"max_file_size" default:"104857600"` // 100MB
}

type LoggingConfig struct {
	Level      string `mapstructure:"level" default:"info"`
	Format     string `mapstructure:"format" default:"json"`
	Output     string `mapstructure:"output" default:"stdout"`
	MaxSize    int    `mapstructure:"max_size" default:"100"`
	MaxBackups int    `mapstructure:"max_backups" default:"3"`
	MaxAge     int    `mapstructure:"max_age" default:"28"`
}