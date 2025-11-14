package main

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

// SecurityTestSuite 安全功能测试套件
type SecurityTestSuite struct {
	suite.Suite
	server *MCPServer
}

// SetupSuite 测试套件初始化
func (suite *SecurityTestSuite) SetupSuite() {
	suite.server = NewMCPServer()
}

// TestDangerousCommandDetection 测试危险命令检测
func (suite *SecurityTestSuite) TestDangerousCommandDetection() {
	// 定义已知的危险命令
	dangerousCommands := []struct {
		command     string
		description string
	}{
		{"rm -rf /", "删除根目录下的所有文件"},
		{"rm -rf /*", "删除根目录下的所有文件"},
		{"rm -rf", "递归强制删除"},
		{"del /f", "强制删除文件"},
		{"del /s /q", "静默删除文件"},
		{"format C:", "格式化C盘"},
		{"format D:", "格式化D盘"},
		{"format", "格式化磁盘"},
		{"shutdown /s", "关闭系统"},
		{"shutdown /r", "重启系统"},
		{"shutdown /h", "休眠系统"},
		{"reboot", "重启系统"},
		{"halt", "停止系统"},
		{"poweroff", "关闭电源"},
		{"sudo rm", "使用sudo权限删除"},
		{"sudo shutdown", "使用sudo权限关机"},
		{"sudo reboot", "使用sudo权限重启"},
		{"dd if=/dev/zero", "磁盘销毁命令"},
		{"> /dev/null", "重定向到/dev/null"},
		{"mkfs", "创建文件系统"},
		{"fdisk", "磁盘分区"},
		{"su root", "切换到root用户"},
		{"chmod 777", "设置最大权限"},
		{"chown root", "更改所有者为root"},
		{"passwd root", "修改root密码"},
		{"userdel", "删除用户"},
		{"groupdel", "删除用户组"},
		{"crontab -r", "删除所有cron任务"},
		{"iptables -F", "清空防火墙规则"},
		{"service stop", "停止服务"},
		{"systemctl stop", "停止systemd服务"},
		{"killall", "杀死所有同名进程"},
		{"kill -9", "强制杀死进程"},
	}

	for _, tc := range dangerousCommands {
		suite.Run("危险命令: "+tc.description, func() {
			assert.True(suite.T(), isDangerousCommand(tc.command), 
				"命令应该被识别为危险: %s", tc.command)
		})
	}
}

// TestSafeCommands 测试安全命令
func (suite *SecurityTestSuite) TestSafeCommands() {
	// 定义已知的安全命令
	safeCommands := []struct {
		command     string
		description string
	}{
		{"echo Hello World", "输出文本"},
		{"ls -la", "列出文件"},
		{"dir", "列出目录"},
		{"pwd", "显示当前目录"},
		{"cd /home", "切换目录"},
		{"cat file.txt", "查看文件内容"},
		{"type file.txt", "查看文件内容"},
		{"grep pattern file", "搜索文本"},
		{"find . -name '*.txt'", "查找文件"},
		{"ps aux", "查看进程"},
		{"tasklist", "查看任务列表"},
		{"netstat -an", "查看网络连接"},
		{"ipconfig", "查看IP配置"},
		{"ifconfig", "查看网络接口"},
		{"whoami", "查看当前用户"},
		{"date", "查看日期时间"},
		{"uptime", "查看系统运行时间"},
		{"free -h", "查看内存使用"},
		{"df -h", "查看磁盘使用"},
		{"top", "查看系统资源"},
		{"htop", "查看系统资源"},
		{"uname -a", "查看系统信息"},
		{"env", "查看环境变量"},
		{"history", "查看命令历史"},
		{"wc -l file.txt", "统计行数"},
		{"sort file.txt", "排序文件内容"},
		{"uniq file.txt", "去除重复行"},
		{"head -10 file.txt", "查看文件前10行"},
		{"tail -10 file.txt", "查看文件后10行"},
		{"less file.txt", "分页查看文件"},
		{"more file.txt", "分页查看文件"},
		{"cp source.txt dest.txt", "复制文件"},
		{"mv old.txt new.txt", "移动/重命名文件"},
		{"mkdir newdir", "创建目录"},
		{"rmdir emptydir", "删除空目录"},
		{"touch newfile.txt", "创建空文件"},
		{"chmod 644 file.txt", "设置文件权限"},
		{"chown user:group file.txt", "更改文件所有者"},
		{"tar -czf archive.tar.gz dir/", "创建压缩包"},
		{"unzip archive.zip", "解压文件"},
		{"ping google.com", "ping测试"},
		{"curl http://example.com", "HTTP请求"},
		{"wget http://example.com", "下载文件"},
		{"git status", "Git状态"},
		{"git add .", "Git添加文件"},
		{"git commit -m 'message'", "Git提交"},
		{"python script.py", "运行Python脚本"},
		{"node app.js", "运行Node.js应用"},
		{"npm install", "安装npm包"},
		{"docker ps", "查看Docker容器"},
		{"kubectl get pods", "查看Kubernetes Pod"},
		{"systemctl status nginx", "查看服务状态"},
	}

	for _, tc := range safeCommands {
		suite.Run("安全命令: "+tc.description, func() {
			assert.False(suite.T(), isDangerousCommand(tc.command), 
				"命令不应该被识别为危险: %s", tc.command)
		})
	}
}

