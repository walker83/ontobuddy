# myonto Command Reference

完整命令速查。规则：

- **所有 `entity <name>`、`link <s> <p> <o>` 等处的 name/s/p/o**：支持 3 种形式
  - 裸 local name（推荐）：`isaac-newton`
  - 前缀形式：`ex:isaac-newton`
  - 完整 IRI：`<http://example.org/isaac-newton>` 或 `http://...`
- **所有 read 类命令**（`list`/`search`/`show`）支持 `--json` 全局 flag
- **写操作默认就动本体**，不需要 `--json`

---

## `bin/myonto init`

初始化本体库（生成 `.myonto.toml` + `ontology.ttl`）。**TUI 向导优先**，无 TTY 走参数模式。

```bash
bin/myonto init                                  # TUI 向导
bin/myonto init --wizard                         # 强制向导
bin/myonto init --no-wizard                      # 强制参数模式
bin/myonto init -i http://myorg.org/ -p my       # 自定义 IRI 和前缀
bin/myonto init --data-file my.ttl               # 自定义数据文件
bin/myonto init -f                               # 覆盖已存在配置
```

---

## `bin/myonto entity` — 实体管理

| 子命令 | 说明 |
|---|---|
| `add <name>` | 添加个体 |
| `add-class <name>` | 添加类 |
| `list` | 列出所有实体 |
| `show <name>` | 查看某实体的全部三元组 |
| `edit <name>` | 修改 |
| `rm <name>` | 删除 |

### `myonto entity add <name>`

```bash
bin/myonto entity add isaac-newton
bin/myonto entity add isaac-newton -t Person -d "英国数学家、物理学家"
bin/myonto entity add isaac-newton -g 重要 -g 历史人物   # 多个标签
```

| Flag | 说明 |
|---|---|
| `-t CLASS` | 类型（可省略） |
| `-d TEXT` | 描述（写入 rdfs:comment） |
| `-g TAG` | 标签（可多次） |

### `myonto entity add-class <name>`

```bash
bin/myonto entity add-class Person
bin/myonto entity add-class Scientist -p Person -d "科学家（Person 的子类）"
```

| Flag | 说明 |
|---|---|
| `-p CLASS` | 父类（建立 subClassOf） |
| `-d TEXT` | 描述 |

### `myonto entity list [-t TYPE] [-g TAG]`

```bash
bin/myonto entity list                          # 全部
bin/myonto entity list -t Person                # 只看 Person
bin/myonto entity list -g 重要                  # 只看带"重要"标签的
bin/myonto list --json                          # JSON 输出供程序消费
```

### `myonto entity show <name>`

```bash
bin/myonto entity show isaac-newton
bin/myonto entity show aristotle --json
```

### `myonto entity edit <name>`

```bash
bin/myonto entity edit isaac-newton -d "新描述"
bin/myonto entity edit isaac-newton -t Scientist -t Mathematician
```

### `myonto entity rm <name>`

```bash
bin/myonto entity rm leibniz -f
```

---

## `myonto link / unlink` — 关系管理

### `myonto link <s> <p> <o>`

```bash
bin/myonto link isaac-newton knows leibniz
bin/myonto link isaac-newton bornIn 1643 -l             # -l = 宾语当字面量
bin/myonto link newton calculus invented --label "发明了"
```

| Flag | 说明 |
|---|---|
| `-l` | 宾语当字面量（而非 IRI 实体） |
| `--label TEXT` | 给谓词本身加 rdfs:label（语义化自定义关系） |

### `myonto unlink <s> <p> <o>`

```bash
bin/myonto unlink newton knows leibniz
bin/myonto unlink newton knows "*" -a   # -a 删 newton 所有 knows 关系
```

| Flag | 说明 |
|---|---|
| `-l` | 宾语按字面量匹配 |
| `-a` | 宾语通配，删该主语+谓词的所有三元组 |

---

## `myonto search <keyword> [-t TYPE]`

在 local name / `rdfs:label` / `rdfs:comment` 上做子串匹配。

```bash
bin/myonto search newton
bin/myonto search 数学 -t Scientist
bin/myonto search newton --json
```

---

## `bin/myonto list`

`entity list` 的快捷别名，参数一致。

---

## `myonto reason [-a] [-n N]`

RDFS/OWL 轻量规则推理。规则：

1. subClassOf 传递闭包
2. 类型继承（x a A, A ⊑ B ⟹ x a B）
3. subPropertyOf 传递 + 继承
4. owl:TransitiveProperty
5. owl:SymmetricProperty
6. owl:inverseOf

```bash
bin/myonto reason           # dry-run：展示推导出的新三元组
bin/myonto reason -a        # 物化到本体
bin/myonto reason -n 50     # 最多展示 50 条
```

---

## `myonto graph [-o FILE] [-O] [--include-pred PRED] [--include-labels KEYWORD]`

生成交互式力导向图 HTML（vis-network）。

```bash
bin/myonto graph                                          # 写 ontology-graph.html，自动开浏览器
bin/myonto graph -o my-graph.html                         # 自定义输出
bin/myonto graph -O                                       # 不开浏览器（仅生成）
bin/myonto graph --include-pred knows --include-pred likes  # 只画这些关系
```

| Flag | 说明 |
|---|---|
| `-o FILE` | 输出文件路径 |
| `-O` | 自动开浏览器 |
| `--include-pred PRED` | 只画这些谓词（可多次） |
| `--include-labels KEYWORD` | 节点按 label 关键词过滤 |

---

## `bin/myonto ai` — LLM 辅助（默认 dry-run）

| 子命令 | 说明 |
|---|---|
| `summarize <entity>` | 归纳实体的所有三元组 |
| `extract "<text>"` | 从自然语言文本抽取实体+关系（生成 Turtle 草稿） |
| `suggest-relations <entity>` | 基于上下文建议新关系 |
| `qa "<question>"` | 基于本体的问答 |

```bash
bin/myonto ai summarize aristotle
bin/myonto ai summarize aristotle -a      # 写到 rdfs:comment
bin/myonto ai extract "苏格拉底是柏拉图的老师" -a
bin/myonto ai suggest-relations aristotle
bin/myonto ai qa "牛顿认识谁？"
```

| Flag | 说明 |
|---|---|
| `-a / --apply` | 物化输出到本体（默认仅展示） |

---

## `myonto config llm` — LLM 配置管理

```bash
bin/myonto config llm list-providers        # 看 4 个内置供应商（alibaba-coding / openai / deepseek / ollama）
bin/myonto config llm set-key              # 交互式选供应商、隐藏输入 key、自动加密
bin/myonto config llm set-key alibaba-coding -m glm-5
bin/myonto config llm show                 # 显示当前配置（key 隐藏为「已加密存储」）
bin/myonto config llm test                 # 发送测试请求验证连通
```

API key 加密流程：用户输入明文 → 机器指纹派生主密钥 → scrypt → AES-256-GCM → 密文写入 `api_key_token` 字段。**明文永不入盘**。

---

## `bin/myonto tui`

进入 TUI 交互模式（主菜单 → 选动作 → 子表单）。无 TTY 不可用。

```bash
bin/myonto tui
```

适合人类用户；CLI 模式适合 LLM 调用。

---

## `bin/myonto version`

显示版本号。
