# controllerless

**An agent runtime that watches Kubernetes resources and reconciles them via natural-language *skills*, replacing the role of custom controllers.**

This is a paradigm-shift POC, not a pragmatic production system. The thesis: business logic that has historically lived in Go controllers can instead live in natural-language skill documents that an LLM-backed agent loads, dispatches, and executes against real Kubernetes APIs.

Target cluster: **Kubernetes v1.35**. Default LLM: **gemma4:12b-mxfp8** via Ollama (local).

Full design rationale: see [DESIGN.md](./DESIGN.md).

---

## Architecture

```
Skill loader (fsnotify hot-reload)
    │  GVK → []*Skill trigger index
    ▼
Dynamic informer factory          ← one shared informer per GVK
    │  Add / Update / Delete events
    ▼
Per-GVK rate-limited workqueue    ← dedup by NamespacedName; exponential backoff
    │  pop(key)
    ▼
Dispatcher
    │  fetch from cache → evaluate skill conditions → collect matches
    │  for each matching skill, run independently (no DAG, no planner)
    ▼
Reconcile session per (key, skill)
    │  prompt = system prompt + skill body + event payload
    │  agentic tool loop: LLM ↔ tool registry
    │  until: done() | requeueAfter() | max turns | timeout
    ▼
JSONL trace + slog + K8s Events
```

**Package responsibilities:**

| Package | Responsibility |
|---|---|
| `cmd/controllerless/` | Entry: config + wiring + signal handling |
| `internal/config/` | Env vars + flags → `Config` struct |
| `internal/skill/` | Load, parse frontmatter, lint L1–L8, trigger index, hot-reload |
| `internal/kube/` | Dynamic client, REST mapper, informer factory, workqueue registry, Events recorder |
| `internal/llm/` | `Provider` interface, Ollama impl, mock impl |
| `internal/tools/` | Primitive tool registry (K8s CRUD, cron/time helpers, meta) |
| `internal/dispatch/` | Event router, agentic tool loop, four safeguards, prompt assembly |
| `internal/trace/` | JSONL writer (Tier 2), slog setup (Tier 1) |
| `internal/crd/scheduledtask/` | CRD manifest for `ScheduledTask` |

---

## Skill Model

### Structure

A skill is a directory under `--skills-dir` containing `SKILL.md` with YAML frontmatter + Markdown body.

```yaml
---
name: scheduledtask-fire-due-pod        # must match parent directory name (L8)
description: One-line summary for dispatcher LLM picker.
triggers:
  - gvk: agentic.io/v1alpha1/ScheduledTask
    eventTypes: [Added, Modified]       # subset of {Added, Modified, Deleted}
    conditions:                         # CEL expressions evaluated at dispatch time, not watch time
      - "spec.suspend != true"
allowedTools:                           # enforced at dispatch — not in prompt
  - get
  - list
  - patch
  - create
  - cron.nextFireTime
  - time.secondsUntil
  - time.toUnix
  - requeueAfter   # terminal: schedules re-enqueue and ends session
  - done           # terminal: ends session successfully (no requeue)
---
```

### Body skeleton (required shape)

```markdown
# What this skill does
<1–2 sentences>

# Procedure

1. **Stability check.** <If already converged, call done() and return.>
2. ...

# Errors
<Skill-specific error handling beyond harness defaults.>

# Examples (optional)
<Annotated trace(s) used as few-shot demonstrations.>
```

### Lint rules (enforced at load; failure drops the skill, agent continues)

| Rule | Requirement |
|---|---|
| L1 | Step 1 is a stability check |
| L2 | Each numbered step starts with an imperative verb |
| L3 | Tool references in backticks: `` `toolName(...)` `` |
| L4 | ≤ 8 numbered steps |
| L5 | Conditional nesting ≤ 1 level deep |
| L6 | All tools in the body must be in `allowedTools` |
| L7 | Body has `# What this skill does` and `# Procedure` sections |
| L8 | `name:` matches parent directory name |