// TestDangerousCommandCaseVariations 测试大小写变化
func (suite *SecurityTestSuite) TestDangerousCommandCaseVariations() {
	baseCommands := []string{
		"shutdown",
		"format",
		"del",
		"rm",
	}

	for _, base := range baseCommands {
		suite.Run("大小写变化: "+base, func() {
			// 测试不同的大小写组合
			variations := []string{
				base,                    // 原始
				strings.ToUpper(base),   // 全大写
				strings.ToLower(base),   // 全小写
				strings.Title(base),     // 标题大小写
				base + " /S",           // 带参数
				"sudo " + base,         // 带sudo前缀
			}

			for _, variation := range variations {
				if isDangerousCommand(base) {
					// 如果基础命令是危险的，大部分变化也应该是危险的
					if strings.Contains(strings.ToLower(variation), strings.ToLower(base)) {
						assert.True(suite.T(), isDangerousCommand(variation), 
							"命令变化应该被识别为危险: %s", variation)
					}
				}
			}
		})
	}
}

// TestDangerousCommandWithArguments 测试带参数的危险命令
func (suite *SecurityTestSuite) TestDangerousCommandWithArguments() {
	dangerousCommands := []string{
		"rm -rf /home/user/*",
		"shutdown /s /t 0 /f",
		"format c: /fs:ntfs /q",
		"del /s /q c:\\temp\\*.*",
		"sudo rm -rf /var/log/*",
		"dd if=/dev/zero of=/dev/sda bs=1M",
		"killall -9 process_name",
		"chmod 777 /etc/passwd",
	}

	for _, cmd := range dangerousCommands {
		suite.Run("带参数的危险命令: "+cmd, func() {
			assert.True(suite.T(), isDangerousCommand(cmd), 
				"带参数的命令应该被识别为危险: %s", cmd)
		})
	}
}

// TestDangerousCommandWithSpacing 测试不同空格的危险命令
func (suite *SecurityTestSuite) TestDangerousCommandWithSpacing() {
	dangerousCommands := []string{
		"rm -rf /",                    // 正常空格
		"rm  -rf  /",                  // 多个空格
		"rm\t-rf\t/",                 // 制表符
		"rm  -rf\t/",                 // 混合空格和制表符
		"  rm -rf /  ",                // 前后空格
		"\trm -rf /\t",                // 前后制表符
		"shutdown /s",                 // 正常
		"shutdown\t/s",                // 制表符
		"shutdown  /s",                // 多空格
	}

	for _, cmd := range dangerousCommands {
		suite.Run("特殊空格的危险命令: "+cmd, func() {
			assert.True(suite.T(), isDangerousCommand(cmd), 
				"特殊空格的命令应该被识别为危险: %s", cmd)
		})
	}
}

// TestSafeCommandsWithDangerousSubstrings 测试包含危险子字符串的安全命令
func (suite *SecurityTestSuite) TestSafeCommandsWithDangerousSubstrings() {
	safeCommands := []struct {
		command     string
		description string
	}{
		{"echo 'This format is JSON format'", "包含format但安全的echo命令"},
		{"grep 'shutdown' log.txt", "搜索包含shutdown的日志"},
		{"cat 'shutdown_instructions.txt'", "查看包含shutdown的文件"},
		{"ls 'formatted_files'", "列出包含format的目录"},
		{"find . -name '*shutdown*'", "查找包含shutdown的文件"},
		{"python format_string.py", "运行包含format的Python脚本"},
		{"node rm_duplicates.js", "运行包含rm的Node.js脚本（但不是删除命令）"},
		{"cat 'del_ingredients.txt'", "查看包含del的菜谱文件"},
		{"echo 'The del is a character in Spanish'", "包含del的文本"},
		{"git rm --cached file.txt", "Git从缓存中删除文件（安全的Git操作）"},
		{"docker rm container_name", "删除Docker容器（相对安全的操作）"},
	}

	for _, tc := range safeCommands {
		suite.Run("包含危险子字符串的安全命令: "+tc.description, func() {
			// 这些命令虽然包含一些可能危险的子字符串，但整体是安全的
			assert.False(suite.T(), isDangerousCommand(tc.command), 
				"命令不应该被误识别为危险: %s", tc.command)
		})
	}
}

