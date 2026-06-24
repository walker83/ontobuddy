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
7. rdfs:domain（x P y, P domain C ⟹ x a C）
8. rdfs:range（x P y, P range C ⟹ y a C，仅 IRI）

```bash
bin/myonto reason           # dry-run：展示推导出的新三元组
bin/myonto reason -a        # 物化到本体
bin/myonto reason -n 50     # 最多展示 50 条
```

---

## `myonto check` — 一致性检查

检查本体是否违反自身声明的约束（如 `owl:disjointWith`）。与 `reason` 不同：`reason` 推导隐含成立的三元组，`check` 报告**不应存在的矛盾**（不可物化，是错误报告）。

```bash
bin/myonto check                  # 跑检查，打印发现的问题
bin/myonto check --strict         # 有 error 级问题时退出码 1（CI 门禁）
bin/myonto check --json           # 结构化输出
```

当前支持：`owl:disjointWith` 违规检测（含经 `subClassOf` 继承的隐式冲突）。

---

## `myonto closure <entity> -p <predicate>` — 传递闭包查询

算某实体沿指定谓词的传递闭包（不物化，纯查询）。典型用途：**影响面分析**——"改了 store 包，间接影响哪些？"。

```bash
bin/myonto closure cmd -p dependsOn           # cmd 依赖的全部包（正向）
bin/myonto closure rdf -p dependsOn -r        # 谁依赖了 rdf（反向）
bin/myonto closure alice -p reportsTo -d 2    # 限制 2 跳
bin/myonto closure cmd -p dependsOn --json    # 结构化输出
```

| Flag | 说明 |
|---|---|
| `-p PRED` | 沿此谓词展开（必填） |
| `-r` | 反向遍历（宾语 → 主语） |
| `-d N` | 最大深度（0=无限） |

注意：在**原始边集**上 BFS，深度反映真实跳数。如需含隐式边的闭包，先 `reason -a` 物化。

---

## `myonto path <from> <to>` — 最短路径

找两实体间的最短路径（BFS）。典型用途：**关系解释**——"A 和 B 怎么关联上的？"。

```bash
bin/myonto path alice bob                   # 任意谓词的最短路径
bin/myonto path alice bob -p knows          # 只走 knows 关系
bin/myonto path alice bob --json            # 结构化输出
```

找不到路径时退出码 1（便于脚本判断连通性）。

---

## `myonto query` — 轻量查询引擎（SPARQL 子集）

三元组模式匹配 + GROUP BY / COUNT / Top-N。**自动包含推理推出的隐式知识**（这是本体驱动分析的核心价值）。

```bash
bin/myonto query -w "?s a ex:Person"                          # 列出所有 Person（含子类实例）
bin/myonto query -w "?s ex:bornIn ?o" -g "?o" -c              # 按出生地分组计数
bin/myonto query -w "?s ex:knows ?o" -g "?o" -c -n 5          # Top 5 社交达人
bin/myonto query -w "?s ex:knows ?o" -w "?o ex:knows ex:carol" # JOIN：谁认识 carol 的朋友
bin/myonto query -w "?s a ex:Scientist" --json                # 结构化输出
```

| Flag | 说明 |
|---|---|
| `-w 'S P O'` | 三元组模式（`?var` 为变量，`a` = rdf:type，多 `-w` 做 JOIN） |
| `-g VAR` | 按变量分组（`?` 前缀可选） |
| `-c` | COUNT 每组元素数 |
| `-n N` | Top-N |
| `-d` | 结果去重 |

---

## `myonto serve` — Web UI 服务器

启动交互式 Web UI，提供图谱可视化、规则管理、推理执行、一致性检查。

```bash
bin/myonto serve                                  # 默认 localhost:7399
bin/myonto serve --port 9090                      # 自定义端口
bin/myonto serve --host 0.0.0.0                   # 监听所有接口
bin/myonto serve -O                               # 自动打开浏览器
```

| Flag | 说明 |
|---|---|
| `--host` | 监听地址（默认 localhost） |
| `--port` | 端口号（默认 7399） |
| `-O` | 自动打开浏览器 |

**端口冲突处理**：如果端口被占用且不是 myonto 自己的进程，自动尝试下一个端口（最多 10 次）。如果是 myonto 自己已运行，提示已存在并退出。

**自定义 HTML 模板**：
- 项目级：`.myonto/web/index.html`
- 全局级：`~/.config/myonto/web/index.html`
- 内置默认：`internal/web/ui/index.html`

修改模板后重启服务即可生效。

Web UI 标签页：
- **图谱** — vis-network 力导向图，节点按类型着色
- **规则** — 查看全部推理规则（ID/名称/分类/说明），支持启用/禁用
- **推理** — 执行推理，展示每条规则的产出统计和推导三元组
- **检查** — 一致性检查，显示问题严重度/规则/详情

API 端点：
- `GET /api/rules` — 规则列表
- `PUT /api/rules/:id` — 启用/禁用规则
- `POST /api/reason` — 执行推理
- `POST /api/check` — 一致性检查
- `GET /api/graph` — 图数据
- `GET /api/triples?s=&p=&o=` — 三元组查询
- `GET /api/stats` — 统计信息

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
