# 工具链与依赖

本文档是 [`AGENTS.md`](../AGENTS.md) 的**人类版详尽补充**——含更多背景、备选方案、故障排查。

---

## 本项目自身

| 资源 | 地址 | 说明 |
|---|---|---|
| Gitea Web UI | https://github.com/walker83/ontobuddy | 浏览代码、PR、Issue |
| Git 远程 | `https://github.com/walker83/ontobuddy.git` | 推送/拉取用 |
| 克隆 | `git clone https://github.com/walker83/ontobuddy.git` | |
| 二进制（全局安装） | `~/.local/bin/myonto` | `make link` 创建符号链接 |
| 二进制（本地构建） | `./bin/myonto` | `make build` 产物 |
| Skills 安装路径 | `~/.claude/skills/myonto/` | `make install-skills` 复制 |


---

## 上游 Go 生态

### Go 工具链

| 来源 | URL | 速度 | 备注 |
|---|---|---|---|
| 阿里云镜像 | `https://mirrors.aliyun.com/golang/` | 快（国内 CDN） | **推荐** |
| 官方 | `https://go.dev/dl/` | 慢（境外） | 备选 |

**安装命令**（阿里云镜像）：

```bash
# macOS arm64
curl -fSL -o /tmp/go.tar.gz https://mirrors.aliyun.com/golang/go1.26.4.darwin-arm64.tar.gz
mkdir -p $HOME/.local && tar -C $HOME/.local -xzf /tmp/go.tar.gz
export PATH=$HOME/.local/go/bin:$PATH
```

### Go 模块代理

| 代理 | URL | 备注 |
|---|---|---|
| goproxy.cn | `https://goproxy.cn,direct` | 七牛维护，**实测最快** |
| 阿里云官方 | `https://mirrors.aliyun.com/goproxy/,direct` | 阿里云背书 |
| proxy.golang.org | 默认 | 境外，慢 |

**配置**（`make setup` 自动完成）：

```bash
go env -w GOPROXY=https://goproxy.cn,direct
go env -w GOSUMDB=sum.golang.google.cn
```

### 校验和数据库

| 源 | URL | 备注 |
|---|---|---|
| 阿里云镜像 | `sum.golang.google.cn` | 默认 + 国内可达 |
| sum.golang.org | 默认 | 境外 |

---

## 直接 Go 依赖

本项目**不依赖任何** Go 标准库之外的 Go module 做核心功能；以下 5 个是直接依赖。

| 包 | 版本 | 许可证 | 作用 |
|---|---|---|---|
| `github.com/alecthomas/kong` | v1.15.0 | MIT | CLI 框架（命令解析 + help 生成） |
| `github.com/charmbracelet/huh` | v1.0.0 | MIT | TUI 表单向导（基于 bubbletea） |
| `github.com/pelletier/go-toml/v2` | v2.3.1 | Apache-2.0 | `.myonto.toml` 解析 |
| `golang.org/x/crypto` | latest | BSD-3-Clause | scrypt KDF（API key 加密） |
| `golang.org/x/term` | latest | BSD-3-Clause | 终端密码隐藏输入 |

### 间接依赖（共 26 个）

通过上述 5 个直接依赖间接拉入，主要是 charmbracelet 生态（bubbletea/lipgloss/bubbles/ansi/term 等 TUI 辅助）。完整列表：

```
github.com/atotto/clipboard
github.com/aymanbagabas/go-osc52/v2
github.com/catppuccin/go
github.com/charmbracelet/bubbles
github.com/charmbracelet/bubbletea
github.com/charmbracelet/colorprofile
github.com/charmbracelet/lipgloss
github.com/charmbracelet/x/ansi
github.com/charmbracelet/x/cellbuf
github.com/charmbracelet/x/exp/strings
github.com/dustin/go-humanize
github.com/erikgeiser/coninput
github.com/hexops/gotextdiff
github.com/lucasb-eyer/go-colorful
github.com/mattn/go-isatty
github.com/mattn/go-localereader
github.com/mattn/go-runewidth
github.com/mitchellh/hashstructure/v2
github.com/muesli/ansi
github.com/muesli/cancelreader
github.com/muesli/termenv
github.com/rivo/uniseg
github.com/xo/terminfo
golang.org/x/sync
golang.org/x/sys
golang.org/x/text
```