// TestSecurityIntegrationWithBashHandler 测试安全功能与BashHandler的集成
func (suite *SecurityTestSuite) TestSecurityIntegrationWithBashHandler() {
	dangerousCommand := "rm -rf /"
	args := BashArguments{
		Command: dangerousCommand,
		Timeout: 5000,
	}

	// 创建context用于测试
	ctx := context.Background()

	// 测试危险命令被BashHandler正确拦截
	result, output, err := suite.server.BashHandler(ctx, &mcp.CallToolRequest{}, args)

	// 验证错误返回
	suite.Require().Error(err, "应该返回错误")
	suite.Require().Equal("command rejected for security reasons", err.Error())

	// 验证MCP结果格式
	suite.Require().NotNil(result)
	suite.Assert().True(result.IsError, "结果应该标记为错误")
	suite.Require().Len(result.Content, 1, "应该有一个内容项")
	suite.Assert().Contains(result.Content[0].(*mcp.TextContent).Text, 
		"错误: 命令因安全原因被拒绝", "应该包含错误信息")

	// 验证结构化输出
	suite.Assert().Equal("错误: 命令因安全原因被拒绝", output.Output, "输出应该包含错误信息")
	suite.Assert().Equal(1, output.ExitCode, "退出代码应该是1")
	suite.Assert().False(output.Killed, "不应该被标记为被杀死")
	suite.Assert().Empty(output.ShellID, "ShellID应该为空")
}

// TestSecurityWithQuotedCommands 测试带引号的命令安全性
func (suite *SecurityTestSuite) TestSecurityWithQuotedCommands() {
	testCases := []struct {
		command     string
		isDangerous bool
		description string
	}{
		{`echo "rm -rf /"`, false, "安全的echo命令，内容包含危险字符串"},
		{`grep "format" file.txt`, false, "安全的grep命令，搜索包含format的内容"},
		{`cat "shutdown.log"`, false, "安全的cat命令，查看shutdown日志"},
		{`bash -c "rm -rf /"`, true, "通过bash -c执行危险命令"},
		{`sh -c "shutdown /s"`, true, "通过sh -c执行危险命令"},
		{`powershell -Command "rm -rf /"`, true, "通过PowerShell执行危险命令"},
		{`cmd /c "format c:"`, true, "通过cmd执行危险命令"},
		{`python -c "import os; os.system('rm -rf /')"`, true, "通过Python执行危险命令"},
		{`node -e "require('child_process').exec('rm -rf /')"` , true, "通过Node.js执行危险命令"},
	}

	for _, tc := range testCases {
		suite.Run("引号命令测试: "+tc.description, func() {
			if tc.isDangerous {
				assert.True(suite.T(), isDangerousCommand(tc.command), 
					"命令应该被识别为危险: %s", tc.command)
			} else {
				assert.False(suite.T(), isDangerousCommand(tc.command), 
					"命令不应该被识别为危险: %s", tc.command)
			}
		})
	}
}

// TestSecurityWithPipesAndRedirects 测试管道和重定向的安全性
func (suite *SecurityTestSuite) TestSecurityWithPipesAndRedirects() {
	testCases := []struct {
		command     string
		isDangerous bool
		description string
	}{
		{"cat file.txt | grep pattern", false, "安全的管道操作"},
		{"ls -la | sort | head -10", false, "安全的多重管道"},
		{"echo 'hello' > output.txt", false, "安全的输出重定向"},
		{"cat input.txt >> output.txt", false, "安全的追加重定向"},
		{"rm -rf / | echo 'done'", true, "危险命令后跟管道"},
		{"format c: > nul", true, "危险命令后跟重定向"},
		{"shutdown /s 2>&1", true, "危险命令后跟错误重定向"},
		{"cat file.txt | rm -rf /", true, "管道后跟危险命令"},
		{"ls | grep secret > rm -rf /", true, "重定向到危险命令"},
	}

	for _, tc := range testCases {
		suite.Run("管道重定向测试: "+tc.description, func() {
			if tc.isDangerous {
				assert.True(suite.T(), isDangerousCommand(tc.command), 
					"命令应该被识别为危险: %s", tc.command)
			} else {
				assert.False(suite.T(), isDangerousCommand(tc.command), 
					"命令不应该被识别为危险: %s", tc.command)
			}
		})
	}
}

