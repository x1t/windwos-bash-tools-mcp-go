// 工具输入类型定义
type ToolInput =

  | BashInput
  | BashOutputInput

  | KillShellInput


// Bash工具输入
interface BashInput {
  /** 要执行的命令 */
  command: string;
  /** 必选超时时间（毫秒，最大600000） */
  timeout: number;
  /** 5-10个词简要描述命令功能 */
  description?: string;
  /** 设置为true可在后台运行命令 */
  run_in_background: boolean;
}

// BashOutput工具输入
interface BashOutputInput {
  /** 要获取输出的后台shell ID */
  bash_id: string;
  /** 用于过滤输出行的可选正则表达式 */
  filter?: string;
}


// KillBash工具输入
interface KillShellInput {
  /** 要终止的后台shell的ID */
  shell_id: string;
}


// 工具输出类型定义
type ToolOutput =

  | BashOutput
  | BashOutputToolOutput

  | KillBashOutput


// Bash工具输出
interface BashOutput {
  /** 合并的stdout和stderr输出 */
  output: string;
  /** 命令的退出代码 */
  exitCode: number;
  /** 命令是否因超时而被终止 */
  killed?: boolean;
  /** 后台进程的shell ID */
  shellId?: string;
}

// BashOutput工具输出
interface BashOutputToolOutput {
  /** 自上次检查以来的新输出 */
  output: string;
  /** 当前shell状态 */
  status: 'running' | 'completed' | 'failed';
  /** 退出代码（完成时） */
  exitCode?: number;
}


// KillBash工具输出
interface KillBashOutput {
  /** 成功消息 */
  message: string;
  /** 被终止的shell ID */
  shell_id: string;
}