---

## 构建工具

| 工具 | 版本 | 用途 | 替代 |
|---|---|---|---|
| GNU Make | 3.81+ | `make` 命令 | Task、just、mage |
| go | 1.22+ | 编译、vet、test、build | — |
| 阿里云 GOPROXY | - | 模块下载 | goproxy.cn、proxy.golang.org |
| `bash` | - | Makefile 调用 shell | zsh、sh |

### Makefile target 速查

| target | 作用 |
|---|---|
| `setup` | 配置 Go 镜像（首次必做） |
| `build` | 编译到 `./bin/myonto` |
| `link` | 全局安装（`~/.local/bin/myonto`） |
| `unlink` | 卸载符号链接 |
| `install-skills` | 复制 skills 到 `~/.claude/skills/myonto/` |
| `uninstall-skills` | 卸载 skills |
| `test` | 跑全部单测（69 个） |
| `vet` / `fmt` | 静态检查 / 格式化 |
| `cross-compile` | 跨平台构建到 `./dist/` |

---

## 集成目标

### 已实现

| 平台 | 接入方式 | 文档 |
|---|---|---|
| Claude Code | `make install-skills` → `~/.claude/skills/myonto/` | `skills/myonto/SKILL.md` |

### 适配各 Agent 平台的 Skills 路径

| 平台 | 路径约定 |
|---|---|
| Claude Code | `~/.claude/skills/<name>/SKILL.md` |
| Codex | `<codex-config>/skills/<name>/SKILL.md` |
| 其他（通用 Anthropic Skills 标准） | `<config>/skills/<name>/SKILL.md` |

### 未来可考虑的集成

- **MCP server**：在二进制里加 stdio 模式，启动时同时作为 MCP server（暴露工具）。参考 [`tggo/goRDFlib` 思路](https://github.com/tggo/goRDFlib)——但目前 OWL/RDF 的 MCP 生态不成熟，按需引入。
- **Anthropic Prompt Caching**：prompt 字段已暴露，未来 ai 命令可加 `cache_control` 让重复 prompt 走 Anthropic 缓存。
- **OpenAI Function Calling**：`myonto list` 等命令的 JSON 输出已经稳定，包装一份 OpenAPI 描述即可让 OpenAI function calling 直接发现。

---

## 故障排查

### `go mod download` 卡住 / 超时

GOPROXY 没配国内镜像。运行 `make setup`，或手动：

```bash
go env -w GOPROXY=https://goproxy.cn,direct
go env -w GOSUMDB=sum.golang.google.cn
```

### `make link` 后 `myonto: command not found`

`~/.local/bin` 不在 PATH。`echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.zshrc && source ~/.zshrc`

### `myonto tui` 在远程 SSH 报"需要 TTY"

正常行为。远程 shell 没分配 TTY。TUI 模式必须在真终端跑；远程管理用 CLI 命令即可。

### `make install-skills` 后 Claude Code 看不到

重启 Claude Code 会话（Agent 启动时扫描 `~/.claude/skills/`）。验证：

```bash
ls ~/.claude/skills/myonto/SKILL.md  # 应存在
```

### LLM 调用报"fingerprint 不匹配"

换机器/重装系统导致 token 无法解密。在新机器上重新 `myonto config llm set-key` 即可。

### 阿里云 Go 镜像源 `https://mirrors.aliyun.com/golang/` 根目录 404

正常——根目录无索引，但**具体文件 URL**（如 `go1.26.4.darwin-arm64.tar.gz`）是可达的。直接指定版本号下载。

---

## 引用

- [`README.md`](../README.md) 顶部「Agent 索引」— Agent 启动时加载的精简版
- [`README.md`](../README.md) — 项目总览
- [`docs/data-model.md`](data-model.md) — RDF/OWL 数据模型
- [`docs/turtle.md`](turtle.md) — Turtle 格式速查
- [`docs/interop.md`](interop.md) — 与外部 RDF 工具互通
- [`docs/faq.md`](faq.md) — 常见问题