### Storage and hot-reload

- Default skills directory: `~/.controllerless/skills/`. Override: `--skills-dir`.
- Repo ships demo skills in `./skills/`.
- `fsnotify` watches recursively. On `SKILL.md` change: re-parse + lint → atomic registry swap. Lint failure keeps previous version alive.
- In-flight reconciles complete with the old body; new reconciles use the new body.

---

## Demo CRD: `ScheduledTask`

Agentic-native design: minimal typed fields, append-only status log, label-combo dedup. Does **not** follow the kubebuilder CronJob schema.

### Spec

```yaml
apiVersion: agentic.io/v1alpha1
kind: ScheduledTask
metadata:
  name: nightly-backup
  namespace: default
spec:
  schedule: "0 2 * * *"    # 5-field cron expression
  podSpec:                  # direct PodSpec — no Job wrapper
    containers:
      - name: backup
        image: my-org/pg-backup:latest
  historyLimit: 5
  suspend: false
```

### Status

```yaml
status:
  nextFireTime: "2026-06-12T02:00:00Z"
  fires:                    # append-only log (x-kubernetes-list-type: map, key: scheduledAt)
    - scheduledAt: "2026-06-11T02:00:00Z"
      podName: nightly-backup-1717984800-x7yz
      phase: Succeeded      # mirrors Pod.status.phase
      note: "exit 0"        # free-form LLM-friendly note
  conditions:
    - type: Ready
      status: "True"
      reason: Scheduled
      lastTransitionTime: "2026-06-11T02:00:05Z"
```

### Pod labels and naming (invariant — do not change)

Every Pod created by the agent MUST carry:

```yaml
metadata:
  generateName: <task-name>-<scheduledAtUnix>-
  labels:
    agentic.io/scheduledtask: <task-name>        # dedup label 1
    agentic.io/scheduled-at: "<scheduledAtUnix>" # dedup label 2
  ownerReferences:
    - apiVersion: agentic.io/v1alpha1
      kind: ScheduledTask
      controller: true
      blockOwnerDeletion: true
```

Idempotency: before `create`, always `list` by the label combo and skip if non-empty.

---

## Tool Surface

### K8s primitives

```
get(gvk, namespace, name) -> object
list(gvk, namespace, labelSelector?, fieldSelector?) -> []object

patch(gvk, namespace, name, patch, type=SSA, fieldManager?, subresource?) -> object
create(object) -> object
delete(gvk, namespace, name, propagationPolicy=Background) -> ok
```

`patch` defaults to **server-side apply**. `fieldManager` defaults to `skill/<name>`. Status writes must use `subresource=status`.

### Pure-function helpers (no I/O, no side effects)

```
cron.nextFireTime(expr, after) -> time.Time
time.parseDuration(s) -> time.Duration
time.secondsUntil(t) -> int
time.since(t) -> int
time.now() -> time.Time
time.toUnix(t) -> int64
time.fromUnix(unix) -> time.Time
```

### Meta (control flow)

```
done(summary) -> terminates reconcile session
requeueAfter(seconds, reason) -> schedules re-enqueue; terminates session
```

**No composite tools** (e.g., no `createOwnedJob`). Invariants are expressed in skill prose and enforced by CRD design. See DESIGN.md §7.4.

---

## LLM Provider

### Go interface

```go
type Provider interface {
    Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
    Name() string
}

type ChatRequest struct {
    Model       string
    Messages    []Message
    Tools       []ToolDef
    Temperature float32
    MaxTokens   int
}

type Message struct {
    Role       Role          // system | user | assistant | tool
    Content    string
    ToolCalls  []ToolCall
    ToolCallID string
    Name       string
}

type ToolCall struct {
    ID        string
    Name      string
    Arguments json.RawMessage  // opaque at interface boundary; dispatcher unmarshals
}
```

