
## 这是什么

myonto 是一个命令行工具，帮你把脑子里的知识结构化成一个**本体（ontology）**：
- 定义**类**（Person、Project、Idea…）
- 添加**实体**（isaac-newton、my-project-2024…）
- 建立实体之间的**关系**（knows、partOf、inspired…）

所有数据遵循 W3C RDF 标准，存为 `.ttl` 文件。你可以 git 跟踪它的变化、用文本编辑器直接修改、或导入到 Protégé / Apache Jena / rdflib 等标准工具。

## 3 种使用模式

myonto 同时支持 3 种入口，**按使用场景选**：

| 模式 | 入口 | 适合 |
|---|---|---|
| **TUI 模式** | `myonto tui` | 人类用户，菜单+表单式交互 |
| **CLI 模式** | `myonto <子命令> [flags]` | 老手、脚本、远程登录 |
| **Skills 模式** | 被 Claude Code / Codex 等 Agent 加载 | LLM 自然语言调用 |

### 1. TUI 模式（人类友好）

```bash
myonto tui          # 进入主菜单（list/search/add/link/reason/graph/ai/setup）
```

主菜单用 huh（charmbracelet）渲染，支持上下键选择 + 回车确认。无需背任何命令。

### 2. CLI 模式（老手/脚本）

```bash
myonto init
myonto entity add isaac-newton -t Person -d "英国数学家、物理学家"
myonto link isaac-newton knows leibniz
myonto list                          # 人类可读
myonto list --json                   # 程序可读
```

`--json` 是**全局 flag**，所有 read 类命令（`list`/`search`/`show`）都支持。

### 3. Skills 模式（LLM 调用）

```bash
make install-skills   # 把 skills/myonto/ 复制到 ~/.claude/skills/myonto/
```

之后 Claude Code / Codex / 其他 Agent 启动时会**自动加载** myonto 的 Skill。LLM 看到用户说"记住 X 是个 Y"、"列出所有 Person"、"给 aristotle 总结一下"等请求时，**自动调用 `myonto` 命令**。

SKILL 描述（Anthropic 协议唯一触发入口）：

> "CLI for managing a personal RDF/OWL ontology (entity-relationship knowledge graph stored as Turtle). Use when creating/editing/listing typed entities, linking them with predicates, querying or searching the graph, running RDFS/OWL inference, generating interactive visualizations, or invoking LLM-assisted ontology maintenance. Trigger on 'remember', 'what do I know about', 'add an entity', ..."

详细命令参考在 `skills/myonto/references/commands.md`，安装后 LLM 也会读。

## 3 分钟上手（向导式）

```bash
mkdir my-knowledge && cd my-knowledge
myonto init   # 首次运行会自动启动 TUI 向导，问几个问题完成设置
```

向导会问你：
1. 命名空间 IRI（用默认 `http://example.org/` 就好）
2. 命名空间前缀（默认 `ex`）
3. 是否现在配置 LLM（选「是」则继续选供应商、隐藏输入 API key）
4. 是否复制示例本体（`philosophers.ttl`，方便试玩）

不用背任何参数。已有配置想改？跑 `myonto config setup` 重新走一遍。

在非 TTY 环境（CI、管道、远程 shell）自动回退到参数模式，`myonto init -i ... -p ...` 仍可用。

---

## 快速上手（3 分钟）

### 第 1 步：安装

```bash
git clone <本项目> MyOntopo && cd MyOntopo
make setup      # 配置 Go 国内镜像（首次必做）
make link       # 编译并创建全局符号链接
```

完成后，`myonto` 命令在任意目录都可用。验证：

```bash
myonto version
# 输出：myonto dev (P0+P1)
```

> 如果提示找不到命令，把 `~/.local/bin` 加进 PATH：
> ```bash
> echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.zshrc && source ~/.zshrc
> ```

### 第 2 步：创建你的第一个本体

```bash
mkdir my-knowledge && cd my-knowledge
myonto init
```

输出：
```
已初始化本体库：/Users/you/my-knowledge
  配置文件：.myonto.toml
  数据文件：ontology.ttl
  命名空间：ex <http://example.org/>
```

### 第 3 步：定义类、添加实体、建立关系

