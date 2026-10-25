# Antigravity CLI 窗口标题指示器 (`antigravity-cli-title`)

<p align="center">
  <b>Google Antigravity CLI (<code>agy</code>) 终端窗口标题动态感知与生命周期状态指示器</b>
</p>

<p align="center">
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-blue.svg" alt="License: MIT"></a>
  <a href="https://github.com/google-antigravity/antigravity-cli"><img src="https://img.shields.io/badge/Antigravity%20CLI-v1.4%2B-blueviolet.svg" alt="Antigravity CLI"></a>
  <img src="https://img.shields.io/badge/Platform-Windows%20%7C%20Linux%20%7C%20macOS-success.svg" alt="Platform Support">
</p>

<p align="center">
  <a href="README.md">English</a> | <b>简体中文</b>
</p>

<p align="center">
  <img src="docs/demo.gif" alt="Antigravity CLI 窗口标题生命周期指示演示" width="850">
</p>

---

## 💡 为什么需要 `antigravity-cli-title`？

在多个终端标签页（Tabs）或独立窗口中并行协作时，频繁切换上下文往往难以分辨：**哪个会话仍在运行、哪个已执行完毕、哪个正卡在交互提示等待你的确认**。

现有的解决方案（例如底部状态栏 statusline）仅在窗口置于前台时可见；而原生的 CLI 标题脚本将“等待用户确认”笼统归为工具执行阶段，导致被挂起等待的后台会话看起来依然显示为繁忙。

**`antigravity-cli-title`** 直接在你的终端标签页 / 窗口标题栏中实现了一套完整的 **4 状态生命周期状态机**：

```
[窗口标签]  agy: ⚪ Docker compose setup            <- 就绪 / 等待首次输入
[窗口标签]  agy: ⚙️ Fix user login issue            <- 正在思考 / 执行工具
[窗口标签]  agy: ⚠️ Deploy to production            <- 已阻断！等待确认或输入
[窗口标签]  agy: ✅ Refactor database schema        <- 执行完成！等待查看结果
```

---

## ✨ 核心特性

- **🎯 完整的 4 状态生命周期状态机**：
  - `⚪ 就绪 (Ready)`：新会话启动或重新加载历史会话后的初始干净状态。
  - `⚙️ 工作中 (Working)`：正在深度思考、读取文件、分析代码或执行命令、后台异步任务（Tasks）与子代理（Subagents）。
  - `⚠️ 等待中 (Waiting)`：触发交互等待（如 `ask_question`）或敏感权限门禁（`permissions.ask`，命令放行后自动切回工作中）。
  - `✅ 已完成 (Completed)`：当前轮次任务及后台子代理/任务全部完成；保持完成态直至提交新请求。
- **⚡ 后台与中断精确感知**：
  - 增量解析 `transcript.jsonl` 尾部与 `settings.json` 权限配置，联动子代理/任务计数与原生进程树，毫秒级区分“正常工具/后台执行”与“需要人工介入的授权阻塞”。
- **🪟 多窗口/多标签严格隔离**：
  - 将会话 ID 与活跃 CLI 进程生命周期精准绑定，原生进程树回溯机制，跨窗口绝不串台或状态污染。
- **🏷️ 智能标题多级回退**：
  - 优先读取 `conversation_title` ➔ 回退至用户首句输入（智能截断） ➔ 回退至当前工作区目录名。
- **🚀 原生超高速执行引擎 (`title.exe`)**：
  - 基于纯 Go 构建的高性能执行引擎，零第三方依赖，直调 Windows 系统级 API（`NtQueryInformationProcess` 与 `winsqlite3.dll`），耗时极低（通常 < 3ms，比 PowerShell 进程冷启动快 13 倍以上）。
  - `install.ps1` 安装器会自动优先在本地编译或从 GitHub Release 下载预编译的 `title.exe`。
