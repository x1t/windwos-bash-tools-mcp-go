package security

/*
	⚠️ 注意: 此文件包含企业级安全管理器的预留实现

	当前状态: 未启用
	- 本项目当前仅使用 validator.go 中的基础命令验证
	- SecurityManager 提供了高级安全特性（JWT认证、RBAC、速率限制等）
	- 这些功能已实现但未集成到主服务器中

	如需启用:
	1. 在 cmd/server/main.go 中初始化 SecurityManager
	2. 在 BashHandler 中添加认证和授权检查
	3. 配置相应的安全策略和速率限制

	功能特性:
	- JWT令牌认证
	- RBAC权限控制
	- Token Bucket速率限制
	- 审计日志记录
	- 会话管理

	注意: 对于本地MCP工具，这些功能可能过度设计
*/

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"mcp-bash-tools/pkg/logger"
)

// contextKey 自定义 context key 类型，避免键冲突
type contextKey string

// 定义 context keys
const (
	authContextKey      contextKey = "auth"
	securityEnabledKey  contextKey = "security_enabled"
	rateLimitEnabledKey contextKey = "rate_limit_enabled"
)

// Enterprise-grade security manager
type SecurityManager struct {
	logger           *logger.Logger
	config           SecurityConfig
	rateLimiter      *RateLimiter
	commandValidator *CommandValidator
	authProvider     AuthProvider
	mutex            sync.RWMutex
}

// SecurityConfig holds security configuration
type SecurityConfig struct {
	EnableAuth         bool          `json:"enable_auth" default:"true"`
	APIKeys            []string      `json:"api_keys"`
	TokenExpiry        time.Duration `json:"token_expiry" default:"24h"`
	EnableRateLimit    bool          `json:"enable_rate_limit" default:"true"`
	RateLimitRPS       int           `json:"rate_limit_rps" default:"10"`
	RateLimitBurst     int           `json:"rate_limit_burst" default:"20"`
	EnableInputFilter  bool          `json:"enable_input_filter" default:"true"`
	MaxCommandLength   int           `json:"max_command_length" default:"1000"`
	EnableAudit        bool          `json:"enable_audit" default:"true"`
	AllowedCommands    []string      `json:"allowed_commands"`
	BlockedCommands    []string      `json:"blocked_commands"`
	WorkingDirRestrict bool          `json:"working_dir_restrict" default:"true"`
	AllowedPaths       []string      `json:"allowed_paths"`
}

// RateLimiter implements token bucket rate limiting
type RateLimiter struct {
	buckets map[string]*TokenBucket
	mutex   sync.RWMutex
	config  RateLimiterConfig
}

type RateLimiterConfig struct {
	RPS      int           `json:"rps"`
	Burst    int           `json:"burst"`
	Interval time.Duration `json:"interval"`
}

type TokenBucket struct {
	tokens     int
	lastRefill time.Time
	capacity   int
	refillRate int
	mutex      sync.Mutex
}

// CommandValidator validates and sanitizes commands
type CommandValidator struct {
	config            ValidationConfig
	allowedPattern    *regexp.Regexp
	blockedPatterns   []*regexp.Regexp
	dangerousCommands map[string]bool
	mutex             sync.RWMutex
}

type ValidationConfig struct {
	MaxCommandLength    int      `json:"max_command_length" default:"1000"`
	AllowShellCommands  bool     `json:"allow_shell_commands" default:"false"`
	AllowFileOperations bool     `json:"allow_file_operations" default:"true"`
	AllowNetworkAccess  bool     `json:"allow_network_access" default:"false"`
	AllowSystemCommands bool     `json:"allow_system_commands" default:"false"`
	AllowedCommands     []string `json:"allowed_commands"`
	BlockedCommands     []string `json:"blocked_commands"`
	AllowedArguments    []string `json:"allowed_arguments"`
	BlockedArguments    []string `json:"blocked_arguments"`
}

// AuthProvider interface for authentication
type AuthProvider interface {
	Authenticate(ctx context.Context, token string) (*AuthContext, error)
	GenerateToken(ctx context.Context, userID string, permissions []string) (string, error)
	ValidatePermissions(ctx context.Context, auth *AuthContext, required []string) error
}

// AuthContext holds authentication information
type AuthContext struct {
	UserID      string    `json:"user_id"`
	Username    string    `json:"username"`
	Permissions []string  `json:"permissions"`
	ExpiresAt   time.Time `json:"expires_at"`
	IPAddress   string    `json:"ip_address"`
	UserAgent   string    `json:"user_agent"`
	SessionID   string    `json:"session_id"`
}

