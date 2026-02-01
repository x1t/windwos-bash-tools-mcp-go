package security

import (
	"regexp"
	"strings"
	"sync"
)

// 危险命令模式列表 (Windows专用) - 预编译正则表达式
// 采用黑名单策略：只拦截明确会造成系统破坏的命令
var dangerousPatterns = []string{
	// 系统破坏命令 (Windows) - 只拦截带破坏性参数的
	`del\s+[/\\]f.*[/\\]s`, // del /f /s (递归强制删除)
	`del\s+[/\\]s.*[/\\]f`, // del /s /f
	`rmdir\s+[/\\]s`,       // rmdir /s (递归删除目录)
	`rd\s+[/\\]s`,          // rd /s
	`format\s+[a-zA-Z]:`,   // format C: (格式化磁盘)
	`fdisk`,                // fdisk (磁盘分区)
	`diskpart`,             // diskpart (磁盘管理)

	// 系统控制命令 - 关机/重启
	`shutdown\s+[/\\-]`, // shutdown /s 或 shutdown -s
	`restart-computer`,  // PowerShell重启
	`stop-computer`,     // PowerShell关机

	// 权限提升和用户管理 - 只拦截添加管理员
	`net\s+user.*[/\\]add`,                       // 添加用户
	`net\s+localgroup.*administrators.*[/\\]add`, // 添加到管理员组

	// 系统关键文件修改 - 只拦截Windows和System32目录
	`del\s+.*[/\\]windows[/\\]system32`,
	`rmdir\s+.*[/\\]windows[/\\]system32`,
	`rd\s+.*[/\\]windows[/\\]system32`,

	// 后门和恶意软件下载
	`powershell.*-enc`,                  // PowerShell编码命令
	`powershell.*-encodedcommand`,       // PowerShell编码命令
	`invoke-expression.*downloadstring`, // 下载并执行
	`iex.*downloadstring`,               // 下载并执行
	`certutil.*-urlcache.*http`,         // certutil下载文件
	`bitsadmin.*[/\\]transfer.*http`,    // bitsadmin下载

	// 注册表危险操作 - 只拦截删除和修改HKLM
	`reg\s+delete.*hklm`,
	`reg\s+delete.*hkey_local_machine`,
	`remove-item.*hklm:.*-recurse`,
}

// 预编译的正则表达式缓存
var compiledPatterns []*regexp.Regexp
var compileOnce sync.Once

// initializePatterns 初始化并编译所有正则表达式
func initializePatterns() {
	// 编译危险命令模式
	compiledPatterns = make([]*regexp.Regexp, 0, len(dangerousPatterns))
	for _, pattern := range dangerousPatterns {
		if re, err := regexp.Compile(pattern); err == nil {
			compiledPatterns = append(compiledPatterns, re)
		}
	}
}

// IsDangerousCommand 检测潜在的恶意命令
// 采用黑名单策略：只拦截明确危险的命令，而不是要求所有命令都匹配白名单
func IsDangerousCommand(command string) bool {
	// 确保正则表达式已编译
	compileOnce.Do(initializePatterns)

	// 转换为小写进行检测
	lowerCommand := strings.ToLower(command)

	// 检查危险命令模式（黑名单）
	for _, re := range compiledPatterns {
		if matches := re.FindStringIndex(lowerCommand); matches != nil {
			// 检查匹配位置是否在引号外
			if !isInQuotes(command, matches[0]) {
				return true
			}
		}
	}

	// 检查特别危险的字符组合（只拦截明确的恶意模式）
	dangerousSequences := []struct {
		pattern string
		desc    string
	}{
		{";rm ", "命令注入删除文件"},
		{";del ", "命令注入删除文件"},
		{"; rm ", "命令注入删除文件"},
		{"; del ", "命令注入删除文件"},
		{"| rm ", "管道删除文件"},
		{"| del ", "管道删除文件"},
		{"`rm ", "反引号命令注入"},
		{"`del ", "反引号命令注入"},
		{"$(rm ", "命令替换删除"},
		{"$(del ", "命令替换删除"},
	}

	for _, seq := range dangerousSequences {
		if strings.Contains(lowerCommand, seq.pattern) {
			// 检查是否在引号内
			idx := strings.Index(lowerCommand, seq.pattern)
			if idx >= 0 && !isInQuotes(command, idx) {
				return true
			}
		}
	}

	// 允许所有其他命令（包括管道、重定向、命令链接等）
	// 这是一个更宽松的策略，信任用户不会执行恶意命令
	return false
}