- **🌐 全平台原生极速支持**：
  - Windows、Linux、macOS 全平台统一 **Go 原生编译二进制 (`title.exe` / `title`)**。
  - 零外部运行时依赖（无需额外安装 Python3 或配置 PowerShell），全平台微秒级极速响应。

---

## 📊 状态速查表

| 状态指示符 | 纯文本模式 | 对应阶段 | 含义解释 |
| :---: | :---: | :--- | :--- |
| **`⚪`** | `[READY]` | **初始就绪 / 空闲** | 新会话刚刚启动，或重开旧历史会话；等待用户输入提示词。 |
| **`⚙️`** | `[WORKING]` | **思考中 / 工具执行** | Agent 正在深度思考、分析代码、运行测试或执行 Shell 命令。 |
| **`⚠️`** | `[WAITING]` | **交互阻断 / 待确认** | Agent 调用了 `ask_question`，或触发了需人工授权的敏感权限提示。 |
| **`✅`** | `[DONE]` | **当前轮次完成** | Agent 完成本轮所有工作并输出最终答复。持续保持直到提交下一次输入。 |

---

## 🚀 快速上手

### Windows (PowerShell)

在 PowerShell 中执行以下命令一键安装：

```powershell
# 1. 基础一键安装（纯净模式，仅安装标题管理器）
irm https://raw.githubusercontent.com/kjqg-cn/antigravity-cli-title/main/install.ps1 | iex

# 2. 推荐：一键安装并自动配置 Windows Terminal 专属 Profile（官方彩色 Logo + 敲 agy 自动唤起）
& ([scriptblock]::Create((irm https://raw.githubusercontent.com/kjqg-cn/antigravity-cli-title/main/install.ps1))) -WTProfile
```

> [!TIP]
> 如果此前已安装过，任何时候运行带 `-WTProfile` 的命令均可**安全增量开启 Profile**，已有的自定义配置将 100% 完整保留。

*或者克隆源码本地安装：*

```powershell
git clone https://github.com/kjqg-cn/antigravity-cli-title.git
cd antigravity-cli-title
.\install.ps1

# 本地带 Profile 安装
.\install.ps1 -WTProfile
```

### Linux / macOS (Bash)

在终端中执行以下命令一键安装：

```bash
# 通过 GitHub 一键在线安装
curl -fsSL https://raw.githubusercontent.com/kjqg-cn/antigravity-cli-title/main/install.sh | bash
```

*或者克隆源码本地安装：*

```bash
git clone https://github.com/kjqg-cn/antigravity-cli-title.git
cd antigravity-cli-title
./install.sh
```

---

## ⚙️ 配置与可选参数

安装程序会自动配置 `~/.gemini/antigravity-cli/settings.json`：

```json
{
  "title": {
    "type": "command",
    "command": "C:/Users/YourName/.gemini/antigravity-cli-title/title.exe",
    "enabled": true
  }
}
```

### 自定义状态标识与即时热重载（Hot-Reload）

你可以直接通过安装脚本传参（如 `-WaitingSymbol 🔔`），或在 `settings.json` 的 `command` 字段中追加参数来自由定制各状态指示器：

```json
{
  "title": {
    "type": "command",
    "command": "C:/Users/YourName/.gemini/antigravity-cli-title/title.exe --no-prefix --waiting 🔔 --working ⚡",
    "enabled": true
  }
}
```

> [!NOTE]
> **免重启即时热生效（Hot-Reload）**：`title.exe` 在运行时会实时动态解析 `settings.json` 中 `command` 的最新参数。修改保存后，**下一个动作立即换用新图标，完全无需重启 `agy`**！
>
> **为什么不配置独立 JSON 属性？**：官方 `agy` 拥有强类型结构体白名单序列化机制，在会话重启或更新配置写盘时，会将未在官方结构体中声明的未知 key 自动清除。因此将参数安全配置在官方合法的 `command` 字段中是最稳固、防清洗的标准实践。