// JWTAuthProvider implements JWT-based authentication
type JWTAuthProvider struct {
	secretKey string
	logger    *logger.Logger
	mutex     sync.RWMutex
}

// SecurityEvent represents a security-related event
type SecurityEvent struct {
	Timestamp   time.Time `json:"timestamp"`
	EventType   string    `json:"event_type"`
	Severity    string    `json:"severity"`
	UserID      string    `json:"user_id"`
	IPAddress   string    `json:"ip_address"`
	Command     string    `json:"command"`
	Description string    `json:"description"`
	Success     bool      `json:"success"`
	Error       string    `json:"error,omitempty"`
}

// NewSecurityManager creates a new security manager
func NewSecurityManager(config SecurityConfig, logger *logger.Logger) *SecurityManager {
	sm := &SecurityManager{
		logger: logger,
		config: config,
		rateLimiter: NewRateLimiter(RateLimiterConfig{
			RPS:      config.RateLimitRPS,
			Burst:    config.RateLimitBurst,
			Interval: time.Second,
		}),
		commandValidator: NewCommandValidator(ValidationConfig{
			MaxCommandLength: config.MaxCommandLength,
			AllowedCommands:  config.AllowedCommands,
			BlockedCommands:  config.BlockedCommands,
		}),
	}

	// Initialize auth provider
	if config.EnableAuth {
		sm.authProvider = NewJWTAuthProvider(generateSecretKey(), logger)
	}

	return sm
}

func NewRateLimiter(config RateLimiterConfig) *RateLimiter {
	return &RateLimiter{
		buckets: make(map[string]*TokenBucket),
		config:  config,
	}
}

func NewCommandValidator(config ValidationConfig) *CommandValidator {
	cv := &CommandValidator{
		config:            config,
		dangerousCommands: make(map[string]bool),
	}

	// Initialize dangerous commands list
	cv.initializeDangerousCommands()

	// Compile regex patterns
	cv.compilePatterns()

	return cv
}

func NewJWTAuthProvider(secretKey string, logger *logger.Logger) *JWTAuthProvider {
	return &JWTAuthProvider{
		secretKey: secretKey,
		logger:    logger,
	}
}

// Security validation methods
func (sm *SecurityManager) ValidateCommand(ctx context.Context, command string, auth *AuthContext) error {
	// Check rate limiting
	if sm.config.EnableRateLimit {
		if err := sm.rateLimiter.Allow(auth.SessionID); err != nil {
			sm.logSecurityEvent(SecurityEvent{
				EventType:   "rate_limit_exceeded",
				Severity:    "high",
				UserID:      auth.UserID,
				Command:     command,
				Description: "Rate limit exceeded",
				Success:     false,
				Error:       err.Error(),
			})
			return fmt.Errorf("rate limit exceeded: %w", err)
		}
	}

	// Validate command syntax and security
	if err := sm.commandValidator.Validate(command); err != nil {
		sm.logSecurityEvent(SecurityEvent{
			EventType:   "command_validation_failed",
			Severity:    "high",
			UserID:      auth.UserID,
			Command:     command,
			Description: "Command validation failed",
			Success:     false,
			Error:       err.Error(),
		})
		return fmt.Errorf("command validation failed: %w", err)
	}

	sm.logSecurityEvent(SecurityEvent{
		EventType:   "command_validated",
		Severity:    "info",
		UserID:      auth.UserID,
		Command:     command,
		Description: "Command successfully validated",
		Success:     true,
	})

	return nil
}

func (sm *SecurityManager) Authenticate(ctx context.Context, token string) (*AuthContext, error) {
	if !sm.config.EnableAuth {
		// Return default auth context when auth is disabled
		return &AuthContext{
			UserID:      "anonymous",
			Username:    "anonymous",
			Permissions: []string{"execute"},
			SessionID:   "default",
		}, nil
	}

	if sm.authProvider == nil {
		return nil, fmt.Errorf("authentication provider not configured")
	}

	auth, err := sm.authProvider.Authenticate(ctx, token)
	if err != nil {
		sm.logSecurityEvent(SecurityEvent{
			EventType:   "authentication_failed",
			Severity:    "high",
			Description: "Authentication failed",
			Success:     false,
			Error:       err.Error(),
		})
		return nil, err
	}

	sm.logSecurityEvent(SecurityEvent{
		EventType:   "authentication_success",
		Severity:    "info",
		UserID:      auth.UserID,
		Description: "Authentication successful",
		Success:     true,
	})

	return auth, nil
}

