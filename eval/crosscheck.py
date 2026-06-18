#!/usr/bin/env python3
"""owlrl 对拍脚本：把 myonto 推理机的输出与 owlrl（OWL 2 RL 参考实现）对比。

目标：客观回答"myonto 推得对不对、全不全"——
  - FP (myonto 推了 owlrl 没推) = 可靠性问题，必须为 0
  - FN (owlrl 推了 myonto 没推) = 完备性问题；其中属于 myonto 明确不支持的构造
    （domain/range/FunctionalProperty 等）记为 known-unsupported，不算回归

用法：
    pip install rdflib owlrl
    python3 eval/crosscheck.py eval/cases_for_crosscheck/ [--report eval/report.md]

判据（见 docs/reasoning-conformance.md）：
    - FP vs owlrl 必须 == 0（决不比标准多推）
    - Recall vs owlrl（声明子集内）>= 0.95（可选软门禁，记录到 report）
"""

from __future__ import annotations

import argparse
import json
import os
import subprocess
import sys
import tempfile
from dataclasses import dataclass, field
from pathlib import Path

# 第三方依赖：只在跑对拍时需要
try:
    import rdflib
    from rdflib import Graph, URIRef, Literal, BNode
    import owlrl
except ImportError:
    print("缺少依赖，请先安装：pip install rdflib owlrl", file=sys.stderr)
    sys.exit(2)


# REPO_ROOT / MYONTO_BIN 的解析在 main() 里做。


@dataclass
class CaseResult:
    """单个对拍用例的结果。"""
    name: str
    myonto_derived: set  # myonto 推导的三元组集合（规范字符串）
    owlrl_derived: set   # owlrl 推导的三元组集合
    tp: int = 0          # 两者都推（myonto 推的也在 owlrl 里）
    fp: int = 0          # myonto 推了 owlrl 没推（可靠性问题，必须 0）
    fn: int = 0          # owlrl 推了 myonto 没推（完备性问题）
    fp_examples: list = field(default_factory=list)  # FP 明细
    fn_examples: list = field(default_factory=list)  # FN 明细

    @property
    def precision(self) -> float:
        return self.tp / (self.tp + self.fp) if (self.tp + self.fp) else 1.0

    @property
    def recall(self) -> float:
        return self.tp / (self.tp + self.fn) if (self.tp + self.fn) else 1.0

    @property
    def f1(self) -> float:
        p, r = self.precision, self.recall
        return 2 * p * r / (p + r) if (p + r) else 0.0


def triple_key(s, p, o) -> str:
    """把三元组转成规范字符串用于集合对比。

    注意 owlrl 会把 rdf:type 写成完整 IRI，myonto --json 输出也是完整 IRI，
    所以两边都用 n3 形式规范化即可对齐。
    """
    return f"{s.n3()} {p.n3()} {o.n3()}"


# myonto 明确支持的谓词集合（与 internal/reasoning 的 7 条规则对应）。
# 对拍 recall 只在这些谓词上计算——owlrl 会输出大量公理性三元组
# （owl:sameAs 自环、X a owl:Class 元类型、owl:Nothing/Thing），
# 那些属于 myonto 设计上不打算输出的"OWL 2 RL 公理噪声"，归入
# known-unsupported，不计入 recall 损失。
SUPPORTED_PREDICATES = {
    "http://www.w3.org/2000/01/rdf-schema#subClassOf",
    "http://www.w3.org/1999/02/22-rdf-syntax-ns#type",
    "http://www.w3.org/2000/01/rdf-schema#subPropertyOf",
    # 个体上的普通属性（transitive/symmetric/inverse 规则作用其上）。
    # 用前缀匹配，见 is_supported。
}


