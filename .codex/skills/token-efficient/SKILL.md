---
name: token-efficient
description: Execute investigation, coding, review, or documentation tasks using a bounded, evidence-driven workflow that minimizes unnecessary working tokens while preserving correctness, required validation, and repository or workspace policies.
---

# Token-Efficient Workflow

Minimize unnecessary working tokens while maintaining correctness, required validation, and applicable project policies. Reduce exploration by working from evidence instead of curiosity.

## Operating Principles

- Treat each task as a bounded evidence-gathering problem.
- Start with an explicit budget:
  - **Small**: single file or straightforward answer.
  - **Standard**: localized change.
  - **Extended**: cross-component, high-risk, or architecture changes.
- Escalate the budget only when new evidence expands the required scope.
- Follow a **Search → Pinpoint Read → Change → Focused Verify** workflow.
- Stop investigating once additional context is unlikely to change the implementation or conclusion.
- Do not speculate. Base conclusions on code, documentation, tool output, or explicit user requirements.

## Workflow

### 1. Define the Goal

- Summarize the intended outcome in one sentence.
- Select the smallest suitable budget.
- Record only assumptions that materially affect the task.

### 2. Search First

- Begin with up to three targeted searches using symbols, paths, errors, or acceptance criteria.
- Add searches only when each new search resolves a newly discovered dependency.
- Avoid broad workspace scans unless the task explicitly requires inventory.

### 3. Read Only What Answers the Question

- Search before opening files.
- Read the smallest coherent range that answers the current question.
- Expand the range only when the current context is incomplete.
- Avoid reading entire large files unless the task genuinely requires full-file context.

### 4. Inspect Dependencies Incrementally

- Inspect direct dependencies first.
- Expand transitively only when correctness depends on it.
- Prefer stable interfaces and contracts before implementation details.

### 5. Keep Tool Output Small

- Batch independent read-only checks where practical.
- Request only the fields, symbols, or ranges needed.
- Discard irrelevant output instead of carrying it forward.

### 6. Make the Smallest Coherent Change

- Implement a single focused patch.
- Preserve unrelated changes.
- Re-search affected symbols rather than rereading unchanged files.

### 7. Verify Proportionally

- Run the narrowest deterministic validation that proves the change.
- Start with formatting and the smallest affected tests or package.
- Expand validation only when:
  - focused validation fails,
  - repository policy requires broader validation,
  - or the change affects high-risk areas such as authentication, authorization, persistence, calculations, logging, or public APIs.
- Never claim verification that was not performed.

### 8. Finish Early

Stop when:

- acceptance criteria are satisfied,
- required validation has completed,
- and additional investigation is unlikely to change the outcome.

## Context Rules

- Treat user-provided paths, symbols, errors, and acceptance criteria as the initial search boundary.
- Prefer concise fact summaries over large code or log excerpts.
- Avoid rereading unchanged ranges.
- Read governing instructions (for example AGENTS.md) only when they apply to the task.
- Do not expand investigation without evidence that additional context is required.

## Compression Patterns

| Need | Default Action | Escalate When |
|------|----------------|---------------|
| Locate code | Targeted symbol search | No direct match exists |
| Understand logic | Read declaration plus one caller/callee | Control flow crosses boundaries |
| Inspect API | Read request/response contract first | Behavior depends on implementation |
| Investigate bug | Start from error or stack trace | Root cause remains unresolved |
| Check impact | Search affected public symbols | Additional consumers appear |
| Verify change | Formatter + narrowest deterministic validation | Validation fails or policy requires more |
| Report | Conclusions only | User requests detailed analysis |

## Reporting

Return concise results containing:

1. Outcome
2. Files changed
3. Verification performed
4. Assumptions or TODOs
5. Material risks (if any)

Report conclusions rather than replaying the investigation process.

## Guardrails

- Follow repository, workspace, security, and project policies.
- Do not skip required documentation, formatting, or validation.
- Required validation always overrides the token budget.
- Prefer implementations requiring the least additional context when multiple valid solutions exist.
- Do not infer behavior without supporting evidence.