Implementations: `providers/ollama` (native `/api/chat`, not OpenAI-compat), `providers/mock` (for tests).

### Prompt structure

1. **System message (harness-owned, fixed):**
   ```
   You are an autonomous Kubernetes reconciler.
   You will be given (a) a single triggering event and (b) one skill describing what to do.
   Use only the provided tools to gather state and make changes.
   Before acting, check current state — if the desired outcome is already true, call done().
   When all work is finished, call done(). If you need to be retried later, call requeueAfter().
   Tool errors are real — if a tool returns an error, do not retry the same tool with the same arguments.
   ```
2. **Second system message:** skill body from `SKILL.md`.
3. **User message:** event type + full object YAML + cluster context + timestamp.

### Defensive parsing (in dispatcher, not provider)

| Failure | Recovery |
|---|---|
| Malformed JSON in Arguments | Append tool error message; continue loop |
| Unknown / disallowed tool name | Append tool error message; continue loop |
| Same (tool, argsHash) called >3 times | Force-stop; requeue with backoff |
| Exceeded `AGENT_MAX_TOOL_TURNS` | Requeue with backoff; log full transcript at WARN |

---

## Multi-Skill Safeguards

Skills are independent — no planner, no DAG. Composition happens through shared cluster state, exactly like kube-controller-manager.

Four mandatory safeguards:

| # | Safeguard | Where enforced |
|---|---|---|
| S1 | Idempotency-as-prose: Step 1 = stability check | Lint rule L1 |
| S2 | SSA with `fieldManager=skill/<name>` | `patch` tool default |
| S3 | Per-(resource, skill) token bucket rate limit | Dispatcher; default 1 fire per 5s |
| S4 | CEL trigger conditions | Frontmatter; skill conditions become false after convergence |

---

## Observability

**Three tiers:**

**Tier 1 — stdout (`slog` JSON):** INFO = high-signal events. DEBUG = tool calls. TRACE = full prompts.

**Tier 2 — JSONL traces (primary debugging artifact):**
```
~/.controllerless/traces/<YYYY-MM-DD>/<ns>__<gvk>__<name>__<unix>.jsonl
```
One file per reconcile. Contains: dispatch decision, matched skills, full prompts, LLM responses, tool calls + results, K8s mutations (verb + request body + response status), outcome, requeue. Walk this file to debug anything.

**Tier 3 — Kubernetes Events (sparing):** `source=skill/<name>`. Failure events include the trace path.

---

## Demo Skills

| Skill | Triggers | Purpose |
|---|---|---|
| `scheduledtask-fire-due-pod` | `ScheduledTask` Add/Modify | Compute next fire time; create Pod; idempotent via label combo; requeueAfter |
| `scheduledtask-update-fire-status` | `Pod` Modify with `agentic.io/scheduledtask` label | Patch matching `status.fires[]` entry on owning ScheduledTask |
| `scheduledtask-prune-history` | `ScheduledTask` Modify where fires > historyLimit | Sort, drop oldest, delete orphan Pods, patch status |
| `scheduledtask-auto-suspend-on-failures` | `ScheduledTask` Modify where last 3 fires are Failed | **Judgment skill.** Patch `spec.suspend=true`; append Suspended condition |

First three reimplement the kubebuilder CronJob controller in prose. Fourth adds a behavior that doesn't exist in kubebuilder.

---

## Environment Variables

```
LLM_PROVIDER=ollama
LLM_BASE_URL=http://localhost:11434
LLM_MODEL=gemma4:12b-mxfp8
LLM_API_KEY=                           # reserved
LLM_TEMPERATURE=0.2
LLM_MAX_TOKENS=4096
LLM_TIMEOUT=120s
LLM_NUM_CTX=16384

AGENT_MAX_TOOL_TURNS=10
AGENT_RECONCILE_TIMEOUT=5m
AGENT_PER_SKILL_RATE_LIMIT=5s

KUBECONFIG=$HOME/.kube/config
SKILLS_DIR=./skills
TRACES_DIR=~/.controllerless/traces
LOG_LEVEL=info
```