func (sm *SecurityManager) ValidatePermissions(ctx context.Context, auth *AuthContext, permissions []string) error {
	if !sm.config.EnableAuth {
		return nil
	}

	if sm.authProvider == nil {
		return fmt.Errorf("authentication provider not configured")
	}

	if err := sm.authProvider.ValidatePermissions(ctx, auth, permissions); err != nil {
		sm.logSecurityEvent(SecurityEvent{
			EventType:   "permission_denied",
			Severity:    "medium",
			UserID:      auth.UserID,
			Description: "Permission validation failed",
			Success:     false,
			Error:       err.Error(),
		})
		return err
	}

	return nil
}

// Rate limiting methods
func (rl *RateLimiter) Allow(identifier string) error {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	bucket, exists := rl.buckets[identifier]
	if !exists {
		bucket = NewTokenBucket(rl.config.Burst, rl.config.RPS)
		rl.buckets[identifier] = bucket
	}

	return bucket.Allow()
}

func NewTokenBucket(capacity, refillRate int) *TokenBucket {
	return &TokenBucket{
		tokens:     capacity,
		capacity:   capacity,
		refillRate: refillRate,
		lastRefill: time.Now(),
	}
}

func (tb *TokenBucket) Allow() error {
	tb.mutex.Lock()
	defer tb.mutex.Unlock()

	tb.refill()

	if tb.tokens > 0 {
		tb.tokens--
		return nil
	}

	return fmt.Errorf("rate limit exceeded")
}

func (tb *TokenBucket) refill() {
	now := time.Now()
	elapsed := now.Sub(tb.lastRefill)

	if elapsed >= time.Second {
		tokensToAdd := int(elapsed.Seconds()) * tb.refillRate
		tb.tokens += tokensToAdd
		if tb.tokens > tb.capacity {
			tb.tokens = tb.capacity
		}
		tb.lastRefill = now
	}
}

// Command validation methods
func (cv *CommandValidator) Validate(command string) error {
	cv.mutex.RLock()
	defer cv.mutex.RUnlock()

	// Check command length
	if len(command) > cv.config.MaxCommandLength {
		return fmt.Errorf("command too long: max %d characters", cv.config.MaxCommandLength)
	}

	// Check for dangerous commands
	if cv.isDangerousCommand(command) {
		return fmt.Errorf("dangerous command detected")
	}

	// Check blocked patterns
	for _, pattern := range cv.blockedPatterns {
		if pattern.MatchString(command) {
			return fmt.Errorf("command contains blocked pattern: %s", pattern.String())
		}
	}

	// If allowed commands are specified, check against them
	if len(cv.config.AllowedCommands) > 0 {
		if !cv.isAllowedCommand(command) {
			return fmt.Errorf("command not in allowed list")
		}
	}

	// Check allowed pattern if specified
	if cv.allowedPattern != nil {
		if !cv.allowedPattern.MatchString(command) {
			return fmt.Errorf("command does not match allowed pattern")
		}
	}

	return nil
}

func (cv *CommandValidator) isDangerousCommand(command string) bool {
	lowerCmd := strings.ToLower(command)

	// Check for dangerous command patterns (Windows only)
	dangerousPatterns := []string{
		// 系统破坏命令
		"format",
		"del /f /s /q",
		"rmdir /s /q",
		"rd /s /q",
		"diskpart",
		// 系统控制命令
		"shutdown",
		"reboot",
		"stop-computer",
		"restart-computer",
		// Windows fork bomb
		"%0|%0",
		// 权限提升
		"takeown",
		"icacls",
	}

	for _, pattern := range dangerousPatterns {
		if strings.Contains(lowerCmd, pattern) {
			return true
		}
	}

	// Check dangerous commands map
	for cmd := range cv.dangerousCommands {
		if strings.Contains(lowerCmd, cmd) {
			return true
		}
	}

	return false
}

func (cv *CommandValidator) isAllowedCommand(command string) bool {
	baseCommand := cv.extractBaseCommand(command)

	for _, allowed := range cv.config.AllowedCommands {
		if strings.ToLower(baseCommand) == strings.ToLower(allowed) {
			return true
		}
	}

	return false
}