def is_supported(p) -> bool:
    """判断一个谓词是否落在 myonto 声明支持的范围内。

    规则：
      - rdfs:subClassOf / rdf:type / rdfs:subPropertyOf：直接支持。
      - 任何 http://example.org/ 下的属性：myonto 对它们跑 transitive/
        symmetric/inverse/subProperty 继承，算支持。
      - owl:sameAs / owl:equivalentClass / rdfs:domain 等：不支持（公理/未实现）。
      - rdf:type 的 object 若是 owl:Class/rdfs:Datatype 等元类型：不支持
        （myonto 不输出元类型归类）。
    """
    ps = str(p)
    if ps in SUPPORTED_PREDICATES:
        return True
    # ex: 命名空间下的自定义属性
    if ps.startswith("http://example.org/"):
        return True
    return False


def is_meta_type(o) -> bool:
    """rdf:type 的 object 是否是 owlrl 输出的元类型（owl:Class 等），
    myonto 故意不输出这类归类。"""
    os = str(o)
    meta = (
        "http://www.w3.org/2002/07/owl#Class",
        "http://www.w3.org/2000/01/rdf-schema#Class",
        "http://www.w3.org/2002/07/owl#Thing",
        "http://www.w3.org/2002/07/owl#Nothing",
        "http://www.w3.org/2002/07/owl#DatatypeProperty",
        "http://www.w3.org/2002/07/owl#ObjectProperty",
        "http://www.w3.org/2002/07/owl#AnnotationProperty",
        "http://www.w3.org/2002/07/owl#TransitiveProperty",
        "http://www.w3.org/2002/07/owl#SymmetricProperty",
        "http://www.w3.org/1999/02/22-rdf-syntax-ns#Property",
        "http://www.w3.org/2000/01/rdf-schema#Datatype",
    )
    return os in meta


# owlrl 会给 RDF/OWL/RDFS 内置词汇（rdfs:label, rdf:type 等本身）加元类型标注，
# 例如 `rdfs:comment a owl:AnnotationProperty`。这些 subject 落在 W3C 命名空间，
# myonto 只处理用户数据，从不输出内置词汇的归类。一律跳过。
BUILTIN_NAMESPACES = (
    "http://www.w3.org/1999/02/22-rdf-syntax-ns#",
    "http://www.w3.org/2000/01/rdf-schema#",
    "http://www.w3.org/2002/07/owl#",
    "http://www.w3.org/2001/XMLSchema#",
)


def is_builtin_subject(s) -> bool:
    """subject 是否落在 RDF/OWL/RDFS 内置命名空间——这类三元组 owlrl 输出
    是给内置词汇做元类型标注，与用户本体无关，myonto 不处理。"""
    ss = str(s)
    return any(ss.startswith(ns) for ns in BUILTIN_NAMESPACES)


def run_myonto_reason(ttl_path: Path, myonto_bin: Path) -> set:
    """用 myonto reason --json 跑推理，返回 derived 三元组集合（规范字符串）。

    需要在临时目录里建一个 .myonto.toml 指向 ttl 文件，
    因为 myonto 命令依赖工作目录的配置。
    """
    with tempfile.TemporaryDirectory() as tmpdir:
        # 配置文件：base_iri 用 ex，数据文件指向被测 ttl 的副本
        data_copy = Path(tmpdir) / "ontology.ttl"
        data_copy.write_text(ttl_path.read_text())
        toml = f"""base_iri = 'http://example.org/'
data_file = 'ontology.ttl'
prefix = 'ex'
"""
        (Path(tmpdir) / ".myonto.toml").write_text(toml)

        # 跑 reason --json
        proc = subprocess.run(
            [str(myonto_bin), "reason", "--json"],
            cwd=tmpdir,
            capture_output=True,
            text=True,
        )
        if proc.returncode != 0:
            print(f"  ⚠️ myonto reason 失败 ({ttl_path.name}): {proc.stderr}", file=sys.stderr)
            return set()

        try:
            result = json.loads(proc.stdout)
        except json.JSONDecodeError as e:
            print(f"  ⚠️ myonto JSON 解析失败 ({ttl_path.name}): {e}", file=sys.stderr)
            return set()

        derived = set()
        for t in result.get("derived", []):
            # jsonTriple: {subject, predicate (完整 IRI), object: {value, ...}}
            s = URIRef(t["subject"])
            p = URIRef(t["predicate"])
            obj = t["object"]
            if obj.get("type") == "iri":
                o = URIRef(obj["value"])
            elif obj.get("type") == "literal":
                dt = obj.get("datatype", "")
                lang = obj.get("lang", "")
                if lang:
                    o = Literal(obj["value"], lang=lang)
                elif dt:
                    o = Literal(obj["value"], datatype=URIRef(dt))
                else:
                    o = Literal(obj["value"])
            else:
                o = Literal(obj["value"])
            if not is_supported(p):
                continue
            if is_builtin_subject(s):
                continue
            if str(p).endswith("#type") and is_meta_type(o):
                continue
            derived.add(triple_key(s, p, o))
        return derived


