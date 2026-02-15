# wgpilot Documentation Foundation — Master Index

## What You Have (Complete)

| # | File | Location in Repo | Status |
|---|---|---|---|
| 1 | `CLAUDE.md` | `/CLAUDE.md` (project root) | **Place now** |
| 2 | Split spec docs | `docs/` (tree) | **Done** |
| 3 | Logging spec | `docs/operations/logging-debugging.md` | **Place now** |
| 4 | Security spec | `docs/security-spec.md` | **Place now** |
| 5 | Testing strategy | `docs/testing-strategy.md` | **Place now** |
| 6 | Implementation roadmap | `docs/implementation-roadmap.md` | **Place now** |

## Claude Code Instruction Prompts (Execute in Order)

| # | File | When | What It Does |
|---|---|---|---|
| — | `implementation-roadmap.md` | **Reference** | Contains all 13 phase prompts — copy each phase's prompt into Claude Code when ready |
| A | `implement-logging-prompt.md` | **Phase 1** | Detailed logging implementation (merged into Phase 1 of roadmap) |

## Repo Structure After Placing All Files

```
wgpilot/
├── CLAUDE.md                              ← Claude Code reads this automatically
├── docs/
│   ├── OVERVIEW.md                        ← from spec split
│   ├── architecture/
│   │   ├── tech-stack.md
│   │   ├── data-model.md
│   │   ├── api-surface.md
│   │   └── project-structure.md
│   ├── features/
│   │   ├── install-script.md
│   │   ├── first-run.md
│   │   ├── network-management.md
│   │   ├── peer-management.md
│   │   ├── monitoring.md
│   │   ├── auth.md
│   │   └── multi-network.md
│   ├── operations/
│   │   ├── service.md
│   │   ├── tls.md
│   │   ├── updates.md
│   │   └── logging-debugging.md           ← new
│   ├── decisions/
│   │   └── adr-001-no-mesh.md
│   ├── security-spec.md                   ← new
│   ├── testing-strategy.md                ← new
│   └── implementation-roadmap.md          ← new
├── SPEC.md.bak                            ← original backup
├── cmd/
│   └── wgpilot/
├── internal/
├── frontend/
└── ...
```

## Execution Order

```
Step 1:  Place CLAUDE.md in repo root
Step 2:  Place logging-debugging.md, security-spec.md, testing-strategy.md,
         implementation-roadmap.md in docs/
Step 3:  Open implementation-roadmap.md
Step 4:  Copy Phase 0 prompt into Claude Code → execute → verify → commit
Step 5:  Copy Phase 1 prompt into Claude Code → execute → verify → commit
         (supplement with implement-logging-prompt.md for extra detail)
Step 6:  Continue phase by phase through Phase 12
Step 7:  Tag v0.1.0
```

## What Each Document Prevents

| Document | Prevents |
|---|---|
| `CLAUDE.md` | Inconsistent code style, architectural drift, global state, bare errors, wrong package boundaries |
| `logging-debugging.md` | Insufficient debug info, unstructured logs, missing request correlation, cryptic errors |
| `security-spec.md` | Auth bypasses, SQL injection, XSS, missing input validation, credential logging |
| `testing-strategy.md` | Wrong test level, missing critical scenarios, untestable code, no mocks |
| `implementation-roadmap.md` | Wrong build order, too-large Claude Code sessions, missing verification steps |
| Split spec docs | Context window overload, hallucinated features, missed requirements |
