package security

import (
	"regexp"
	"strings"
	"sync"
)

// 危险命令模式列表 (Windows专用) - 预编译正则表达式
var dangerousPatterns = []string{
	// 系统破坏命令 (Windows)
	`del\s+[/\\]`,
	`rmdir\s+.*[/\\]`,
	`rd\s+[/\\]`,
	`format\s+[a-zA-Z]:`,
	`fdisk`,
	`diskpart`,

	// 系统控制命令
	`shutdown`,
	`reboot`,
	`restart`,
	`logoff`,
	`taskkill\s+[/\\]f`,
	`tsdiscon`,
	`stop-computer`,
	`restart-computer`,

	// 权限提升命令
	`takeown`,
	`icacls.*everyone`,
	`cacls.*everyone`,
	`net\s+user.*\/add`,
	`net\s+localgroup.*administrators`,

	// 网络攻击命令
	`net\s+view`,
	`net\s+use`,
	`net\s+session`,

	// 磁盘操作危险命令
	`copy\s+con`,

	// 系统关键文件修改
	`echo\s+.*>\s*[a-zA-Z]:[/\\]windows`,
	`echo\s+.*>\s*[a-zA-Z]:[/\\]system32`,
	`del\s+.*[/\\]windows[/\\]system32`,

	// 后门和恶意软件
	`powershell.*-enc`,
	`powershell.*-encodedcommand`,
	`invoke-expression.*\(new-object`,
	`iex.*\(new-object`,
	`downloadstring`,
	`downloadfile`,
	`cmd\s+[/\\]c.*http`,
	`bitsadmin`,
	`certutil.*-urlcache`,
	`mshta.*http`,

	// 包管理器（防止安装恶意软件）
	`choco\s+install`,
	`scoop\s+install`,
	`winget\s+install`,
	`pip\s+install`,
	`npm\s+install.*-g`,
	`npm\s+install.*--global`,

	// 环境变量注入
	`setx\s+path`,
	`set\s+path=`,

	// 注册表危险操作
	`reg\s+delete`,
	`reg\s+add.*hklm`,
	`reg\s+add.*hkey_local_machine`,
	`remove-item.*hklm:`,
	`new-item.*hklm:`,
}

// 预编译的正则表达式缓存
var compiledPatterns []*regexp.Regexp
var compiledSafePipePatterns []*regexp.Regexp
var compiledSafeRedirectPatterns []*regexp.Regexp
var compileOnce sync.Once

// 安全管道模式列表
var safePipePatternStrings = []string{
	`findstr.*\|.*findstr`,
	`findstr.*\|.*more`,
	`dir\s+.*\|.*findstr`,
	`dir\s+.*\|.*find`,
	`where\s+.*\|.*xargs`,
	`get-process.*\|.*where-object`,
	`get-childitem.*\|.*select-object`,
	`get-content.*\|.*select-string`,
}

// 安全重定向模式列表
var safeRedirectPatternStrings = []string{
	`>\s*[a-zA-Z]:[/\\]temp[/\\]`,
	`>\s*[a-zA-Z]:[/\\]tmp[/\\]`,
	`>\s*\.\.\\`,
	`>\s*\\`,
	`>\s*[^/]*\.log`,
	`>\s*[^/]*\.txt`,
	`>\s*[^/]*\.out`,
	`>>\s*\.log`,
	`>>\s*\.txt`,
	`>>\s*[a-zA-Z]:[/\\]temp[/\\]`,
}

// initializePatterns 初始化并编译所有正则表达式
func initializePatterns() {
	// 编译危险命令模式
	compiledPatterns = make([]*regexp.Regexp, 0, len(dangerousPatterns))
	for _, pattern := range dangerousPatterns {
		if re, err := regexp.Compile(pattern); err == nil {
			compiledPatterns = append(compiledPatterns, re)
		}
	}

	// 编译安全管道模式
	compiledSafePipePatterns = make([]*regexp.Regexp, 0, len(safePipePatternStrings))
	for _, pattern := range safePipePatternStrings {
		if re, err := regexp.Compile(pattern); err == nil {
			compiledSafePipePatterns = append(compiledSafePipePatterns, re)
		}
	}

	// 编译安全重定向模式
	compiledSafeRedirectPatterns = make([]*regexp.Regexp, 0, len(safeRedirectPatternStrings))
	for _, pattern := range safeRedirectPatternStrings {
		if re, err := regexp.Compile(pattern); err == nil {
			compiledSafeRedirectPatterns = append(compiledSafeRedirectPatterns, re)
		}
	}
}

// IsDangerousCommand 检测潜在的恶意命令
func IsDangerousCommand(command string) bool {
	// 确保正则表达式已编译
	compileOnce.Do(initializePatterns)

	// 转换为小写进行检测
	lowerCommand := strings.ToLower(command)

	// 首先检查危险命令模式，检查是否在引号外
	for _, re := range compiledPatterns {
		if matches := re.FindStringIndex(lowerCommand); matches != nil {
			// 检查匹配位置是否在引号外
			if !isInQuotes(command, matches[0]) {
				return true
			}
		}
	}

	// 检查是否包含可疑的字符序列（但排除引号内的）
	suspiciousChars := []string{
		";",
		"|",
		"&",
		"`",
		"$(",
		">>",
		"<<",
	}

	// 如果命令中包含这些字符且不是在引号内，则可能危险
	for _, char := range suspiciousChars {
		if strings.Contains(command, char) {
			// 找到字符的所有位置
			indices := []int{}
			start := 0
			for {
				idx := strings.Index(command[start:], char)
				if idx == -1 {
					break
				}
				indices = append(indices, start+idx)
				start = start + idx + 1
			}

			// 检查每个出现的位置
			for _, pos := range indices {
				if !isInQuotes(command, pos) {
					// 对于一些常见的安全操作，允许使用管道和重定向
					if char == "|" && isSafePipeUsage(command) {
						continue
					}
					if char == ">" && isSafeRedirectUsage(command) {
						continue
					}
					if char == ">>" && isSafeRedirectUsage(command) {
						continue
					}
					// 发现不在引号内的危险字符
					return true
				}
			}
		}
	}

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
		pos     int
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

// isSafePipeUsage 检查安全的管道使用（使用预编译的正则表达式）
func isSafePipeUsage(command string) bool {
	// 确保正则表达式已编译
	compileOnce.Do(initializePatterns)

	lowerCommand := strings.ToLower(command)
	for _, re := range compiledSafePipePatterns {
		if re.MatchString(lowerCommand) {
			return true
		}
	}

	return false
}

// isSafeRedirectUsage 检查安全的重定向使用（使用预编译的正则表达式）
func isSafeRedirectUsage(command string) bool {
	// 确保正则表达式已编译
	compileOnce.Do(initializePatterns)

	lowerCommand := strings.ToLower(command)
	for _, re := range compiledSafeRedirectPatterns {
		if re.MatchString(lowerCommand) {
			return true
		}
	}

	return false
}