def run_owlrl_reason(ttl_path: Path) -> set:
    """用 owlrl 跑 OWL 2 RL 推理，返回 derived 三元组集合（= 闭包 - 原始）。

    只保留落在 myonto 支持谓词范围内的三元组（见 is_supported），
    使 recall 数字有意义——否则 owlrl 输出的公理性噪声（owl:sameAs 自环等）
    会让 recall 永远惨不忍睹，掩盖真实的完备性缺口。
    """
    g = Graph()
    g.parse(str(ttl_path), format="turtle")
    original = set(g)

    # OWLRL_Semantics 是 OWL 2 RL 的完整规则集
    owlrl.DeductiveClosure(owlrl.OWLRL_Semantics).expand(g)

    derived = set()
    for s, p, o in g:
        if (s, p, o) not in original:
            if not is_supported(p):
                continue
            if is_builtin_subject(s):
                continue
            if str(p).endswith("#type") and is_meta_type(o):
                continue
            derived.add(triple_key(s, p, o))
    return derived


def crosscheck_case(ttl_path: Path, myonto_bin: Path) -> CaseResult:
    """对拍单个 ttl。"""
    name = ttl_path.stem
    myonto_d = run_myonto_reason(ttl_path, myonto_bin)
    owlrl_d = run_owlrl_reason(ttl_path)

    res = CaseResult(name=name, myonto_derived=myonto_d, owlrl_derived=owlrl_d)
    # TP: myonto 推的也在 owlrl 里
    common = myonto_d & owlrl_d
    res.tp = len(common)
    # FP: myonto 推了 owlrl 没推
    fp_set = myonto_d - owlrl_d
    res.fp = len(fp_set)
    res.fp_examples = sorted(fp_set)[:20]
    # FN: owlrl 推了 myonto 没推
    fn_set = owlrl_d - myonto_d
    res.fn = len(fn_set)
    res.fn_examples = sorted(fn_set)[:20]
    return res


