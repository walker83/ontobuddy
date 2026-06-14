# 与外部 RDF 工具互通

myonto 的数据是标准 Turtle（`.ttl`），不锁死在本工具内。本文档列出如何把它接入其他 RDF 生态工具。

---

## 为什么能互通

myonto 生成的 `.ttl` 文件是 100% 标准 W3C Turtle 格式：
- 用标准命名空间（`rdf:` / `rdfs:` / `owl:` / `xsd:`）
- 不引入任何私有扩展
- 不依赖外部 schema

任何符合 W3C 标准的 RDF 工具都能读写它。

---

## Python：rdflib

[rdflib](https://rdflib.readthedocs.io/) 是 Python 最成熟的 RDF 库。

### 读取 myonto 生成的本体

```python
import rdflib

g = rdflib.Graph()
g.parse("ontology.ttl", format="turtle")

# 查询所有科学家
for s in g.subjects(rdflib.RDF.type, rdflib.URIRef("http://example.org/Scientist")):
    print(s)
```

### SPARQL 查询

```python
q = """
PREFIX ex: <http://example.org/>
SELECT ?person ?friend
WHERE {
    ?person ex:knows ?friend .
}
"""
for row in g.query(q):
    print(row.person, row.friend)
```

### 写回 myonto 能读的格式

```python
g.serialize(destination="ontology.ttl", format="turtle")
# 之后 myonto entity list 等命令可正常工作
```

---

## Apache Jena（命令行）

[Jena](https://jena.apache.org/) 提供 `arq`（SPARQL 查询）和 `riot`（格式转换）等 CLI 工具。

### SPARQL 查询
```bash
arq --data=ontology.ttl --query=query.rq
```

`query.rq`：
```sparql
PREFIX ex: <http://example.org/>
SELECT ?s ?o WHERE { ?s ex:knows ?o }
```

### 格式转换
```bash
# Turtle → JSON-LD
riot --output=jsonld ontology.ttl > ontology.jsonld

# Turtle → RDF/XML
riot --output=rdfxml ontology.ttl > ontology.rdf

# Turtle → N-Triples
riot --output=nt ontology.ttl > ontology.nt
```

---

## Protégé（图形界面）

[Protégé](https://protege.stanford.edu/) 是斯坦福的 OWL 本体编辑器，适合可视化浏览和复杂本体编辑。

### 用 Protégé 打开 myonto 本体
1. 启动 Protégé
2. `File → Open` 选择你的 `ontology.ttl`
3. Protégé 会显示类层级、实例、关系

### 在 Protégé 里编辑后回到 myonto
Protégé 默认存为 OWL/XML（`.owl`）。要回到 Turtle：
```bash
# 用 Jena 转换
riot --output=turtle ontology.owl > ontology.ttl
```

或在 Protégé 里 `File → Save as` 选择 Turtle 格式。

---

## 在线工具

### RDF Validator
- https://www.w3.org/RDF/Validator/ — 验证 Turtle 语法
- 把 `.ttl` 内容粘贴进去即可

### RDF Playground
- https://rdfplayground.org/ — 在线 SPARQL 查询

---

## 格式互转速查

myonto 内部只支持 Turtle，但通过外部工具可转换为：

| 格式 | 扩展名 | 特点 | 转换工具 |
|---|---|---|---|
| Turtle | `.ttl` | 人类可读、git 友好（myonto 默认） | — |
| N-Triples | `.nt` | 最简单、一行一条、无前缀 | `riot` / rdflib |
| JSON-LD | `.jsonld` | JSON 格式、Web 友好 | `riot` / rdflib |
| RDF/XML | `.rdf` / `.owl` | 传统格式、Protégé 默认 | `riot` / rdflib |
| Trig | `.trig` | 支持命名图 | `riot` |

---

## 合并多个本体

如果在不同目录建立了多个本体库（例如 `books/`、`projects/`），想合并：

### 用 rdflib 合并
```python
import rdflib

g = rdflib.Graph()
g.parse("books/ontology.ttl", format="turtle")
g.parse("projects/ontology.ttl", format="turtle")
g.serialize(destination="merged.ttl", format="turtle")
```

### 前提：命名空间不冲突
如果两个库都用了 `http://example.org/`，合并时会有 IRI 冲突。建议每个库用不同命名空间：
```bash
myonto init -i "http://mybooks.org/" -p books
myonto init -i "http://myproj.org/" -p proj
```

---

## 推荐工作流

1. **日常增删改**：用 myonto 命令（快、防错）
2. **批量导入**：写 Python 脚本用 rdflib
3. **复杂查询**：用 SPARQL（通过 Jena `arq` 或 rdflib）
4. **可视化浏览**：导出到 Protégé 或后续 myonto `graph` 命令（P2）
5. **格式分发**：按需转 JSON-LD（给 Web）/ RDF-XML（给 Protégé）