```bash
# 定义一个类
myonto entity add-class Person -d "一个人类"
myonto entity add-class Scientist -p Person -d "科学家（Person 的子类）"

# 添加实体
myonto entity add isaac-newton -t Scientist -d "英国数学家、物理学家"
myonto entity add leibniz     -t Scientist -d "德国数学家"

# 建立关系
myonto link isaac-newton knows leibniz
myonto link leibniz     knows isaac-newton
myonto link isaac-newton bornIn 1643 -l    # -l 表示宾语是字面量

# 浏览
myonto list
myonto search newton
myonto entity show isaac-newton
```

---

## 核心概念

### 实体（Entity）vs 类（Class）

- **类**是"种类"，如 `Person`、`Scientist`、`Project`。类之间可以继承（`Scientist` 是 `Person` 的子类）。
- **实体**是具体的"东西"，如 `isaac-newton`、`leibniz`。每个实体可以属于一个或多个类。

### 关系（Relation / Triple）

myonto 用 **RDF 三元组** 描述一切：

```
<主语>  <谓词>  <宾语>
isaac-newton  knows  leibniz
isaac-newton  bornIn  1643
```

`myonto link <主语> <谓词> <宾语>` 就是在添加一条三元组。谓词可以是任意你自定义的关系名（`knows`、`partOf`、`inspired`、`worksOn`…）。

### 命名空间（Namespace）

每个实体都有一个全局唯一的 IRI（类似 URL）。myonto 默认用 `http://example.org/` 作为基础，所以 `isaac-newton` 的完整 IRI 是 `http://example.org/isaac-newton`。

写 Turtle 时用前缀缩写：`ex:isaac-newton`。你可以在 `myonto init -i <你的IRI>` 时换成自己的命名空间。

---

## 命令参考

> 任何命令加 `-h` 或 `--help` 看完整帮助，例如 `myonto entity add -h`。

### `myonto init`
在当前目录初始化本体库。
```bash
myonto init [-i BASE_IRI] [-p PREFIX] [--data-file FILE] [-f]
```
- `-i`：命名空间基础 IRI，默认 `http://example.org/`
- `-p`：命名空间前缀，默认 `ex`
- `-f`：已存在时强制覆盖

### `myonto entity` — 实体管理

| 子命令 | 说明 | 示例 |
|---|---|---|
| `add` | 添加个体 | `myonto entity add 牛顿 -t Person -d "物理学家" -g 重要 -g 历史` |
| `add-class` | 添加类 | `myonto entity add-class Scientist -p Person` |
| `list` | 列出实体 | `myonto entity list -t Person` |
| `show` | 查看详情 | `myonto entity show isaac-newton` |
| `edit` | 修改 | `myonto entity edit isaac-newton -d "新描述"` |
| `rm` | 删除 | `myonto entity rm leibniz -f` |

**`entity add` 参数：**
- `NAME`（必填）：实体名，会被 slug 化（`Isaac Newton` → `isaac-newton`）
- `-t CLASS`：类型，可省略
- `-d TEXT`：描述
- `-g TAG`：标签，可重复（`-g 重要 -g 历史`）

### `myonto link` — 建立关系
```bash
myonto link <主语> <谓词> <宾语> [-l] [--label TEXT]
```
- `<谓词>`：任意关系名，如 `knows` / `partOf` / `inspired`
- `-l`：宾语当字面量（字符串），而非实体 IRI
- `--label`：给谓词附人类可读名

示例：
```bash
myonto link isaac-newton knows leibniz           # 实体 → 实体
myonto link isaac-newton bornIn 1643 -l          # 实体 → 字面量
myonto link newton calculus invented --label "发明了"
```

### `myonto unlink` — 删除关系
```bash
myonto unlink <主语> <谓词> <宾语> [-l] [-a]
```
- `-a`：通配宾语，删除该 `<主语> <谓词>` 的所有三元组

### `myonto search` — 全文搜索
```bash
myonto search <关键词> [-t CLASS]
```
在 local name / `rdfs:label` / `rdfs:comment` 上做子串匹配。

### `myonto list` — 列出全部
`myonto entity list` 的快捷别名。

---

## 典型工作流

### 场景 1：建立读书笔记本体

```bash
mkdir books && cd books
myonto init -i "http://mybooks.org/" -p book

myonto entity add-class Book -d "一本书"
myonto entity add-class Author -d "作者"
myonto entity add-class Topic -d "主题"

myonto entity add-Class Philosophy    # 主题类
myonto entity add-class Philosophy -t Topic

myonto entity add thinking-fast-slow -t Book -d "《思考，快与慢》"
myonto entity add daniel-kahneman -t Author -d "丹尼尔·卡尼曼，诺贝尔经济学奖得主"

myonto link thinking-fast-slow writtenBy daniel-kahneman
myonto link thinking-fast-slow about psychology

myonto search kahneman
myonto entity show thinking-fast-slow
```