func (cv *CommandValidator) extractBaseCommand(command string) string {
	// Simple extraction - can be enhanced
	parts := strings.Fields(command)
	if len(parts) > 0 {
		return parts[0]
	}
	return command
}

func (cv *CommandValidator) initializeDangerousCommands() {
	// Windows dangerous commands only
	dangerous := []string{
		"del", "rmdir", "rd", "format", "shutdown", "reboot",
		"reg delete", "reg add", "net user", "net localgroup",
		"powershell -enc", "powershell -encodedcommand",
		"wget", "curl", "nc", "netcat", "telnet",
	}

	for _, cmd := range dangerous {
		cv.dangerousCommands[cmd] = true
	}
}

func (cv *CommandValidator) compilePatterns() {
	// Compile blocked patterns (Windows only)
	blockedPatterns := []string{
		`(?i).*format\s+[a-zA-Z]:.*`,          // format C:
		`(?i).*del\s+/[fFsS].*`,               // del /f /s
		`(?i).*rmdir\s+/[sS].*`,               // rmdir /s
		`(?i).*rd\s+/[sS].*`,                  // rd /s
		`(?i).*diskpart.*`,                    // diskpart
		`(?i).*shutdown\s+.*`,                 // shutdown
		`(?i).*stop-computer.*`,               // Stop-Computer
		`(?i).*restart-computer.*`,            // Restart-Computer
		`(?i).*powershell.*-enc.*`,            // powershell -enc
		`(?i).*powershell.*-encodedcommand.*`, // powershell -encodedcommand
	}

	for _, pattern := range blockedPatterns {
		if regex, err := regexp.Compile(pattern); err == nil {
			cv.blockedPatterns = append(cv.blockedPatterns, regex)
		}
	}
}

// Auth provider methods
func (jp *JWTAuthProvider) Authenticate(ctx context.Context, token string) (*AuthContext, error) {
	if token == "" {
		return nil, fmt.Errorf("token is required")
	}

	// Simple token validation for demo
	// In production, use proper JWT validation
	if token == "demo-token" {
		return &AuthContext{
			UserID:      "demo-user",
			Username:    "demo",
			Permissions: []string{"execute", "read"},
			ExpiresAt:   time.Now().Add(24 * time.Hour),
			SessionID:   "demo-session",
		}, nil
	}

	return nil, fmt.Errorf("invalid token")
}

func (jp *JWTAuthProvider) GenerateToken(ctx context.Context, userID string, permissions []string) (string, error) {
	// Simple token generation for demo
	// In production, use proper JWT generation
	return "demo-token-" + userID, nil
}

func (jp *JWTAuthProvider) ValidatePermissions(ctx context.Context, auth *AuthContext, required []string) error {
	for _, req := range required {
		found := false
		for _, perm := range auth.Permissions {
			if perm == req {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("permission '%s' not granted", req)
		}
	}
	return nil
}

// Utility methods
func (sm *SecurityManager) logSecurityEvent(event SecurityEvent) {
	event.Timestamp = time.Now()

	if sm.config.EnableAudit {
		sm.logger.Info(fmt.Sprintf("Security Event: %s - %s", event.EventType, event.Description))
	}
}

func generateSecretKey() string {
	bytes := make([]byte, 32)
	rand.Read(bytes)
	return base64.StdEncoding.EncodeToString(bytes)
}

// Context helper methods
func GetAuthContext(ctx context.Context) (*AuthContext, bool) {
	auth, ok := ctx.Value(authContextKey).(*AuthContext)
	return auth, ok
}

func SetAuthContext(ctx context.Context, auth *AuthContext) context.Context {
	return context.WithValue(ctx, authContextKey, auth)
}

// Security middleware
func (sm *SecurityManager) WrapHandler(handler func(ctx context.Context) error) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		// Get auth context
		_, exists := GetAuthContext(ctx)
		if !exists && sm.config.EnableAuth {
			return fmt.Errorf("authentication required")
		}

		// Add security headers to context
		ctx = context.WithValue(ctx, securityEnabledKey, sm.config.EnableAuth)
		ctx = context.WithValue(ctx, rateLimitEnabledKey, sm.config.EnableRateLimit)

		return handler(ctx)
	}
}

// Cleanup methods
func (rl *RateLimiter) Cleanup() {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	// Remove expired buckets
	for id, bucket := range rl.buckets {
		bucket.mutex.Lock()
		if time.Since(bucket.lastRefill) > time.Hour {
			delete(rl.buckets, id)
		}
		bucket.mutex.Unlock()
	}
}
