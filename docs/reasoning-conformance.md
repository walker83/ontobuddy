# 推理一致性声明（Reasoning Conformance）

本文档把"myonto 推理机承诺什么、不承诺什么"白纸黑字化，并给出可证伪的判据。
目的：终结"推理效果怎么说都有道理"的主观讨论——有了 conformance matrix 和
对拍基线，好不好变成几个硬数字。

> 相关代码：`internal/reasoning/`（引擎）、`internal/reasoning/eval/`（评估器）、
> `eval/crosscheck.py`（owlrl 对拍）。跑 `make eval` 看 golden 回归，`make crosscheck` 看对拍基线。

## 1. 正确性边界声明

myonto 推理机保证 **soundness（可靠性）**，不保证 **completeness（完备性）**：

- **Soundness（承诺）**：推理机输出的每一条三元组在语义上都成立。
  即"推出来的都是对的"。判据：**对拍 FP vs owlrl == 0**（见 §4 基线）。
- **Completeness（有限承诺）**：仅在下面 §2 列出的"已实现规则"范围内保证完备。
  对未实现的 OWL 构造（§3），不保证推出 owlrl 会推出的全部结论——这些缺口
  是**有意为之的设计选择**，记录在案，非 bug。

这个立场与 `internal/reasoning/reasoning.go:11-12` 的包注释一致：
> 本项目不实现完整的 OWL 2 DL 推理（那需要 Java + HermiT/Pellet）。
> 实现的是 OWL 2 RL 风格的"可规则化"子集。

## 2. 已实现的规则（✅，声明子集内保证完备）

每条规则都对应一个 golden case（`internal/reasoning/eval/testdata/cases/`）
和对拍证据。所有 ✅ 规则在对拍基线中 recall = 100%。

| # | 规则 | 形式 | 代码 | 测试 case | 状态 |
|---|------|------|------|-----------|------|
| 1 | subClassOf 传递 | A⊑B, B⊑C ⟹ A⊑C | `subClassOfTransitiveRule` | 01, 08, 10 | ✅ |
| 2 | 类型继承 | x a A, A⊑B ⟹ x a B | `typeInheritanceRule` | 02, 08, 10 | ✅ |
| 3 | subPropertyOf 传递 | P⊑Q, Q⊑R ⟹ P⊑R | `subPropertyOfTransitiveRule` | 03 | ✅ |
| 4 | 属性继承 | a P b, P⊑Q ⟹ a Q b | `propertyInheritanceRule` | 04, 09 | ✅ |
| 5 | 传递属性 | P:Transitive, aPb,bPc ⟹ aPc | `transitivePropertyRule` | 05, 09, 10 | ✅ |
| 6 | 对称属性 | P:Symmetric, aPb ⟹ bPa | `symmetricPropertyRule` | 06 | ✅ |
| 7 | 逆属性（单向） | P inverseOf Q, aPb ⟹ bQa | `inverseOfRule` | 07, 10 | ✅ |
| 8 | rdfs:domain | x P y, P domain C ⟹ x a C | `domainRule` | 13 | ✅ |
| 9 | rdfs:range | x P y, P range C ⟹ y a C（y 为 IRI） | `rangeRule` | 13, 16 | ✅ |

> **注意规则 7 的单向性**：实现里 `inverses` 只存正向映射，规则只推 `a P b ⟹ b Q a`，
> 不推 `b Q a ⟹ a P b`。owlrl 的 inverseOf 是双向的——这是与标准的**已知差异**，
> 见 §4 基线说明。

> **规则 8/9 的 soundness 守卫**：domain/range 不对元谓词（type/label/comment/
> subClassOf/subPropertyOf/disjointWith/equivalentClass/inverseOf）生效——给
> schema 三元组的主语/宾语加用户类型会污染本体。range 还跳过字面量宾语
> （字面量无类型身份，加 rdf:type 语义错误且破坏去重）。

## 3. 未实现的 OWL 构造（❌，明确不支持，缺口可追溯）

以下构造 owlrl 会推导、myonto 不会。每一条都是**有意不做**，原因记录在此。
对拍基线（§4）会把它们精确暴露为 FN，便于评估"如果补上能提升多少 recall"。

| 构造 | 典型推导 | 不支持原因 / 状态 |
|------|---------|-------------------|
| `owl:equivalentClass` | A≡B ⟹ A⊑B, B⊑A | 未实现。 |
| `owl:equivalentProperty` | 同上，属性版 | 未实现。 |
| `owl:FunctionalProperty` | 唯一性合并 | 未实现。涉及个体合并，复杂度高。 |
| `owl:InverseFunctionalProperty` | 同上 | 未实现。 |
| `owl:hasKey` | 按键合并个体 | 未实现。 |
| `owl:hasValue` / `someValuesFrom` / `allValuesFrom` | 限制推理 | 未实现。 |
| `owl:sameAs`（reflexivity） | X sameAs X | 故意不输出（公理性噪声，无用户价值）。 |
| `owl:propertyChainAxiom` | 属性链 | 未实现。 |
| 元类型归类 | X a owl:Class / owl:Thing | 故意不输出（避免元类污染）。 |

