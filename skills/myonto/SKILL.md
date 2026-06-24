---
name: myonto
description: CLI for managing a personal RDF/OWL ontology (entity-relationship knowledge graph stored as Turtle). Use when creating/editing/listing typed entities, linking them with predicates, querying or searching the graph, running RDFS/OWL inference, checking consistency, computing transitive closure or shortest paths, aggregating/grouping entities (SPARQL-like), generating interactive visualizations, or invoking LLM-assisted ontology maintenance. Trigger on "remember", "what do I know about", "add an entity", "link X to Y", "show relationships", "list entities", "run inference", "check consistency", "impact analysis", "transitive closure", "shortest path", "group by / count entities", "visualize", or any request involving a local knowledge graph stored in ontology.ttl.
---

# myonto

> **重要：本 skill 是自包含的**——本目录 `bin/myonto` 包含本工具的二进制副本（安装时由 `make install-skills` 复制）。所有命令使用**相对路径** `bin/myonto <subcommand>` 调用，**不要**依赖系统的 `PATH` 里的 `myonto`。
>
> **首次使用前**：cd 进一个工作目录，跑 `bin/myonto init` 完成 TUI 向导（或 `bin/myonto init --no-wizard` 走参数模式）。
> **默认 cwd**：myonto 操作的是当前目录的 `ontology.ttl`（git 友好，W3C Turtle 标准）。
>
> 具体调用方式见 [INSTALLATION.md](INSTALLATION.md)。

# myonto

A typed vocabulary + constraint CLI for managing a personal knowledge graph stored as standard W3C Turtle (`.ttl`). All data is plain-text and git-friendly.

## Core Concept

Everything is a **triple** on the working directory's `ontology.ttl`:

```
<subject>  <predicate>  <object>
```

- **Classes** are typed with `rdf:type` (e.g. `ex:Person`, `ex:Project`).
- **Entities** are individuals belonging to one or more classes.
- **Properties** are user-defined predicates (e.g. `:knows`, `:partOf`).
- **Comments** use `rdfs:comment`; **labels** use `rdfs:label`.

The local namespace IRI is configurable in `.myonto.toml` (default `ex:` → `http://example.org/`).

## When to Use

| Trigger phrase | Command |
|---|---|
| "remember that X is a Y" | `bin/myonto entity add <name> -t <class> -d "<desc>"` |
| "what do I know about X" | `bin/myonto entity show <name>` |
| "list all people" | `bin/myonto entity list -t Person` |
| "search for X" | `bin/myonto search <keyword>` |
| "link X to Y via knows" | `bin/myonto link X knows Y` |
| "unlink X from Y" | `bin/myonto unlink X knows Y` |
| "delete entity X" | `bin/myonto entity rm X -f` |
| "find what would follow from my data" | `bin/myonto reason` |
| "check for inconsistencies" | `bin/myonto check` |
| "what does X transitively depend on" | `bin/myonto closure <X> -p <pred>` |
| "what depends on X" | `bin/myonto closure <X> -p <pred> -r` |
| "how are A and B related" | `bin/myonto path <A> <B>` |
| "count entities by type / group by" | `bin/myonto query -w "?s a ?o" -g "?o" -c` |
| "show me a graph" | `bin/myonto graph` |
| "summarize this entity in plain English" | `bin/myonto ai summarize <name>` |
| "extract entities from this text" | `bin/myonto ai extract "<text>"` |
| "what can I ask about my data" | `bin/myonto ai qa "<question>"` |
| "configure which LLM I use" | `bin/myonto config llm set-key` |
| "what classes/relations exist" | `bin/myonto schema --json` |
| "summarize the ontology for context" | `bin/myonto export --for-llm` |
| "import this markdown as entities" | `bin/myonto import <file>` (then `bin/myonto entity apply <out.json>`) |
| "batch add these entities" | `bin/myonto entity apply <json-file>` |

## Output Format

All commands that read data accept `--json` (global flag) for machine-readable output. Use `--json` whenever you need to parse the result programmatically.

| Command | `--json` output shape |
|---|---|
| `bin/myonto list` | `{"count":N, "entities":[{local, iri, label, types[], desc}]}` |
| `bin/myonto search <kw>` | `{"count":N, "entities":[{local, iri, label, types[], desc, match_kind}]}` |
| `bin/myonto entity show <name>` | `{"entity":"<iri>", "count":N, "triples":[{subject, predicate, object:{type,value,lang?,datatype?}}]}` |
| `bin/myonto reason` | `{"saturated":bool, "derived":[{subject, predicate, object}], "will_apply":bool, "applied":N}` |
| `bin/myonto check` | `{"findings":[{severity, rule, subject, detail, evidence[]}], "errors":N, "warnings":N}` |
| `bin/myonto closure <e> -p <p>` | `{"seed":"<iri>", "predicate":"<iri>", "reverse":bool, "reachable":[{term, depth}], "count":N}` |
| `bin/myonto path <a> <b>` | `{"from":"<iri>", "to":"<iri>", "found":bool, "length":N, "path":[{subject, predicate, object}]}` |
| `bin/myonto query` | `{"patterns":[...], "group_by":"var", "count":bool, "results":[{key, count?}], "total":N}` |

