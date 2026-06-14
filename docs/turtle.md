# Turtle 格式速查

Turtle（`.ttl`）是 RDF 的人类可读序列化格式，也是 myonto 的默认存储格式。本文档列出你写/读 Turtle 时需要知道的全部语法。

---

## 基本结构

一条三元组以句点 `.` 结尾：

```turtle
ex:newton ex:knows ex:leibniz .
```

读作："newton knows leibniz."

---

## 前缀声明

文件开头声明命名空间缩写：

```turtle
@prefix ex: <http://example.org/> .
@prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .
@prefix rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#> .
@prefix xsd: <http://www.w3.org/2001/XMLSchema#> .
```

之后就能用 `ex:newton` 代替完整 IRI `<http://example.org/newton>`。

---

## 合并相同主语：分号 `;`

多条共享主语的三元组用 `;` 连接：

```turtle
ex:newton ex:knows ex:leibniz ;
    ex:bornIn 1643 ;
    ex:diedIn 1727 .
```

等价于三条独立的三元组。

---

## 合并相同主语+谓词：逗号 `,`

多个宾语共享同一主语+谓词时用 `,`：

```turtle
ex:newton ex:studied ex:physics, ex:mathematics, ex:astronomy .
```

---

## 关键字 `a`

`a` 是 `rdf:type` 的简写：

```turtle
ex:newton a ex:Scientist .
# 等价于
ex:newton rdf:type ex:Scientist .
```

---

## 字面量

### 字符串
```turtle
ex:newton rdfs:label "Isaac Newton" .
ex:newton rdfs:comment "英国数学家、物理学家" .
```

### 带语言标签
```turtle
ex:greeting ex:hello "hello"@en, "你好"@zh .
```

### 带数据类型
```turtle
ex:newton ex:bornIn "1643"^^xsd:integer .
ex:newton ex:alive "false"^^xsd:boolean .
```

### 三引号（多行字符串）
```turtle
ex:newton ex:bio """Isaac Newton (1643-1727)
英国数学家、物理学家、天文学家。
微积分的共同发明者。""" .
```

---

## 注释

`#` 到行尾是注释：

```turtle
# 这是文件头注释
ex:newton a ex:Scientist .  # 这是行尾注释
```

---

## 完整示例

```turtle
@prefix ex: <http://example.org/> .
@prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .
@prefix rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#> .
@prefix xsd: <http://www.w3.org/2001/XMLSchema#> .

# 类定义
ex:Person a rdfs:Class ;
    rdfs:label "Person" ;
    rdfs:comment "一个人类" .

ex:Scientist a rdfs:Class ;
    rdfs:subClassOf ex:Person ;
    rdfs:comment "科学家" .

# 个体
ex:isaac-newton a ex:Scientist ;
    rdfs:label "Isaac Newton" ;
    rdfs:comment "英国数学家、物理学家" ;
    ex:bornIn "1643"^^xsd:integer ;
    ex:knows ex:leibniz ;
    ex:motto "hypotheses non fingo"@la .

ex:leibniz a ex:Scientist ;
    rdfs:label "Gottfried Wilhelm Leibniz" .
```

---

## 手动编辑的注意事项

myonto 生成的 Turtle 是规范化的（subject 按字母序、谓词按出现序）。如果你手动编辑：

1. **顺序无关紧要**——RDF 是集合语义，三元组顺序不影响含义。
2. **去重自动处理**——重复的三元组会被自动合并。
3. **myonto 会保留你的前缀声明**——读入时解析到的所有 `@prefix` 都会保留到输出。
4. **格式会被规范化**——下次 `myonto` 写入时，会重新排序和缩进。如果你在意手动格式，建议把"手动编辑的区段"放在单独文件，用 `@prefix` 引入。

---

## 转义

字符串中需要转义的字符：

| 字符 | 写法 |
|---|---|
| `"` | `\"` |
| `\` | `\\` |
| 换行 | `\n` |
| 回车 | `\r` |
| Tab | `\t` |

Unicode 字符用 `\uXXXX`（4 位十六进制）：
```turtle
ex:alpha ex:label "\u03B1-wave" .   # α-wave
```

---

## myonto 解析器支持的子集

myonto 自实现的 Turtle 解析器支持：

- ✅ `@prefix` / `PREFIX` 声明
- ✅ `@base` / `BASE`（解析但暂不使用）
- ✅ `<IRI>` 完整形式
- ✅ `prefix:local` 缩写
- ✅ `"..."`、`'...'`、`"""..."""`、`'''...'''` 字符串
- ✅ `@lang` 语言标签
- ✅ `^^<type>` / `^^prefix:type` 数据类型
- ✅ `a` 关键字
- ✅ `.` / `;` / `,` 分隔符
- ✅ `#` 注释
- ✅ `_:blank` 空白节点

**暂不支持**（对本项目使用场景足够）：
- `[]` 空白节点方括号语法
- `()` 集合语法
- 嵌套 blank node 谓词

如果需要这些高级特性，可手动编辑 `.ttl` 文件，但 myonto 在下次写入时会跳过无法解析的部分。