---

## Project Layout

```
controllerless/
├── cmd/controllerless/main.go
├── internal/
│   ├── config/
│   ├── skill/          loader.go  parse.go  lint.go  trigger.go  types.go
│   ├── llm/            provider.go  types.go  providers/{ollama,mock}/
│   ├── tools/          registry.go  k8s.go  cron.go  timefn.go  meta.go
│   ├── dispatch/       dispatcher.go  reconcile.go  safeguards.go  prompt.go
│   ├── kube/           client.go  informers.go  workqueue.go  events.go
│   ├── trace/          jsonl.go  log.go
│   └── crd/scheduledtask/crd.yaml
├── skills/
│   ├── scheduledtask-fire-due-pod/SKILL.md
│   ├── scheduledtask-update-fire-status/SKILL.md
│   ├── scheduledtask-prune-history/SKILL.md
│   └── scheduledtask-auto-suspend-on-failures/SKILL.md
├── examples/nightly-backup.yaml
├── go.mod              # module github.com/starbops/controllerless; go 1.25
├── Makefile
├── CONTEXT.md          # this file
└── DESIGN.md           # full rationale
```

**`go.mod` key deps:**
- `k8s.io/apimachinery`, `k8s.io/client-go`, `k8s.io/api` → `v0.35.3`
- `github.com/ollama/ollama` (native client, pin at scaffold time)
- `github.com/robfig/cron/v3`, `github.com/fsnotify/fsnotify`, `gopkg.in/yaml.v3`, `github.com/oklog/ulid/v2`
- **No `sigs.k8s.io/controller-runtime`** — dynamic client + raw workqueue only.

---

## Quick Start

```sh
make install-crd      # apply ScheduledTask CRD to cluster
make run              # build + run agent (uses env var defaults above)
make example-task     # kubectl apply examples/nightly-backup.yaml
```

Full demo loop: `make install-crd && make run`, then in another terminal `make example-task`.
Watch `~/.controllerless/traces/` for JSONL files as reconciles fire.

---

## Key Invariants (do not violate)

- Every Pod created for a ScheduledTask MUST have both dedup labels and the owner reference.
- Skills MUST only write to `/status` via `subresource=status` (SSA). Never update the whole object.
- `allowedTools` in frontmatter is a security boundary, not a hint. Dispatcher enforces it; don't work around it in prose.
- Step 1 of every skill body MUST be a stability/idempotency check. This is a lint rule (L1) and a runtime correctness requirement.
- The reconcile loop MUST terminate with either `done()` or `requeueAfter()`. No infinite loops.

---

## Deferred (v2)

Skills-as-CRDs (`kind: Skill`), leader election, user-defined custom tools (plugins/wasm), additional LLM providers (OpenAI/Anthropic), startup catch-up for missed schedules, OTel/Prometheus, per-skill RBAC.

---

## References

- [DESIGN.md](./DESIGN.md) — full decision rationale
- [Kubebuilder CronJob tutorial](https://book.kubebuilder.io/cronjob-tutorial/cronjob-tutorial.html) — reference behavior
- [Kubebuilder CronJob controller implementation](https://book.kubebuilder.io/cronjob-tutorial/controller-implementation.html)
- [Gemma 4 on Ollama](https://ollama.com/library/gemma4)
- [Ollama `/api/chat` with tools](https://github.com/ollama/ollama/blob/main/docs/api.md)
- [client-go dynamic informers](https://pkg.go.dev/k8s.io/client-go/dynamic/dynamicinformer)
- [client-go workqueue](https://pkg.go.dev/k8s.io/client-go/util/workqueue)
- [Server-side apply](https://kubernetes.io/docs/reference/using-api/server-side-apply/)
