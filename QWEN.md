# QWEN.md — RAG

## INHERITED FROM constitution/QWEN.md

**The inheritance below is conditional. Both cases are stated; neither is
assumed.**

When this module is consumed inside a project that includes the Helix
Constitution submodule, the rules in `constitution/QWEN.md` — and in the
`constitution/Constitution.md` it references — are authoritative for every
topic not covered here. The module-local rules below extend them; they never
weaken or override them.

When this module is consumed standalone — cloned on its own, with no
constitution reachable in any parent — there is nothing to inherit, and **only
the module-local rules below apply**.

### Locating the base file: a resolver, never a path

`constitution/QWEN.md` in the heading above is the **canonical name of the base
file**, written exactly as the constitution's own examples write it. It is not
a filesystem path relative to this module, and it must not be rewritten into
one:

- a consuming project may mount the constitution under more than one layout,
  and this module cannot know which one it got;
- the same commit of this module can be checked out at two different depths at
  the same time, so no single relative path is correct for both;
- a standalone clone has no constitution anywhere, so any hardcoded path would
  simply dangle.

Resolve it at run time with the constitution's own parent-walk resolver,
**`find_constitution.sh`**. It walks up the parent chain trying each layout the
constitution supports, follows
`git rev-parse --show-superproject-working-tree` out of nested submodules so it
works from any nested depth, and exits non-zero with an explicit error when no
constitution is reachable — which is precisely the standalone case above.

This file therefore hardcodes **no** parent-project path and **no**
depth-dependent path, keeping the module project-not-aware, decoupled and
reusable per §11.4.28(B). Agent tooling with a native file-import syntax must
not turn the heading into one: an `@constitution/QWEN.md` import resolves
relative to *this* file, so inside a module it points at a path that does not
exist and silently resolves to nothing.

Canonical reference:
<https://github.com/HelixDevelopment/HelixConstitution>

## Module-local notes

This carrier is read by Qwen Code.

See [`README.md`](README.md) for what this module is and how it is used.
Module-specific rules go below this line; they extend the inherited base rules
and never weaken them.

### What this module is

`digital.vasic.rag` is a standalone, reusable Go library of **retrieval**
primitives: it turns a corpus and a query into a ranked list of documents. It
ships no `main`, no server, no vector store and no embedding model, and it is
deliberately project-not-aware (§11.4.28(B)) — it carries no consuming
project's vocabulary, corpus or filesystem paths, and none may be added.

The identity type every stage passes around is:

```go
type Document struct {
    ID       string         `json:"id"`
    Content  string         `json:"content"`
    Metadata map[string]any `json:"metadata,omitempty"`
    Score    float64        `json:"score,omitempty"`
    Source   string         `json:"source,omitempty"`
}
```

and the contract every source of documents satisfies is one method:

```go
type Retriever interface {
    Retrieve(ctx context.Context, query string, opts Options) ([]Document, error)
}
```

Package map, measured in this tree — re-derive with `ls pkg/` and
`go build ./...`:

| Package | What it holds |
|---|---|
| `pkg/retriever` | `Document`, `Options`, the `Retriever` interface, and `MultiRetriever` — parallel fan-out across several retrievers with score-keyed deduplication |
| `pkg/chunker` | `FixedSizeChunker`, `RecursiveChunker`, `SentenceChunker`, and the `Chunk` type carrying byte offsets back into the source text |
| `pkg/reranker` | `ScoreReranker` and `MMRReranker` — Maximal Marginal Relevance, trading relevance against diversity |
| `pkg/hybrid` | `KeywordRetriever` (BM25), `SemanticRetriever`, `HybridRetriever`, and the Reciprocal-Rank-Fusion and weighted-linear fusion strategies |
| `pkg/pipeline` | the fluent builder chaining retrieve → rerank → format, plus arbitrary custom stages |

What this module deliberately does **not** ship, because the consumer supplies
it: a vector store — `SemanticRetriever` wraps any `Retriever` the consumer
provides — and an embedding model.

### Module-local rules

These extend the inherited base rules; they weaken nothing.

1. `retriever.Document` is this module's identity type for one retrieved unit
   of content, and every stage downstream of chunking consumes and produces it.
   A consumer needing richer per-document facts puts them in `Metadata`; adding
   a field to `Document` changes the contract for every stage at once.
2. The `Retriever` interface is one method and stays one method. New retrieval
   behaviour is a new implementation or a wrapper, never a wider interface
   (§11.4.28 decoupling).
3. Nothing consumer-specific may enter this module — no consuming project's
   corpus, domain vocabulary or paths (§11.4.28(B)). A retrieval feature that
   cannot be described without naming a particular consumer belongs in that
   consumer, not here.
4. Ranking changes are behaviour changes. Any edit to BM25 scoring, to an MMR
   lambda default, or to a fusion constant lands together with a test that pins
   the resulting ordering: a ranking regression is invisible to a test that
   only asserts that some documents came back (§11.4, §11.4.6).
5. Every corpus and fixture committed to this repository is synthetic. No real
   user content, no personal data, no third-party copyrighted text — a
   retrieval library is exactly the place where such material would leak
   silently into consumers.
6. Every claim about this module carries the command that re-derives it and
   that command's output (§11.4.6).

### Build and test

```bash
go build ./...
go test ./... -count=1 -race
go vet ./...
gofmt -l .
```

### Honest boundaries (§11.4.6)

- The cascaded constitutional corpus that the agent carriers of this repository
  previously each carried inline is **not** reproduced here. It is inherited by
  reference through the pointer above (§11.4.28 / §11.4.177) and additionally
  retained in this repository's own `CONSTITUTION.md`. Nothing was dropped from
  the repository; it stopped being duplicated.
- Two claims made by the carrier text this file replaces are **withdrawn**, not
  restated. First, that this module "is part of the HelixPlay system", with a
  link to a feature spec in another organisation's repository: that is exactly
  the project-awareness §11.4.28(B) forbids in a reusable module, and it is
  false of a library that no more belongs to one consumer than to any other.
  Second, that `origin` fetches from GitHub and pushes to GitFlic with "four
  remotes configured": this checkout has a single remote, `origin`, fetching
  and pushing the same URL. Re-derive with `git remote -v`.
