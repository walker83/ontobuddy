# Agent 调用约定

本文档是 [`AGENTS.md`](../AGENTS.md) 提到的"详细约定"承载文件——Agent 在遇到错误、需要判断安全/性能边界时**按需加载**。

## 1. 全局行为

- 写命令（`add` / `link` / `unlink` / `reason -a` / `ai -a` / `set-key`）默认**立即持久化**。
- 读命令（`list` / `search` / `show`）默认**只读**。
- 非 TTY 优雅降级：TUI 报错，普通命令正常执行。
- 退出码：0=成功、1=错误。

## 2. `--json` 全局 flag

任何命令前都接受 `--json`，开启后输出结构化 JSON 替代人类可读文本。

| 命令 | JSON 顶层字段 |
|---|---|
| `list` | `count, entities[]` |
| `search` | `count, entities[]`（多一个 `match_kind`） |
| `entity show` | `entity, count, triples[]` |
| `reason` | `saturated, derived[], will_apply, applied?` |
| `ai summarize` | `entity, entity_iri, triples, prompt, llm_output, will_apply` |
| `ai extract` | `input_text, prompt, llm_output, turtle, will_apply` |
| `ai suggest-relations` | `entity, prompt, llm_output, turtle, will_apply` |
| `ai qa` | `question, prompt, answer` |

**Agent 应优先用 `--json` 解析**——schema 稳定，文本输出可能改格式。

## 3. 写操作的二次确认

| 命令 | 默认行为 | 需要加什么 flag |
|---|---|---|
| `entity rm` | 拒绝（要确认） | `-f` |
| `reason` | dry-run（不物化） | `-a` |
| `ai summarize` / `ai extract` / `ai suggest-relations` | dry-run（不写回） | `-a` |

## 4. API key 安全

- **明文 API key 永不入盘**——`myonto config llm set-key` 走交互式隐藏输入 → AES-256-GCM 加密 → 存 `api_key_token` 字段。
- 加密密钥：机器指纹（macOS IOPlatformUUID / Linux `/etc/machine-id`）+ scrypt KDF。
- 换机器/重装系统后 token 无法解密，提示用户重跑 `set-key`。
- 错误信息**不会**包含解密后的 key；HTTP 错误体自动截断到 512 字节。

## 5. 错误信号表

| 错误 | 含义 | Agent 行动 |
|---|---|---|
| `未找到实体 xxx` | 名词解析失败 | 用 `list --json` 查确切名字 |
| `LLM 未配置` | `[llm]` 段缺失 | 提示用户跑 `set-key` |
| `指纹不匹配` 或 `解密失败（换机器后...）` | token 在别的机器上 | 提示用户重跑 `set-key` |
| `（无匹配）` / `（本体为空）` | 正常空结果 | 不算失败 |
| `（无新推导，原本体已饱和）` | 推理无新产出 | 不算失败 |

## 6. 调用前检查清单

1. `myonto --version` —— 确认二进制可用
2. `ls .myonto.toml` 或 `myonto list` —— 确认本目录有本体
3. 必要时 `myonto config llm show` —— 确认 LLM 已配

## 7. 性能 / 规模建议

- 本体规模 < 10 万三元组时所有命令亚秒级响应。
- AI 命令走 60s 超时。
- 不要每条操作都跑 `reason -a`——只在有"补全类型"需求时跑。

## 8. 安全模型

myonto **防**：
- 误把 `.myonto.toml` commit 到公开 git 仓库
- 配置文件泄漏到备份、镜像、聊天记录

myonto **不防**：
- 能跑用户级代码的攻击者（他们能拿到机器指纹）
- 物理拿到机器 + 知道登录密码的人
- 取得 sudo 权限的攻击者

**不是企业级 KMS 替代品。**

## 9. 仓库协作规范（修改代码时）

| 规则 | 说明 |
|---|---|
| 永远不要 commit 真实 API key | 即使用户提供 |
| 改完跑 `make all`（fmt + vet + test + build） | 必须全过 |
| 新功能加单测 | 至少一个 happy path + 一个 error path |
| 改 CLI 接口同步更新 `skills/myonto/SKILL.md` 和 `references/commands.md` | 否则 LLM 调用会过期 |
| 安全/推理/协议改动的 PR 在 commit msg 标注 ⚠️ 安全 | 提醒 reviewer |
