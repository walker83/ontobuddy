# myonto Skill — 安装与运行说明

> 本文件给 **AI Agent**（Claude Code / Codex 等）加载 skills 后，告知具体怎么调用本 skill。
> **SKILL.md**（frontmatter + 触发描述）决定"何时用"；本文件决定"怎么用"。

---

## 1. skill 是自包含的

安装后 `~/.claude/skills/myonto/` 目录长这样：

```
~/.claude/skills/myonto/
├── SKILL.md              # 触发描述 + 命令示例
├── references/
│   └── commands.md       # 完整命令速查
├── bin/
│   └── myonto            # ⭐ 9MB 静态二进制（由 make install-skills 从 ./bin/myonto 复制）
└── INSTALLATION.md       # 本文件
```

**所有命令用相对路径 `bin/myonto <subcommand>` 调用**——**不要**依赖系统 PATH。

---

## 2. 典型调用流程

Agent 加载本 skill 后，按以下流程工作：

```bash
# 1. 切到用户的工作目录（myonto 操作的是 cwd 的 ontology.ttl）
cd <用户的项目目录>

# 2. 用相对路径调二进制
./<skill_dir>/bin/myonto list --json          # 列出所有实体
./<skill_dir>/bin/myonto search <关键词> --json  # 搜索
./<skill_dir>/bin/myonto entity add <name> -t <class> -d "<desc>"  # 添加
# ... 等等
```

> **不要**写 `myonto xxx`（会依赖系统 PATH，可能找不到）；**不要**写 `~/.local/bin/myonto`（不是 skill 自己的二进制）。
> **总是**用 `bin/myonto` 相对路径，相对于 skill 所在目录。

### 找 skill 所在目录的方法

Agent 启动时通常知道自己的 skills 路径。如果不确定：

```bash
# Claude Code 默认
SKILL_DIR="$HOME/.claude/skills/myonto"
# Codex / 其他平台见各自文档
```

skill 文档中说"使用前 `cd` 进工作目录"——`cd` 之后 cwd **是**用户目录，skill 目录路径仍然固定。

---

## 3. 首选使用 `--json` 标志

skill 设计的核心是**机器可解析**——所有读命令（`list` / `search` / `show` / `reason` / `ai`）都支持 `--json`：

```bash
bin/myonto list --json       # 解析 JSON
bin/myonto ai summarize <name> --json
```

详细 schema 见 `SKILL.md` 的 "Output Format" 章节。

---

## 4. 安全约束（必须遵守）

| 操作 | 默认 | 需加 flag |
|---|---|---|
| 删除实体 | 拒绝 | `-f` |
| 推理物化 | dry-run | `-a` |
| AI 写回本体 | dry-run | `-a` |
| 写 LLM key | 加密存盘 | 交互式输入（不要 `--key` 明文传，会进 shell history） |

**Agent 应**：
- 默认跑 `bin/myonto ai summarize <name>`（不加 `-a`），把 LLM 输出展示给用户**确认**后才加 `-a` 重跑
- 不准 `rm -rf` 任何目录
- 改完本体后建议用户 `git add ontology.ttl && git diff` 检查变化

---

## 5. 故障排查

| 现象 | 排查 |
|---|---|
| `Permission denied` | `chmod +x ~/.claude/skills/myonto/bin/myonto` |
| `myonto: command not found` | 说明你写的是 `myonto` 不是 `bin/myonto`；用相对路径 |
| `未找到 .myonto.toml` | 用户工作目录还没初始化，让用户跑 `bin/myonto init` |
| `LLM 未配置` | 用户没设 key，提示跑 `bin/myonto config llm set-key` |
| `fingerprint 不匹配` | api_key_token 在别的机器上生成，提示重新 `set-key` |

---

## 6. 何时不用这个 skill

- 用户用的是 Protégé / Apache Jena / 其他 RDF 工具 → 直接用那些工具，不调 myonto
- 用户的本体不在 `ontology.ttl` 而在别的文件 → myonto 用 `data_file` 配置，但默认假设是 `ontology.ttl`
- 用户只想查询 / 检索（不需要 CRUD） → 用 SPARQL endpoint 更专业

---

## 7. 引用

- `SKILL.md` — 触发描述 + 命令示例
- `references/commands.md` — 完整命令速查
- `https://github.com/walker/myonto` — 项目主页（README 含人类版详尽文档）