### 场景 2：项目管理本体

```bash
mkdir projects && cd projects
myonto init -i "http://myproj.org/" -p proj

myonto entity add-class Project -d "项目"
myonto entity add-class Task -p Project -d "任务（项目的子类）"
myonto entity add-class Person

myonto entity add myonto-app -t Project -d "个人本体管理工具"
myonto entity add walker -t Person -d "开发者"

myonto link myonto-app hasOwner walker
myonto link myonto-app status "进行中" -l
myonto link myonto-app deadline "2024-12-31" -l
```

### 场景 3：导入示例本体

本项目 `examples/` 目录提供了可运行的样例：

```bash
cp examples/philosophers.ttl .
myonto list
myonto search aristotle
```

---

## 数据文件长什么样

所有数据存在 `ontology.ttl`，是标准 Turtle 格式，例如：

```turtle
@prefix ex: <http://example.org/> .
@prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .
@prefix rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#> .

ex:Person rdf:type rdfs:Class ;
    rdfs:label "Person" ;
    rdfs:comment "一个人类" .

ex:Scientist rdfs:subClassOf ex:Person ;
    rdfs:comment "科学家" .

ex:isaac-newton rdfs:label "isaac-newton" ;
    rdfs:comment "英国数学家、物理学家" ;
    rdf:type ex:Scientist ;
    ex:knows ex:leibniz .
```

可以直接用 `cat` 查看、用 git diff 跟踪变化、用编辑器手动修改。

---

## 配置文件 `.myonto.toml`

```toml
base_iri  = "http://example.org/"   # 本地命名空间基础 IRI
data_file = "ontology.ttl"           # 数据文件名
prefix    = "ex"                     # 命名空间前缀

[llm]                                # AI 辅助功能（ai summarize/extract/suggest/qa）
base_url = "https://api.deepseek.com/v1"   # 任何 OpenAI 兼容端点
api_key  = "sk-..."                         # 也可读 $MYONTO_LLM_API_KEY
model    = "deepseek-chat"                 # 也可读 $MYONTO_LLM_MODEL
```

> 没配置 `[llm]` 时 `ai` 命令会清晰报错，不影响其他命令使用。

---

## 开发

```bash
make setup      # 配置 Go 镜像（首次）
make build      # 编译到 ./bin/myonto
make test       # 跑单测
make vet        # 静态检查
make fmt        # 格式化
make link       # 全局安装（符号链接，重编译后自动生效）
make unlink     # 卸载符号链接
make cross-compile   # 交叉编译多平台
```

深入文档见 [`docs/`](docs/)：
- [数据模型与命名空间](docs/data-model.md)
- [Turtle 格式速查](docs/turtle.md)
- [与外部 RDF 工具互通](docs/interop.md)
- [FAQ](docs/faq.md)

---

## 技术栈