> **已补强的缺口**（从 §3 移到 §2）：
> - `rdfs:domain` / `rdfs:range`：已实现（规则 8/9），对拍 recall 从 0.941 升至 1.000。
> - `owl:disjointWith`：虽未实现为**推理规则**（不推导三元组），但已实现为**一致性检查**
>   （见 §7，`myonto check` 命令 + `Reasoner.Check()` 通道）。

## 4. 对拍基线（vs owlrl，客观证据）

跑 `make crosscheck` 生成 `eval/report.md`。当前基线（4 个用例）：

```
汇总: Precision=1.0000  Recall=1.0000  F1=1.0000 (TP=17 FP=0 FN=0)
  FP vs owlrl = 0     ← soundness 保证（myonto 从不比标准多推）
  FN vs owlrl = 0     ← 补 domain/range 后无缺口
```

**判据解读**：
- **FP == 0**（硬门禁）：4 个用例、17 条推导，**无一比 OWL 2 RL 多推**。
  这是 soundness 的客观证据。CI 可用 `make crosscheck --strict` 把它设为硬门禁。
- **Recall = 1.000**：补强 `rdfs:domain`/`rdfs:range`（规则 8/9）后，
  原 case 04 的唯一 FN（`principia a Work`）已被消除。
- **在声明支持的 9 条规则范围内，recall = 100%**——完备性在承诺子集内成立。

> 对拍 recall 只在 myonto 支持的谓词（subClassOf/type/subPropertyOf + 用户自定义属性）
> 上计算，并过滤掉 owlrl 输出的公理性噪声（owl:sameAs 自环、内置词汇元类型标注、
> owl:Nothing/Thing 等）。详见 `eval/crosscheck.py` 的 `is_supported` / `is_builtin_subject`。

## 5. 如何用这套体系回答"推理好不好"

| 问题 | 看哪里 | 判据 |
|------|--------|------|
| 推出来的都对吗？ | `make crosscheck` 的 FP | FP == 0（硬门禁） |
| 该推的都推了吗（承诺范围内）？ | `make eval` 17 个 golden case | 每 case P=R=F1=1.0 |
| 和标准比漏了多少？ | `make crosscheck` 的 FN | 全部归因到 §3 的 known-unsupported |
| 改动有没有引入回归？ | `make check`（vet+test+eval） | CI 全绿 |
| 一致性检查有效吗？ | `make eval`（check 类 case） | 有 `expect_findings` 的 case 必须 hit 全中、无 extra |
| 代码质量底线 | `make coverage` | 整体 ≥ 45%，reasoning+eval ≥ 80% |

任何一条退化都会被立刻发现、定位到具体三元组、追溯到具体规则或缺口。

## 6. 维护约定

- 新增推理规则时：① 加 golden case；② 更新 §2 表格；③ 重跑对拍确认 FP 仍为 0。
- 修复规则 bug 时：优先加能复现 bug 的 golden 负例，再改代码。
- 补实现 §3 的某构造时：把它从 §3 移到 §2，更新对拍判据（recall 会上升）。
- 新增一致性检查时：① 在 case.json 加 `expect_findings`；② eval 自动跑 check 评估。
- 安全/推理相关 PR 按 `AGENTS.md` 约定在 commit msg 标注 ⚠️。

## 7. 一致性检查（Consistency Checking）

除推导三元组（§2）外，myonto 还有独立的一**致性检查通道**——报告本体违反自身
声明的约束，与推导的三元组是**不同种类的输出**：

- 推导三元组：隐式知识的显式化，语义成立，可安全物化（`reason -a`）。
- 一致性 Finding：本体自相矛盾的错误报告，**不可物化**，由 `check` 命令输出。

| 检查 | 触发条件 | 输出 | 代码 | 测试 case | 状态 |
|------|---------|------|------|-----------|------|
| `owl:disjointWith` | x a A, x a B, A disjointWith B | error Finding | `checkDisjointWith` | 17 | ✅ |

**关键设计**：检查基于**推导后的完整类型集**（先 `Derive()` 再查），因此能捕获经
`subClassOf` 继承得到的隐式冲突（如 `Kitten ⊑ Cat`，`felix a Kitten, a Dog`，
`Cat disjointWith Dog` ⟹ felix 隐式冲突）。

用法：
```bash
myonto check              # 打印所有 Finding
myonto check --strict     # 有 error 时退出码 1（CI 门禁）
myonto check --json       # 结构化输出
```

> `owl:hasKey` / `FunctionalProperty` 等需要个体合并的检查未实现（复杂度高，
> 见 §3）。`disjointWith` 之外的一致性规则是后续优先项。
