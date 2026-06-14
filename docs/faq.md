# FAQ（常见问题）

---

## 安装与使用

### Q: 提示 `myonto: command not found`

`~/.local/bin` 不在 PATH 中。把它加进 shell 配置：

```bash
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.zshrc
source ~/.zshrc
```

验证：`which myonto` 应输出 `/Users/你/.local/bin/myonto`。

### Q: `make setup` 或 `go build` 卡住、拉依赖失败

GOPROXY 没配国内镜像。运行：

```bash
make setup
# 或手动：
go env -w GOPROXY=https://goproxy.cn,direct
go env -w GOSUMDB=sum.golang.google.cn
```

### Q: 改了代码后 `myonto` 命令没变化

`make link` 创建的是符号链接，但二进制本身需要重新编译。运行：

```bash
make build   # 或直接 make link（它会先 build 再 link）
```

### Q: `myonto` 提示 `.myonto.toml not found`

当前目录不是本体库。要么先 `myonto init`，要么 `cd` 到已初始化的目录。

myonto 会从当前目录**向上查找** `.myonto.toml`，所以你可以在本体库的子目录里直接用命令，类似 git。

---

## 概念

### Q: "类"和"实体"有什么区别？

- **类**是"种类"：`Person`、`Book`、`Project`。用 `myonto entity add-class` 创建。
- **实体**（也叫个体、实例）是"具体的东西"：`isaac-newton`、`thinking-fast-slow`。用 `myonto entity add` 创建，可指定类型（`-t Person`）。

类比面向对象编程：类是 `class`，实体是 `new` 出来的对象。

### Q: 为什么我用 `entity add 牛顿`，最后存的标识是 `entity`？

myonto 会把名字 **slug 化**作为 IRI 的 local name：转小写、空格转连字符、**非 ASCII 字符被移除**（中文会全部丢失）。

所以纯中文名字 `"牛顿"` slug 化后变成空，会 fallback 成 `entity`。

**建议**：实体名用英文/拼音（`isaac-newton` / `niu-dun`），中文内容放在 `-d`（描述）里：
```bash
myonto entity add niu-dun -t Person -d "牛顿，英国物理学家"
```

原始中文名会保存在 `rdfs:label` 中（如果你显式提供）。后续 P2 会改进这点。

### Q: IRI 是什么？为什么不是普通的 ID？

IRI（Internationalized Resource Identifier）类似 URL，是 RDF 标准的全局唯一标识。

`http://example.org/isaac-newton` 不需要真的能访问——它只是一个保证全局唯一的字符串。用 IRI 的好处：
- 两个不同本体库的 `newton` 不会冲突（命名空间不同）
- 可以无缝合并、互通（见 [interop.md](interop.md)）

### Q: `rdf:type` 和 `a` 有什么区别？

没有区别。`a` 是 `rdf:type` 的 Turtle 简写。这两行等价：
```turtle
ex:newton rdf:type ex:Scientist .
ex:newton a ex:Scientist .
```

---

## 数据与编辑

### Q: 我能直接编辑 `.ttl` 文件吗？

可以。`.ttl` 是纯文本，用任何编辑器都能改。myonto 下次读取时会重新解析。注意：
- 语法错误（如漏掉句点）会导致 myonto 报错
- 手动加的三元组会保留
- 格式（缩进、顺序）会被 myonto 在下次写入时规范化

### Q: 删除实体时，它的关系也会被删吗？

`myonto entity rm <name>` 会删除该实体作为**主语**的所有三元组。但如果它作为**宾语**出现在别的实体的关系里（如 `someone knows newton`），那些不会被删。

这是有意的设计——避免误删。要清理所有引用，需要手动 `myonto unlink`。

### Q: 怎么知道某个实体被谁引用？

目前需要手动搜索：
```bash
myonto search newton
```
后续 P2 会加 `myonto where-used <name>` 命令。

### Q: 数据存哪里？能放 git 吗？

`.myonto.toml` 和 `ontology.ttl` 都在当前目录，是普通文本文件，**非常适合 git**：

```bash
myonto init
git init
git add .myonto.toml ontology.ttl
git commit -m "init ontology"
```

每次 `entity add` / `link` 后都能 `git diff` 看到精确变化。

---

## 高级

### Q: 怎么备份本体？

`.ttl` 是纯文本，三种方式：
1. **git**（推荐）：版本历史、可回滚
2. **复制文件**：`cp ontology.ttl ontology.bak.ttl`
3. **导出**（后续 P2）：`myonto graph -o backup.html`

### Q: 怎么从其他来源批量导入？

写脚本生成 Turtle，或用 rdflib 合并（见 [interop.md](interop.md)）。例如从 CSV 导入：

```python
import rdflib
from rdflib import Graph, URIRef, Literal
from rdflib.namespace import RDF, RDFS

g = Graph()
g.parse("ontology.ttl", format="turtle")

import csv
with open("people.csv") as f:
    for row in csv.DictReader(f):
        s = URIRef("http://example.org/" + row["slug"])
        g.add((s, RDFS.label, Literal(row["name"])))
        g.add((s, RDFS.comment, Literal(row["desc"])))

g.serialize(destination="ontology.ttl", format="turtle")
```

### Q: 能不能用中文当谓词名（关系名）？

技术上可以（谓词是任意 IRI），但 slug 化会丢中文字符。建议用英文谓词：
```bash
myonto link newton knows leibniz              # ✓ 推荐
# 而不是
myonto link newton 认识 leibniz                # ✗ 认识会被 slug 化成 "entity"
```

### Q: 多个本体库能共享实体吗？

可以，但要用不同命名空间 + 完整 IRI 引用：
```bash
# 库 A
myonto init -i "http://a.org/" -p a
myonto entity add x

# 库 B 引用 A 的实体
myonto init -i "http://b.org/" -p b
myonto link y relatesTo <http://a.org/x>
```

---

## 报错排查

### `turtle parse error at line X col Y: ...`

`.ttl` 文件有语法错误。打开文件第 X 行查看，常见原因：
- 漏掉句点 `.`
- 字符串引号不匹配
- 前缀未声明

### `unknown prefix "xxx"`

用了未声明的前缀。在 `.ttl` 开头加：
```turtle
@prefix xxx: <http://your-namespace/> .
```

### `实体 xxx 已存在`

该 local name 已被占用。用 `entity show xxx` 查看现有内容，或换名：
```bash
myonto entity add isaac-newton-v2 -t Person
```

---

还有问题？提 issue 或查看源码：[`internal/`](../internal/)。