| 层 | 选型 |
|---|---|
| 语言 | Go 1.22+ |
| CLI | [Kong](https://github.com/alecthomas/kong) |
| RDF 读写 | 自实现轻量 Turtle 读写层（`internal/rdf`），覆盖 W3C Turtle 子集 |
| 配置 | [go-toml/v2](https://github.com/pelletier/go-toml) |

## 工具链地址

> 完整的 Agent 元信息（含上游 Go 生态、依赖包 URL、调用约定、安全模型）见本项目根目录的 **[`AGENTS.md`](AGENTS.md)**。
> 本节是给人类读的快速概览。

### 本项目

| 资源 | 地址 |
|---|---|
| Gitea 仓库 | https://github.com/walker83/ontobuddy |
| Git 远程 | `https://github.com/walker83/ontobuddy.git` |
| 二进制（全局） | `~/.local/bin/myonto`（`make link` 创建） |
| 二进制（本地） | `./bin/myonto` |
| Skills 安装路径 | `~/.claude/skills/myonto/` |

### 上游 Go 生态

| 资源 | 地址 |
|---|---|
| Go 工具链 | `https://mirrors.aliyun.com/golang/` |
| Go 模块代理 | `https://goproxy.cn,direct` |
| 备用模块代理 | `https://mirrors.aliyun.com/goproxy/` |
| 校验和 | `sum.golang.google.cn` |

### 直接依赖

| 包 | 版本 | 用途 |
|---|---|---|
| `github.com/alecthomas/kong` | v1.15.0 | CLI 解析 |
| `github.com/charmbracelet/huh` | v1.0.0 | TUI 向导 |
| `github.com/pelletier/go-toml/v2` | v2.3.1 | TOML 解析 |
| `golang.org/x/crypto` | latest | scrypt（KDF） |
| `golang.org/x/term` | latest | 终端密码隐藏 |

### 集成目标

| 平台 | 接入点 |
|---|---|
| Claude Code | `make install-skills` → 复制到 `~/.claude/skills/myonto/` |
| 其他 Agent | 按各平台规范放置 skills |

---

## 路线图

- ✅ **P0** 骨架：init + Turtle 读写 + Store
- ✅ **P1** CRUD：entity / link / unlink / search
- ✅ **P2** 推理 + 可视化
  - `reason`（RDFS/OWL 轻量规则推理：subClassOf 传递、类型继承、TransitiveProperty、SymmetricProperty、inverseOf）
  - `graph`（vis-network 力导向图 HTML，自动打开浏览器，支持 `--include-pred` 过滤）
- ✅ **P3** AI 辅助（需配置 `[llm]`，默认 dry-run）
  - `ai summarize <entity>`：LLM 归纳实体的所有三元组
  - `ai extract "<文本>"`：从自然语言抽取实体与关系（生成 Turtle 草稿）
  - `ai suggest-relations <entity>`：基于上下文让 LLM 建议可能的新关系
  - `ai qa "<问题>"`：基于本体的问答
- ✅ **P4** 安全 + 兼容
  - API key 加密（机器指纹 + scrypt + AES-256-GCM，加密 token 存盘）
  - 4 个内置 LLM provider（alibaba-coding / openai / deepseek / ollama）
  - Anthropic Messages 协议（阿里云 DashScope 兼容）
  - `myonto config llm set-key/show/test/list-providers` 子命令
- ✅ **P5** TUI 向导
  - `myonto init` 首次启动 huh 向导（多步表单 + 隐藏 key + 动态跳过）
  - `myonto config setup` 重走向导
- ✅ **P6** 三种使用模式 + LLM 集成
  - **CLI 模式**：老手 / 脚本使用
  - **TUI 模式**：`myonto tui` 主菜单（list/search/add/link/reason/graph/ai/setup/quit）
  - **Skills 模式**：Anthropic Skills 标准（`make install-skills` 安装到 `~/.claude/skills/myonto/`）
  - **`--json` 全局 flag**：`list`/`search`/`show`/`reason`/`ai` 全部支持 LLM 可解析的结构化输出
- ⬜ **P7** 跨平台 release（`make cross-compile` 已就位）

## 推理示例

```bash
# 在 philosophers 示例上跑推理
myonto reason          # dry-run 展示 5 条新推论（5 位哲学家都属于 Person）
myonto reason -a       # 物化到本体
myonto reason          # 再跑 → "已饱和"
```

`reason` 内置 6 条规则：subClassOf 传递、类型继承、subPropertyOf 传递、属性继承、传递属性、对称属性、inverseOf。详见 `internal/reasoning/`。

## 可视化示例

```bash
myonto graph                        # 生成 ontology-graph.html，自动打开浏览器
myonto graph -o my-graph.html       # 自定义输出
myonto graph --include-pred knows   # 只画 knows 关系
```

生成的 HTML 是单文件，vis-network 9.x 从 CDN 加载（首次需联网），之后浏览器缓存可用。

## AI 示例

```bash
# 1. 在 .myonto.toml 配置 [llm]，或用环境变量：
export MYONTO_LLM_BASE_URL="https://api.deepseek.com/v1"
export MYONTO_LLM_API_KEY="sk-..."
export MYONTO_LLM_MODEL="deepseek-chat"

# 2. 跑 AI 命令（默认 dry-run，审视输出后再 -a 写回）
myonto ai summarize isaac-newton
myonto ai extract "苏格拉底是柏拉图的老师，柏拉图教了亚里士多德"
myonto ai suggest-relations aristotle
myonto ai qa "牛顿是哪个国家的人？"
```

支持任何 OpenAI 兼容端点：OpenAI、DeepSeek、智谱、Ollama（本地）、LM Studio 等。

## 许可

MIT