| 参数选项 | 示例 | 作用说明 |
| :--- | :--- | :--- |
| `--no-prefix` | `title.exe --no-prefix` | 隐藏开头的 `agy: ` 标识（显示如：`✅ Refactor login`）。 |
| `--prefix <str>` | `title.exe --prefix 🤖` | 自定义前缀标识符。 |
| `--text` / `--classic` | `title.exe --text` | 使用 `[DONE]` 等纯 ASCII 文本指示器替代 Emoji 图标。 |
| `--max-length <n>` | `title.exe --max-length 40` | 设置标题截断的最大字符长度（默认：35）。 |
| `--task-timeout <n>` | `title.exe --task-timeout 60` | 后台异步任务输出静默超时（秒），超过此时间无新输出自动解除阻塞进入 DONE（默认：60）。 |
| `--ready <str>` | `title.exe --ready 💤` | 自定义就绪（Ready）状态图标/字符（默认：`⚪` 或 `[READY]`）。 |
| `--working <str>` | `title.exe --working ⚡` | 自定义运行中（Working）状态图标/字符（默认：`⚙️` 或 `[WORKING]`）。 |
| `--waiting <str>` | `title.exe --waiting 🔔` | 自定义等待审批/交互（Waiting）状态图标/字符（默认：`⚠️` 或 `[WAITING]`）。 |
| `--done <str>` | `title.exe --done 🎉` | 自定义完成（Done）状态图标/字符（默认：`✅` 或 `[DONE]`）。 |

> [!TIP]
> **零反斜杠安全规范**：在 `command` 命令行中配置时，常规单个 Emoji 或无空格的字符**无需添加任何引号**（直接写 `--waiting 🔔` 即可），这样可彻底杜绝在 JSON 内部产生 `\"` 转义反斜杠；仅当参数值包含空格（如 `--prefix "My Bot"`）时才需要引号包裹。

---

## 🏗️ 状态机架构图

```mermaid
stateDiagram-v2
    [*] --> 就绪: 启动 CLI / 重新进入历史会话
    就绪 --> 工作中: 用户提交指令
    
    state 工作中 {
        [*] --> 思考中
        思考中 --> 执行工具: 调用 tool_calls
        执行工具 --> 思考中: 接收工具输出返回
    }
    
    工作中 --> 等待中: ask_question 或 敏感权限拦截
    等待中 --> 工作中: 用户确认授权 / 回答提问
    
    工作中 --> 已完成: 输出最终答复
    已完成 --> 工作中: 用户提交新指令
    已完成 --> 就绪: CLI 重启（新进程 PID）
```

---

## 🗑️ 卸载说明

如需移除标题栏配置并恢复原有设置：

### Windows：
```powershell
# 直接运行已安装目录中的卸载脚本：
powershell.exe -File "$HOME\.gemini\antigravity-cli-title\uninstall.ps1"

# 若需彻底清除安装目录及所有历史状态标记：
powershell.exe -File "$HOME\.gemini\antigravity-cli-title\uninstall.ps1" -Purge
```
*(或者在克隆的源码仓库中执行 `.\uninstall.ps1 [-Purge]`)*

### Linux / macOS：
```bash
# 直接运行已安装目录中的卸载脚本：
~/.gemini/antigravity-cli-title/uninstall.sh

# 若需彻底清除安装目录及所有历史状态标记：
~/.gemini/antigravity-cli-title/uninstall.sh --purge
```
*(或者在克隆的源码仓库中执行 `./uninstall.sh [--purge]`)*

---

## 🤝 生态搭配推荐

想要获取极致的终端 AI 开发体验，推荐将 **`antigravity-cli-title`** 搭配以下插件使用：
- **[`antigravity-cli-statusline`](https://github.com/weby-homelab/antigravity-cli-statusline)**：位于屏幕底部的全功能开发者遥测面板（实时展示 Git 分支状态、上下文占用条、Token 与配额用量等）。

---

## 📄 开源许可

本项目遵循 [MIT License](LICENSE) 开源协议。