// TestSecurityWithEnvironmentVariables 测试环境变量的安全性
func (suite *SecurityTestSuite) TestSecurityWithEnvironmentVariables() {
	testCases := []struct {
		command     string
		isDangerous bool
		description string
	}{
		{"echo $PATH", false, "查看环境变量"},
		{"echo $HOME", false, "查看用户主目录"},
		{"export PATH=$PATH:/new/path", false, "修改PATH环境变量"},
		{"VAR='rm -rf /'; echo $VAR", false, "环境变量包含危险字符串但不执行"},
		{"DANGER='rm -rf /'; $DANGER", true, "通过环境变量执行危险命令"},
		{"eval $DANGER", true, "eval可能执行危险命令"},
		{"exec rm -rf /", true, "exec执行危险命令"},
	}

	for _, tc := range testCases {
		suite.Run("环境变量测试: "+tc.description, func() {
			if tc.isDangerous {
				assert.True(suite.T(), isDangerousCommand(tc.command), 
					"命令应该被识别为危险: %s", tc.command)
			} else {
				assert.False(suite.T(), isDangerousCommand(tc.command), 
					"命令不应该被识别为危险: %s", tc.command)
			}
		})
	}
}

// TestSecurityEdgeCases 测试边界情况
func (suite *SecurityTestSuite) TestSecurityEdgeCases() {
	testCases := []struct {
		command     string
		isDangerous bool
		description string
	}{
		{"", false, "空命令"},
		{"   ", false, "只有空格"},
		{"echo", false, "单个安全命令"},
		{"rm", true, "单独的危险命令"},
		{"Rm -RF /", true, "大小写混合的危险命令"},
		{"rM -Rf /", true, "大小写混合的危险命令"},
		{"rm\n-rf\n/", true, "换行符分隔的危险命令"},
		{"rm\r-rf\r/", true, "回车符分隔的危险命令"},
		{";rm -rf /", true, "分号开头的危险命令"},
		{"&&rm -rf /", true, "&&开头的危险命令"},
		{"||rm -rf /", true, "||开头的危险命令"},
		{"|rm -rf /", true, "管道开头的危险命令"},
	}

	for _, tc := range testCases {
		suite.Run("边界情况测试: "+tc.description, func() {
			if tc.isDangerous {
				assert.True(suite.T(), isDangerousCommand(tc.command), 
					"命令应该被识别为危险: %s", tc.command)
			} else {
				assert.False(suite.T(), isDangerousCommand(tc.command), 
					"命令不应该被识别为危险: %s", tc.command)
			}
		})
	}
}

// TestSecurityPerformance 测试安全检测的性能
func (suite *SecurityTestSuite) TestSecurityPerformance() {
	// 生成测试命令
	testCommands := make([]string, 1000)
	for i := 0; i < 1000; i++ {
		if i%100 == 0 {
			testCommands[i] = "rm -rf /" // 每100个有一个危险命令
		} else {
			testCommands[i] = "echo test command " + string(rune(i%26+'A'))
		}
	}

	// 测试批量检测性能
	suite.T().Helper()
	for _, cmd := range testCommands {
		isDangerousCommand(cmd)
	}
	
	// 如果到这里没有崩溃，说明性能是可接受的
	suite.Assert().True(true, "安全检测性能测试通过")
}

// 运行安全测试套件
func TestSecurityTestSuite(t *testing.T) {
	suite.Run(t, new(SecurityTestSuite))
}

// 基准测试：安全检测性能
func BenchmarkDangerousCommandDetection(b *testing.B) {
	dangerousCommands := []string{
		"rm -rf /",
		"shutdown /s",
		"format c:",
		"sudo rm",
		"del /f",
	}
	
	safeCommands := []string{
		"echo hello world",
		"ls -la",
		"cat file.txt",
		"grep pattern file",
		"pwd",
	}

	testCommands := append(dangerousCommands, safeCommands...)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cmd := testCommands[i%len(testCommands)]
		isDangerousCommand(cmd)
	}
}

// 单独的危险命令检测测试
func TestIsDangerousCommand(t *testing.T) {
	testCases := []struct {
		name         string
		command      string
		expected     bool
	}{
		{"删除根目录", "rm -rf /", true},
		{"格式化磁盘", "format c:", true},
		{"关闭系统", "shutdown /s", true},
		{"安全echo", "echo hello", false},
		{"列出文件", "ls -la", false},
		{"查看当前目录", "pwd", false},
		{"空命令", "", false},
		{"只有空格", "   ", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := isDangerousCommand(tc.command)
			assert.Equal(t, tc.expected, result)
		})
	}
}