# AGENTS.md

本文件给 AI Agent（Claude Code / Codex / 其他自动工具）启动时加载。详细工作流见 `docs/agent-contract.md`；按需查阅其他文档。

**关键事实**：

- 仓库：`https://github.com/walker83/ontobuddy.git`（GitHub 开源）
- 内网镜像：请用你自己的 Gitea 实例（如有）
- 二进制：全局 `~/.local/bin/myonto`（`make link`），本地 `./bin/myonto`
- 全局 flag：`--json`（list / search / show / reason / ai 都支持）
- 写操作需 `-a` / `-f` 显式确认；`ai` 默认 dry-run
- API key 永不进盘：明文走 `set-key` 交互 → AES-256-GCM 加密存 `api_key_token`
- Skills 安装：`make install-skills` → `~/.claude/skills/myonto/`

**协作规范**：永远不 commit 真实 API key；改完代码跑 `make all` 全过；改 CLI 接口同步更新 `skills/myonto/SKILL.md` 和 `references/commands.md`；安全/推理/协议改动的 commit msg 标注 ⚠️ 安全。
