package security

import (
	"regexp"
	"strings"
)

// IsDangerousCommand 检测潜在的恶意命令
func IsDangerousCommand(command string) bool {
	// 转换为小写进行检测
	lowerCommand := strings.ToLower(command)
	
	// 危险命令模式列表
	dangerousPatterns := []string{
		// 系统破坏命令
		`rm\s+-rf\s+/`,
		`rm\s+-rf\s+/[^ ]*`,
		`mkfs`,
		`format`,
		`fdisk`,
		
		// 系统控制命令
		`shutdown`,
		`reboot`,
		`halt`,
		`poweroff`,
		
		// 用户管理命令
		`useradd`,
		`userdel`,
		`passwd`,
		`su\s`,
		`sudo\s+su`,
		
		// 网络攻击命令
		`iptables\s+-[fF]`,
		`iptables\s+-[fF]\s+.*`,
		`:\(\)\{\.*\}`,
		`:\(\)\{\.*\}\;`,
		`fork\s+bomb`,
		
		// 设备文件操作
		`dd\s+if=/dev/zero`,
		`dd\s+if=/dev/random`,
		`>\s+/dev/sda`,
		
		// 系统关键文件修改
		`echo\s+.*>\s*/etc/passwd`,
		`echo\s+.*>\s*/etc/shadow`,
		`chmod\s+777\s+/`,
		`chown\s+.*\s+/`,
		
		// 后门和恶意软件
		`curl.*\|.*sh`,
		`wget.*\|.*bash`,
		`nc\s+-l`,
		`netcat\s+-l`,
		`/bin/sh`,
		`/bin/bash`,
		
		// 包管理器（防止安装恶意软件）
		`apt-get\s+install`,
		`yum\s+install`,
		`pip\s+install`,
		`npm\s+install`,
		
		// 环境变量注入
		`export.*=.*\$\(`,
		`export.*=.*` + "`" + `.*` + "`",
	}
	
	// 首先检查完整的危险命令模式（包括引号内的）
	for _, pattern := range dangerousPatterns {
		if matched, _ := regexp.MatchString(pattern, lowerCommand); matched {
			// 如果匹配到了，需要检查是否整个命令都在引号内作为字符串参数
			if isEntireCommandQuoted(command) {
				return false // 整个命令被引号包围，认为是安全的
			}
			return true
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

// isInQuotes 检查指定位置是否在引号内
func isInQuotes(command string, pos int) bool {
	if pos < 0 || pos >= len(command) {
		return false
	}
	
	before := command[:pos]
	singleQuoteCount := 0
	doubleQuoteCount := 0
	
	// 计算有效的引号数量（排除转义的引号）
	for i, char := range before {
		if char == '\'' && (i == 0 || before[i-1] != '\\') {
			singleQuoteCount++
		}
		if char == '"' && (i == 0 || before[i-1] != '\\') {
			doubleQuoteCount++
		}
	}
	
	return singleQuoteCount%2 == 1 || doubleQuoteCount%2 == 1
}

// isSafePipeUsage 检查安全的管道使用
func isSafePipeUsage(command string) bool {
	safePipePatterns := []string{
		`grep\s+.*\|.*head`,
		`grep\s+.*\|.*tail`,
		`grep\s+.*\|.*wc`,
		`cat\s+.*\|.*grep`,
		`ls\s+.*\|.*grep`,
		`ps\s+.*\|.*grep`,
		`find\s+.*\|.*xargs`,
	}
	
	lowerCommand := strings.ToLower(command)
	for _, pattern := range safePipePatterns {
		if matched, _ := regexp.MatchString(pattern, lowerCommand); matched {
			return true
		}
	}
	
	return false
}

// isSafeRedirectUsage 检查安全的重定向使用
func isSafeRedirectUsage(command string) bool {
	// 允许重定向到临时目录或当前目录
	safeRedirectPatterns := []string{
		`>\s*/tmp/`,
		`>\s*\.\/`,
		`>\s*[^/]*\.log`,
		`>\s*[^/]*\.txt`,
		`>\s*[^/]*\.out`,
	}
	
	lowerCommand := strings.ToLower(command)
	for _, pattern := range safeRedirectPatterns {
		if matched, _ := regexp.MatchString(pattern, lowerCommand); matched {
			return true
		}
	}
	
	return false
}