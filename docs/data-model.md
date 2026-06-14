# 数据模型与命名空间

本文档解释 myonto 在底层如何表示你的知识，以及涉及的 RDF/OWL 词表。

---

## RDF 三元组：最基础的单位

RDF（Resource Description Framework）用**三元组（triple）**描述一切：

```
<主语 Subject>  <谓词 Predicate>  <宾语 Object>
```

每一条知识都是一句话："**主语** 有个属性/关系叫 **谓词**，值是 **宾语**"。

例如：
| 主语 | 谓词 | 宾语 | 含义 |
|---|---|---|---|
| `ex:isaac-newton` | `rdfs:label` | `"isaac-newton"` | newton 的显示名 |
| `ex:isaac-newton` | `rdf:type` | `ex:Scientist` | newton 是个 Scientist |
| `ex:isaac-newton` | `ex:knows` | `ex:leibniz` | newton 认识 leibniz |
| `ex:isaac-newton` | `ex:bornIn` | `1643` | newton 生于 1643 年 |

整个本体就是一组三元组的集合。

---

## Term 的三种类型

每个 S/P/O 位置上的值都是一种 **Term**：

### 1. IRI（国际资源标识符）
全局唯一的标识，类似 URL。`ex:isaac-newton` 是缩写，完整形式是 `http://example.org/isaac-newton`。
- 主语必须是 IRI（或空白节点）。
- 谓词必须是 IRI。
- 宾语可以是 IRI（指向另一个实体）或字面量。

### 2. Literal（字面量）
纯文本值，如 `"1643"`、`"英国物理学家"`。可带：
- **语言标签**：`"hello"@en`、`"你好"@zh`
- **数据类型**：`"1643"^^xsd:integer`、`"true"^^xsd:boolean`

`myonto link ... -l` 会让宾语成为字面量。

### 3. Blank Node（空白节点）
没有 IRI 的匿名节点，本项目暂不使用。

---

## 标准词表

myonto 复用 W3C 标准命名空间，不发明新概念：

### `rdf:` — RDF 核心 (`http://www.w3.org/1999/02/22-rdf-syntax-ns#`)
| 谓词 | 含义 | myonto 用法 |
|---|---|---|
| `rdf:type` | "是…的实例" | `entity add -t Person` 自动生成 |

### `rdfs:` — RDF Schema (`http://www.w3.org/2000/01/rdf-schema#`)
| 谓词 | 含义 | myonto 用法 |
|---|---|---|
| `rdfs:label` | 显示名 | `entity add` 自动生成 |
| `rdfs:comment` | 描述 | `entity add -d` / `entity edit -d` |
| `rdfs:Class` | 类的类型 | `entity add-class` 自动生成 |
| `rdfs:subClassOf` | 子类关系 | `entity add-class -p` 自动生成 |
| `rdfs:subPropertyOf` | 子属性关系 | 后续支持 |

### `owl:` — OWL 本体语言 (`http://www.w3.org/2002/07/owl#`)
| 谓词 | 含义 | 状态 |
|---|---|---|
| `owl:TransitiveProperty` | 传递属性（partOf 等） | P2 推理支持 |
| `owl:SymmetricProperty` | 对称属性（knows 等） | P2 推理支持 |
| `owl:inverseOf` | 逆关系（teaches ↔ taughtBy） | P2 推理支持 |

### `xsd:` — XML Schema 数据类型 (`http://www.w3.org/2001/XMLSchema#`)
用于字面量的数据类型：`xsd:string`、`xsd:integer`、`xsd:boolean` 等。

---

## 本地命名空间

`myonto init` 时会指定一个**本地命名空间**，默认 `http://example.org/`，前缀 `ex`。

所有 `myonto entity add isaac-newton` 生成的实体 IRI 是 `http://example.org/isaac-newton`，在 Turtle 里写作 `ex:isaac-newton`。

你可以换成自己的命名空间：
```bash
myonto init -i "http://myorg.org/kb/" -p kb
```

> **为什么需要 IRI？** RDF 要求每个资源全局唯一。即使两个不同的本体都有叫 `newton` 的实体，因为命名空间不同，它们的 IRI 不会冲突，可以无缝合并。

---

## 类层级示例

```
ex:Agent          （顶层抽象类）
  └── ex:Person   （myonto entity add-class Person）
        └── ex:Scientist   （myonto entity add-class Scientist -p Person）
              ├── ex:isaac-newton   （个体）
              └── ex:leibniz         （个体）
```

对应的 Turtle：
```turtle
ex:Agent rdf:type rdfs:Class .
ex:Person rdf:type rdfs:Class ;
    rdfs:subClassOf ex:Agent .
ex:Scientist rdf:type rdfs:Class ;
    rdfs:subClassOf ex:Person .
ex:isaac-newton rdf:type ex:Scientist .    # 隐含也是 Person、Agent
```

P2 的推理功能会自动推导出 `ex:isaac-newton rdf:type ex:Person`（类型继承），无需手动声明。

---

## 内部实现

myonto 在 `internal/rdf` 包里自实现了轻量 RDF 层：
- `Term` 结构体统一表示 IRI / Literal / Blank
- `Triple` 结构体表示一条三元组
- `Store`（`internal/store`）在内存中保存去重的三元组集合

不依赖任何外部 RDF 库——这是为了让二进制尽可能独立、可维护。