## Setup

If no `.myonto.toml` exists in the working directory, the user must run `bin/myonto init` first. Prefer:

```bash
cd <project-dir>
bin/myonto init   # launches interactive TUI wizard (or `myonto init --no-wizard` for non-TTY)
```

## Core Workflows

### Create an entity

```bash
bin/myonto entity add isaac-newton -t Person -d "英国数学家、物理学家"
```

### Create a class with inheritance

```bash
bin/myonto entity add-class Scientist -p Person -d "科学家"
```

### Link two entities

```bash
bin/myonto link isaac-newton knows leibniz          # both are entities
bin/myonto link isaac-newton bornIn 1643 -l         # -l means literal
```

### Search and read

```bash
bin/myonto search newton
bin/myonto entity show isaac-newton
```

### Run inference

```bash
bin/myonto reason        # dry-run: show what would be derived
bin/myonto reason -a     # apply (writes back to ontology.ttl)
```

### Analyze the ontology

Four analysis commands consume the ontology as a knowledge graph:

```bash
# Consistency check — report disjointWith violations (incl. inherited conflicts)
bin/myonto check
bin/myonto check --strict     # CI gate: exit 1 on any error

# Transitive closure — impact analysis ("what does X depend on?")
bin/myonto closure cmd -p dependsOn           # forward
bin/myonto closure rdf -p dependsOn -r        # reverse ("who depends on X?")

# Shortest path — explain how A relates to B
bin/myonto path alice bob
bin/myonto path alice bob -p knows            # only via "knows"

# Lightweight query (SPARQL subset) — group by / count / top-N
# Includes inferred facts automatically (subclass instances, domain/range types)
bin/myonto query -w "?s a ?o" -g "?o" -c              # count by type
bin/myonto query -w "?s ex:bornIn ?o" -g "?o" -c -n 5 # top 5 birthplaces
bin/myonto query -w "?s ex:knows ?o" -w "?o ex:knows ex:carol"  # JOIN
```

### Generate an interactive graph

```bash
bin/myonto graph                                  # writes ontology-graph.html, auto-opens browser
bin/myonto graph --include-pred knows -o g.html   # filter to one relationship type
```

### LLM-assisted maintenance

All `ai` commands default to **dry-run** (output to stdout, no file change). Use `-a`/`--apply` to write back.

```bash
bin/myonto ai summarize aristotle                 # see LLM-generated summary
bin/myonto ai summarize aristotle -a              # write summary to rdfs:comment
bin/myonto ai extract "苏格拉底是柏拉图的老师"     # extract entities from text
bin/myonto ai suggest-relations aristotle         # propose new triples
bin/myonto ai qa "牛顿认识谁？"                    # Q&A over the graph
```

### Configure LLM provider

API keys are **encrypted with a machine-derived key** before being written to `.myonto.toml` (never stored in plaintext):

```bash
bin/myonto config llm list-providers             # see available presets
bin/myonto config llm set-key alibaba-coding      # interactive prompt (key hidden)
bin/myonto config llm show                        # check current config (key masked)
bin/myonto config llm test                        # send a test request
```

## TUI Mode

For interactive use, `bin/myonto tui` launches a terminal UI with a main menu (list, search, add, link, reason, graph, AI, etc.). Use this when a human is at the keyboard; use the CLI commands when an agent or script is calling.

## Storage

Default: `ontology.ttl` in the working directory (path configurable in `.myonto.toml`).
All data is git-friendly. To inspect or edit manually, use any text editor.

## Important Conventions

- **Entity names are slugified**: "Isaac Newton" → `isaac-newton`. Use English/pinyin for local names; store original Chinese in `rdfs:label` or `rdfs:comment`.
- **IRIs use the project namespace** from `.myonto.toml` (default `http://example.org/`). Cross-project references use the full IRI.
- **Predicates are arbitrary IRIs**; common ones are `knows`, `partOf`, `inspired`, `worksOn`. The system doesn't enforce a fixed property vocabulary.
- **Plain Turtle is the canonical format**; `--json` is for tool consumption only.

## References

- `references/commands.md` — Full command reference with all flags