// isEntireCommandQuoted 检查整个命令是否都被引号包围
func isEntireCommandQuoted(command string) bool {
	trimmed := strings.TrimSpace(command)
	if len(trimmed) < 2 {
		return false
	}

	// 检查单引号
	if trimmed[0] == '\'' && trimmed[len(trimmed)-1] == '\'' {
		return true
	}

	// 检查双引号
	if trimmed[0] == '"' && trimmed[len(trimmed)-1] == '"' {
		return true
	}

	return false
}

// isInQuotes 检查指定位置是否在引号内 (PowerShell特有逻辑)
// PowerShell使用反引号(`)作为转义字符，而非反斜杠(\)
// 同时支持PowerShell的Here-String语法: @"..."@ 和 @'...'@
func isInQuotes(command string, pos int) bool {
	if pos < 0 || pos >= len(command) {
		return false
	}

	// 先检查是否在Here-String内
	if isInHereString(command, pos) {
		return true
	}

	before := command[:pos]
	singleQuoteCount := 0
	doubleQuoteCount := 0

	// 计算有效的引号数量（排除PowerShell转义的引号）
	// PowerShell转义字符是反引号(`)，不是反斜杠(\)
	for i := 0; i < len(before); i++ {
		char := before[i]

		// 检查是否被反引号转义
		isEscaped := i > 0 && before[i-1] == '`'

		if char == '\'' && !isEscaped {
			singleQuoteCount++
		}
		if char == '"' && !isEscaped {
			doubleQuoteCount++
		}

		// 跳过转义字符本身
		if char == '`' && i+1 < len(before) {
			i++ // 跳过下一个被转义的字符
		}
	}

	return singleQuoteCount%2 == 1 || doubleQuoteCount%2 == 1
}

// isInHereString 检查指定位置是否在PowerShell Here-String内
// Here-String语法: @"..."@ (可展开) 和 @'...'@ (原义)
func isInHereString(command string, pos int) bool {
	// 查找所有 @" 或 @' 开始标记
	hereStringStarts := []struct {
		pos      int
		isDouble bool // true = @", false = @'
	}{}

	for i := 0; i < len(command)-1; i++ {
		if command[i] == '@' {
			if command[i+1] == '"' {
				hereStringStarts = append(hereStringStarts, struct {
					pos      int
					isDouble bool
				}{i, true})
				i++ // 跳过引号
			} else if command[i+1] == '\'' {
				hereStringStarts = append(hereStringStarts, struct {
					pos      int
					isDouble bool
				}{i, false})
				i++ // 跳过引号
			}
		}
	}

	// 检查pos是否在任何Here-String内
	for _, start := range hereStringStarts {
		if start.pos >= pos {
			continue // 开始位置在检查位置之后
		}

		// 查找对应的结束标记
		endMarker := `"@`
		if !start.isDouble {
			endMarker = `'@`
		}

		// 从开始标记之后查找结束标记
		endPos := strings.Index(command[start.pos+2:], endMarker)
		if endPos == -1 {
			// 没有找到结束标记，认为一直到末尾都在Here-String内
			if pos > start.pos+1 {
				return true
			}
		} else {
			// 计算实际结束位置
			actualEndPos := start.pos + 2 + endPos + len(endMarker)
			if pos > start.pos+1 && pos < actualEndPos {
				return true
			}
		}
	}

	return false
}
