# Claude Code Instruction: Split SPEC.md into Structured Document Tree

## Context

You are working on `wgpilot`, a WireGuard management tool written in Go with an embedded React SPA frontend. The project has a single `SPEC.md` file (~2000 lines) that contains the complete specification. Your task is to split it into a structured document tree that is optimized for incremental, focused development sessions where only relevant spec files are loaded.

## Goal

Split `SPEC.md` into self-contained, focused documents. Each document must be independently readable — a developer should be able to pick up any single file and understand the feature or component it describes without needing to read the entire spec.

## Target Structure

Create the following structure. If a topic from SPEC.md does not map cleanly to one of these files, use your judgment to place it in the most relevant file or create a new file under the appropriate directory. Do NOT discard any content from the original spec.

```
docs/
├── OVERVIEW.md
├── architecture/
│   ├── tech-stack.md
│   ├── data-model.md
│   ├── api-surface.md
│   └── project-structure.md
├── features/
│   ├── install-script.md
│   ├── first-run.md
│   ├── network-management.md
│   ├── peer-management.md
│   ├── monitoring.md
│   ├── auth.md
│   └── multi-network.md
├── operations/
│   ├── service.md
│   ├── tls.md
│   └── updates.md
└── decisions/
    └── adr-001-no-mesh.md
```

## Rules for Each Document

### OVERVIEW.md (max 120 lines)

This is the entry point. It must contain:

- Project name, one-sentence description
- Architecture summary (3-5 sentences)
- Table listing every sub-document with: file path, one-line purpose, and which other docs it relates to
- Build phases table mapping each phase to its required spec files:

```markdown
| Phase | Task | Spec Files |
|-------|------|------------|
| 1 | SQLite schema + repository layer | data-model.md |
| 2 | CLI, systemd, config loading | service.md, project-structure.md |
| 3 | JWT auth + admin setup | auth.md |
| 4 | Network CRUD + WG interface mgmt | network-management.md, data-model.md |
| 5 | Peer CRUD + config generation + QR | peer-management.md, data-model.md |
| 6 | Dashboard API + SSE + metrics | monitoring.md |
| 7 | Setup wizard (frontend) | first-run.md, auth.md |
| 8 | Install script | install-script.md |
| 9 | TLS + ACME | tls.md |
| 10 | Multi-network + bridging | multi-network.md, network-management.md |
| 11 | Self-update mechanism | updates.md |
```

Do NOT put detailed specs in OVERVIEW.md. It is a map, not a document.

### Every Sub-Document

Every sub-document must follow this template:

```markdown
# [Title]

> **Purpose**: One sentence explaining what this document specifies.
>
> **Related docs**: List of other doc file paths this document depends on or connects to.
>
> **Implements**: Which Go packages or frontend directories this spec maps to.

---

[Content from SPEC.md, reorganized for this topic]
```

### Content Rules

1. **No duplication.** If the data model is referenced in peer-management.md, write "See [data-model.md](../architecture/data-model.md) for the Peer entity schema" — do NOT copy the schema into peer-management.md. The only exception: if a document needs a small subset (under 10 lines) of another document's content to be readable standalone, you may include it with a note like "Excerpt from data-model.md for context."

2. **No orphaned content.** Every line of the original SPEC.md must exist in exactly one sub-document. After splitting, concatenating all sub-documents must cover 100% of the original spec's information. If content doesn't fit any planned file, create a new appropriately named file and add it to OVERVIEW.md.

3. **No vague cross-references.** When referencing another document, always use a relative path link: `[data-model.md](../architecture/data-model.md)` — never just "see the data model doc."

4. **Preserve all technical detail.** Do not summarize, shorten, simplify, or paraphrase any technical content. Code blocks, schemas, config examples, API endpoint definitions, CLI commands, and data structures must be transferred verbatim. The split is a reorganization, not a rewrite.

5. **Preserve all decisions and rationale.** If the spec says "we chose X over Y because Z", that rationale must appear in the relevant sub-document, or in an ADR file under `decisions/`.

6. **Consistent heading hierarchy.** Each document uses `#` for its title (one per file), `##` for major sections, `###` for subsections. No deeper than `####`.

7. **Each document should be under 300 lines.** If a topic exceeds 300 lines, split it further into logical sub-documents and update OVERVIEW.md accordingly. For example, if `monitoring.md` is too long, split into `monitoring-dashboard.md`, `monitoring-metrics.md`, and `monitoring-alerts.md`.

## Content Mapping Guide

Use this as a guide for where content from SPEC.md should land. Adapt based on actual content:

| Topic in SPEC.md | Target Document |
|---|---|
| Tech stack choices, framework rationale, library choices | `architecture/tech-stack.md` |
| Database tables, entities, relationships, SQL schemas, enums | `architecture/data-model.md` |
| REST endpoints, request/response shapes, status codes | `architecture/api-surface.md` |
| Directory layout, Go packages, frontend structure, build pipeline | `architecture/project-structure.md` |
| Install script, OS detection, prerequisites, binary download | `features/install-script.md` |
| Setup wizard, first-run flow, steps, edge cases, import existing WG | `features/first-run.md` |
| Network CRUD, topology modes (gateway/site-to-site/hub), interface lifecycle | `features/network-management.md` |
| Peer CRUD, key generation, config file generation, QR codes, AllowedIPs logic | `features/peer-management.md` |
| Dashboard, live stats, SSE, Prometheus metrics, peer snapshots, historical data, alerts | `features/monitoring.md` |
| Authentication, JWT, sessions, bcrypt, admin accounts, login flow | `features/auth.md` |
| Multiple WG interfaces, network isolation, bridging, nftables forwarding between networks | `features/multi-network.md` |
| Systemd unit, capabilities, filesystem layout, CLI subcommands, signals, lifecycle | `operations/service.md` |
| HTTPS, self-signed certs, ACME/Let's Encrypt, manual cert config | `operations/tls.md` |
| Self-update, version detection, binary replacement, GoReleaser | `operations/updates.md` |
| Mesh networking decision and rationale | `decisions/adr-001-no-mesh.md` |

## Execution Steps

1. Read the entire SPEC.md first. Do not start writing until you have read everything.
2. Create the `docs/` directory structure.
3. Create OVERVIEW.md as the table of contents with the build phases table.
4. Work through SPEC.md top to bottom, placing each section's content into the correct sub-document.
5. For each sub-document, add the header template (Purpose, Related docs, Implements).
6. After all content is placed, review each sub-document for:
   - Standalone readability (can someone understand this without reading other docs?)
   - Cross-references use relative links
   - No duplicated content
   - No missing content
   - Line count is under 300
7. Verify completeness: confirm that no content from the original SPEC.md was lost.
8. Rename the original SPEC.md to SPEC.md.bak (do not delete it).

## Validation

After completing the split, run this self-check:

- [ ] Every sub-document has the header template (Purpose, Related docs, Implements)
- [ ] OVERVIEW.md has a complete file listing with one-line descriptions
- [ ] OVERVIEW.md has the build phases table
- [ ] No sub-document exceeds 300 lines
- [ ] No technical detail from SPEC.md was summarized or lost
- [ ] All cross-references use relative markdown links
- [ ] No content is duplicated across files (except small excerpts noted as such)
- [ ] The original SPEC.md is preserved as SPEC.md.bak
