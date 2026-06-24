# 本体驱动的数据分析

本文档说明如何用 myonto 把数据建模成本体后，做消费与分析。涵盖四类分析范式，
每类对应不同的 myonto 能力，并给出何时该外接标准 RDF 工具链的判断。

## 四类分析范式

| 范式 | 干什么 | myonto 能力 | 状态 |
|------|--------|------------|------|
| ① 隐式知识显式化 | 把"写了 A 就该有的 B"补出来 | `reason`（9 条规则） | ✅ 核心强项 |
| ② 图遍历与路径分析 | 闭包、最短路径、影响面 | `closure` / `path` | ✅ |
| ③ 多维聚合统计 | 按类/属性分组、Top-N、JOIN | `query`（SPARQL 子集） | ✅ |
| ④ 一致性审计 | 检测违反约束的矛盾 | `check`（disjointWith） | ✅ 基础 |

## ① 隐式知识显式化（`reason`）

声明 schema（类层级、传递/对称/逆属性、domain/range），录入直接关系，让推理机
自动补出全部隐含结论。

```turtle
# schema
ex:Scientist rdfs:subClassOf ex:Person .
ex:dependsOn a owl:TransitiveProperty .
ex:wrote rdfs:domain ex:Person ; rdfs:range ex:Work .

# 直接数据（只写一层）
ex:newton a ex:Scientist ; ex:wrote ex:principia .
ex:cmd ex:dependsOn ex:cli . ex:cli ex:dependsOn ex:store .
```

```bash
myonto reason -a   # 推出：newton a Person（继承）、principia a Work（range）、
                   #         cmd dependsOn store（传递）...
```

**价值**：写入成本与分类粒度解耦——细粒度录入，粗粒度查询，由 schema 衔接。
规则覆盖见 [`reasoning-conformance.md`](reasoning-conformance.md) §2（9 条）。

## ② 图遍历与闭包查询（`closure` / `path`）

**闭包**——影响面分析。典型场景：组织汇报线、配件 BOM、供应链、代码依赖。

```bash
myonto closure cmd -p dependsOn          # cmd 直接+间接依赖的全部包
myonto closure rdf -p dependsOn -r       # 谁依赖了 rdf（反向）
myonto closure alice -p reportsTo -d 2   # 限制 2 跳
```

在**原始边集**上 BFS，深度反映真实跳数。若要看含隐式边的闭包，先 `reason -a`。

**最短路径**——关系解释。

```bash
myonto path alice bob          # A 和 B 怎么关联上的
myonto path alice bob -p knows # 只走 knows 关系
```

找不到路径时退出码 1，便于脚本判断连通性。

## ③ 多维聚合统计（`query`）

轻量 SPARQL 子集：三元组模式匹配 + GROUP BY + COUNT + Top-N + JOIN。
**自动包含推理推出的隐式知识**——这是本体驱动分析的核心价值。

```bash
# 按类型计数（含子类实例：Scientist ⊑ Person ⟹ Scientist 也算 Person）
myonto query -w "?s a ?o" -g "?o" -c

# Top 5 出生地
myonto query -w "?s ex:bornIn ?o" -g "?o" -c -n 5

# JOIN：谁认识 carol 的朋友？
myonto query -w "?s ex:knows ?o" -w "?o ex:knows ex:carol"
```

模式语法：`?var` 是变量，`a` = `rdf:type`，支持 `prefix:local` / `<IRI>` / `"字面量"`。
输出支持 `--json`（`{patterns, group_by, count, results:[{key, count}], total}`）。

**何时该外接**：`query` 覆盖 80% 常见分析。若需要完整 SPARQL（OPTIONAL/FILTER/
正则、嵌套子查询、CONSTRUCT），导出 TTL 接 Jena `arq` 或 Python rdflib：

```bash
myonto export                  # 导出 TTL（标准格式）
# 然后：arq --data ontology.ttl --query q.rq
```

## ④ 一致性审计（`check`）

检测本体违反自身声明的约束。当前支持 `owl:disjointWith`——若个体同时属于
两个互斥类，报 error。

```turtle
ex:Cat owl:disjointWith ex:Dog .
ex:felix a ex:Cat ; a ex:Dog .   # ← 矛盾
```

```bash
myonto check              # 报告 felix 的冲突
myonto check --strict     # CI 门禁：有 error 时 exit 1
```

**关键**：检查基于推导后的完整类型集，能捕获经继承得到的隐式冲突
（`Kitten ⊑ Cat`，`felix a Kitten, a Dog` ⟹ felix 隐式 a Cat，与 Dog 冲突）。

未覆盖：`hasKey` / `FunctionalProperty`（需个体合并，复杂度高）。

## 一句话总结

> myonto 是"建模 + 推理 + 分析"一体的轻量工具。影响面/闭包/聚合/一致性检查
> 都内置；完整 SPARQL 和 OWL DL 一致性外接标准工具链。所有数据是标准 TTL，
> 可无损导入 Protégé / Jena / rdflib。

详细命令参考：[`skills/myonto/references/commands.md`](../skills/myonto/references/commands.md)
推理能力边界：[`reasoning-conformance.md`](reasoning-conformance.md)