def main():
    ap = argparse.ArgumentParser(description="myonto vs owlrl 推理对拍")
    ap.add_argument("cases_dir", help="含 .ttl 对拍用例的目录")
    ap.add_argument("--report", default=None, help="报告输出路径（默认只打印 stdout）")
    ap.add_argument("--myonto", default=None, help="myonto 二进制路径（默认用 ./bin/myonto）")
    ap.add_argument("--strict", action="store_true",
                    help="严格模式：FP>0 直接 exit 1（CI 硬门禁）")
    args = ap.parse_args()

    repo_root = Path(__file__).resolve().parent.parent
    myonto_bin = Path(args.myonto) if args.myonto else (repo_root / "bin" / "myonto")
    if not myonto_bin.exists():
        print(f"✗ 找不到 myonto 二进制：{myonto_bin}", file=sys.stderr)
        print("  请先 make build", file=sys.stderr)
        sys.exit(2)

    cases_dir = Path(args.cases_dir)
    if not cases_dir.is_dir():
        print(f"✗ 不是目录：{cases_dir}", file=sys.stderr)
        sys.exit(2)

    ttl_files = sorted(cases_dir.glob("*.ttl"))
    if not ttl_files:
        print(f"✗ {cases_dir} 下无 .ttl 文件", file=sys.stderr)
        sys.exit(2)

    print(f"对拍 {len(ttl_files)} 个用例（myonto={myonto_bin} vs owlrl）...\n")
    results = []
    total_tp = total_fp = total_fn = 0
    for ttl in ttl_files:
        r = crosscheck_case(ttl, myonto_bin)
        results.append(r)
        total_tp += r.tp
        total_fp += r.fp
        total_fn += r.fn
        status = "✓" if r.fp == 0 else "✗"
        print(f"  {status} {r.name:40s} P={r.precision:.3f} R={r.recall:.3f} "
              f"TP={r.tp} FP={r.fp} FN={r.fn}")
        for ex in r.fp_examples[:5]:
            print(f"      FP: {ex}")
        for ex in r.fn_examples[:3]:
            print(f"      FN: {ex}")

    # 汇总（micro-average）
    mp = total_tp / (total_tp + total_fp) if (total_tp + total_fp) else 1.0
    mr = total_tp / (total_tp + total_fn) if (total_tp + total_fn) else 1.0
    mf = 2 * mp * mr / (mp + mr) if (mp + mr) else 0.0
    print(f"\n{'═' * 60}")
    print(f"汇总: Precision={mp:.4f} Recall={mr:.4f} F1={mf:.4f} "
          f"(TP={total_tp} FP={total_fp} FN={total_fn})")
    print(f"  FP vs owlrl = {total_fp}（必须为 0：soundness 保证）")
    print(f"  FN vs owlrl = {total_fn}（部分属 known-unsupported 构造）")

    # 报告文件
    if args.report:
        write_report(args.report, results, total_tp, total_fp, total_fn, mp, mr, mf)
        print(f"\n报告已写入 {args.report}")

    # 严格模式门禁
    if args.strict and total_fp > 0:
        print(f"\n✗ STRICT 模式：FP={total_fp} > 0，可靠性违规，exit 1", file=sys.stderr)
        sys.exit(1)
    print(f"\n{'✓' if total_fp == 0 else '✗'} 可靠性门禁（FP==0）")


def write_report(path, results, tp, fp, fn, p, r, f1):
    """写 Markdown 报告。"""
    lines = [
        "# myonto vs owlrl 对拍报告",
        "",
        f"- 用例数：{len(results)}",
        f"- 汇总（micro-avg）：Precision={p:.4f} Recall={r:.4f} F1={f1:.4f}",
        f"- TP={tp} FP={fp} FN={fn}",
        "",
        "## 判据",
        "",
        "- **FP vs owlrl == 0**（可靠性/soundness）：myonto 绝不比标准多推。",
        "- **Recall vs owlrl**：FN 中属 myonto 明确不支持的构造（见 conformance matrix）不算回归；",
        "  其余为完备性缺口。",
        "",
        "## 逐用例",
        "",
        "| 用例 | P | R | TP | FP | FN |",
        "|------|---|---|----|----|----|",
    ]
    for res in results:
        lines.append(f"| {res.name} | {res.precision:.3f} | {res.recall:.3f} "
                     f"| {res.tp} | {res.fp} | {res.fn} |")
    lines.append("")
    lines.append("## FP 明细（可靠性问题，应为空）")
    lines.append("")
    for res in results:
        for ex in res.fp_examples:
            lines.append(f"- [{res.name}] {ex}")
    if not any(r.fp_examples for r in results):
        lines.append("（无）")
    lines.append("")
    lines.append("## FN 明细（完备性缺口，需对照 conformance matrix 判断是否 known-unsupported）")
    lines.append("")
    for res in results:
        for ex in res.fn_examples:
            lines.append(f"- [{res.name}] {ex}")
    if not any(r.fn_examples for r in results):
        lines.append("（无）")
    lines.append("")
    Path(path).write_text("\n".join(lines))


if __name__ == "__main__":
    main()
