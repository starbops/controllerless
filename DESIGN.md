# controllerless — Design Document

> **One-liner**: An agent runtime that watches Kubernetes resources and reconciles them via natural-language *skills*, replacing the role of custom controllers.

This document captures the design decisions reached during a stress-test grilling session. It is intended to be sufficient for a new session (with no prior conversation context) to implement the POC from scratch.

---

## 1. Premise

### Thesis

Software-engineering paradigm shift: business logic that has historically lived in Go controllers (Reconcile loops, state machines, generated typed clients) can instead live in **natural-language skill documents** that an LLM-backed agent loads, dispatches, and executes against Kubernetes resources.

**This is a paradigm-shift demo, not a pragmatic production system.** The intent is to provoke a conversation about *where business logic should live*, not to recommend replacing kube-controller-manager.

### Scope of the POC

| In scope | Out of scope (deferred to v2 or beyond) |
|---|---|
| Single-binary agent run locally against a remote cluster via kubeconfig | Leader election / multi-replica |
| Filesystem-loaded skills with hot-reload (`fsnotify`) | Skills-as-CRDs in-cluster |
| Pluggable LLM provider via env vars; Ollama provider implemented | OpenAI / Anthropic providers (interface is ready) |
| Dynamic-client-based watch on arbitrary GVKs declared by skills | Typed reconcilers, codegen, controller-runtime |
| Demo CRD: `agentic.io/v1alpha1 ScheduledTask` (cron-like) | Bridge to existing K8s controllers (e.g., disabling kube-controller-manager's CronJob handler) |
| Four demo skills (three reimplementing CronJob, one judgment overlay) | User-defined custom tools (Go plugins / wasm) |
| Three-tier observability (slog + JSONL traces + K8s Events) | OpenTelemetry traces, Prometheus metrics |
| Per-reconcile JSONL trace as the primary demo artifact | Startup catch-up for missed schedules during downtime |

### Audience context

- Author: Zespre, Staff Software Engineer at SUSE (Harvester team)
- Tech stack: Go, Kubernetes, Ollama
- Demo audience: cloud-native / Kubernetes engineers familiar with kubebuilder
- Expected reaction: provocative pushback. The doc is structured so the author can defend each decision.

---

## 2. Decision Summary

The full design distilled into a single table. Each row links to the detailed section below.

| # | Decision | Choice | Section |
|---|---|---|---|
| 1 | Demo framing | Paradigm-shift demo, audience-grilled | §3 |
| 2 | Skill structure | Frontmatter + prose body + bounded `allowedTools` | §5 |
| 3 | Skill storage | Local filesystem, hot-reload, mirrors Claude Code layout | §5.5 |
| 4 | Watch machinery | Dynamic informer factory + per-GVK workqueue | §8 |
| 5 | LLM context | Agentic tool loop, no pre-fetched context | §8.3 |
| 6 | Scheduling | Deterministic helper tools (`cron.*`, `time.*`) alongside K8s tools | §7 |
| 7 | Demo scope | Decomposed skills replacing controller + one judgment overlay | §11 |
| 8 | Multi-skill orchestration | Independent skills, controller-style convergence + 4 safeguards | §8.4 |
| 9 | Invariant enforcement | Primitives only — no composite tools; reshape the CRD instead | §7.4 |
| 10 | CRD design | New `ScheduledTask`: direct PodSpec, append-only `status.fires`, label-combo dedup | §6 |
| 11 | LLM provider | Native Ollama API, narrow `Provider` interface, env-var configured | §9 |
| 12 | Observability | Three tiers: slog stdout + per-reconcile JSONL traces + sparing K8s Events | §10 |
| 13 | Skill prose | Numbered procedure with Step 1 = stability check, lint-enforced + optional few-shot | §5.3 |
| 14 | Project layout | client-go directly (no controller-runtime); `cmd/controllerless` + `internal/{skill,llm,tools,dispatch,kube,trace}` | §12 |

---

## 3. Demo Scenario

**The demo replicates the kubebuilder CronJob tutorial scenario, but with a fresh agentic-native CRD** (see §6) and a fresh skill-based implementation. The vanilla kubebuilder CronJob controller is **not** the target — its CRD and behavior are a reference point only.

### What the demo proves

Three claims, in order of importance:

1. **Skills can express controller-level reconciliation logic in readable prose.** A reviewer can read `SKILL.md` files and understand the system without reading Go.
2. **The harness is general-purpose** — adding a new skill (the judgment overlay) requires zero code changes; new GVKs are watched without recompilation.
3. **The cost is real and acknowledged**: 12B-model reconcile takes 10s where a Go controller takes 10ms; non-determinism is a tradeoff, not a bug.

### What the demo deliberately exposes (counterargument material)

- The CronJob reconcile is mostly mechanical plumbing with little LLM judgment content. The judgment overlay skill (`scheduledtask-auto-suspend-on-failures`) is included specifically to show what becomes *newly possible*, not just *newly expressed*.
- Per-reconcile latency is bounded but unpredictable. This harness is for **slow control loops**, not data-path operations.
- Repeatability is best-effort, not exact. Same prompt + same seed can diverge across Ollama restarts.

---

## 4. Architecture Overview

```
                        ┌────────────────────────────────────────┐
                        │ Skill loader (fsnotify hot-reload)     │
                        │ reads ~/.k8s-agent/skills/             │
                        │ produces: GVK → []*Skill (trigger idx) │
                        └────────────────┬───────────────────────┘
                                         │
                                         ▼
                        ┌────────────────────────────────────────┐
                        │ Dynamic informer factory               │
                        │ one shared informer per GVK            │
                        │ populated via REST mapper + discovery  │
                        └────────────────┬───────────────────────┘
                                         │  Add/Update/Delete
                                         ▼
                        ┌────────────────────────────────────────┐
                        │ Per-GVK rate-limited workqueue         │
                        │ dedup by NamespacedName                │
                        │ exponential backoff on requeue         │
                        └────────────────┬───────────────────────┘
                                         │  pop(key)
                                         ▼
                        ┌────────────────────────────────────────┐
                        │ Dispatcher                             │
                        │ fetch obj from informer cache          │
                        │ evaluate per-skill conditions          │
                        │ if N matches: run each independently   │
                        └────────────────┬───────────────────────┘
                                         │
                                         ▼
                        ┌────────────────────────────────────────┐
                        │ Reconcile session per (key, skill)     │
                        │                                        │
                        │  build prompt:                         │
                        │   - harness system prompt              │
                        │   - skill body                         │
                        │   - event payload                      │
                        │  ┌───────────────────────┐             │
                        │  │ agentic tool loop:    │             │
                        │  │ LLM ↔ tool registry   │             │
                        │  │ until done() /        │             │
                        │  │ requeueAfter() /      │             │
                        │  │ max turns hit         │             │
                        │  └───────────────────────┘             │
                        │                                        │
                        │  emit: JSONL trace + slog + K8s Events │
                        └────────────────────────────────────────┘
```

### Component responsibilities

- **`skill/`** — load, parse frontmatter, lint, index by GVK + conditions; hot-reload on filesystem change.
- **`kube/`** — kubeconfig, dynamic client, REST mapper, dynamic informer factory, per-GVK workqueues, K8s Events recorder.
- **`llm/`** — provider interface, Ollama implementation, prompt assembly, defensive parsing of tool calls.
- **`tools/`** — primitive tool registry (K8s CRUD, cron/time helpers, meta); each tool is a Go function with a JSON Schema for params.
- **`dispatch/`** — event router; per-(key, skill) reconcile sessions; the agentic loop; the four safeguards.
- **`trace/`** — JSONL writer (one file per reconcile) + slog setup.
- **`config/`** — env vars + flags → typed `Config` struct.
- **`cmd/controllerless/`** — entry: parse config, wire deps, start signal handler, run dispatcher.

---

## 5. Skill Model

### 5.1 What a skill IS

A skill is a single directory under the configured skills directory, containing at minimum a `SKILL.md` file. The file has YAML frontmatter and a Markdown body. The body is a numbered procedure that the LLM follows.

This mirrors the Claude Code skill format intentionally — same mental model, separate directory.

### 5.2 Frontmatter spec

```yaml
---
name: scheduledtask-fire-due-pod          # unique, kebab-case, matches dir name
description: One-line summary used by the dispatcher's LLM when picking among matched skills.
triggers:
  - gvk: agentic.io/v1alpha1/ScheduledTask
    eventTypes: [Added, Modified]         # subset of {Added, Modified, Deleted}
    conditions:                           # optional CEL expressions evaluated against
      - "spec.suspend != true"            # the cached object after dequeue (not in the watch)
allowedTools:                             # subset of registered tools; enforced at dispatch
  - get
  - list
  - patch
  - create
  - cron.nextFireTime
  - time.secondsUntil
  - requeueAfter
  - done
---
```

**Trigger semantics:**
- A skill matches an event if (a) its `gvk` matches and (b) the event type is in `eventTypes` and (c) all `conditions` evaluate true against the cached object after dequeue.
- Filter evaluation happens at dispatch time, not at watch time. Multiple skills with different conditions can share one informer for a GVK.
- When N skills match a single event, **all N run independently** (see §8.4). No prioritization, no DAG.

### 5.3 Body conventions and lint rules

The body uses a fixed skeleton:

```markdown
# What this skill does
<1–2 sentences — also used by the dispatcher to inform the LLM picker, when present>

# Procedure

1. **Stability check.** <Read current state; if already converged, call done() and return.>
2. <Next step.>
...

# Errors
<1–2 sentences on skill-specific error handling beyond the harness defaults.>

# Examples (optional)
<Annotated trace(s) of input → tool sequence → outcome, used as few-shot demonstrations.>
```

**Lint rules enforced at skill load time** (load fails → skill is dropped with a warning logged; agent continues):

| # | Rule | Why |
|---|---|---|
| L1 | Step 1 is a stability check (idempotency) | Q8 safeguard #1. Without it, skills loop on their own writes. |
| L2 | Each numbered step starts with an imperative verb | Small models follow imperatives more reliably than declaratives. |
| L3 | Tool references are in backticks: `` `toolName(...)` `` | Reduces "narration instead of invocation" failure mode on small models. |
| L4 | No more than 8 numbered steps | If you need more, decompose into multiple skills. |
| L5 | Conditional nesting ≤ 1 level deep | Same reason. Decompose. |
| L6 | All tools mentioned in the body must be in `allowedTools` | Catches drift between prose and permission. |
| L7 | Body has `# What this skill does` and `# Procedure` sections | Required for the dispatcher's LLM picker to summarize the skill. |
| L8 | `name:` in frontmatter matches the parent directory name | Filesystem-as-truth; prevents collisions. |

### 5.4 Example: `scheduledtask-fire-due-pod`

Full body (this is the load-bearing skill; see §11 for the full skill set):

````markdown
---
name: scheduledtask-fire-due-pod
description: Creates a Pod when a ScheduledTask's next scheduled fire time has arrived.
triggers:
  - gvk: agentic.io/v1alpha1/ScheduledTask
    eventTypes: [Added, Modified]
allowedTools:
  - get
  - list
  - patch
  - create
  - cron.nextFireTime
  - time.secondsUntil
  - time.toUnix
  - requeueAfter
  - done
---

# What this skill does

Ensures a ScheduledTask fires on its cron schedule by creating a Pod from
`spec.podSpec` at each scheduled time, with label-based deduplication so
re-reconciles don't create duplicates.

# Procedure

1. **Stability check.** If `spec.suspend == true`, call `done()` and return.

2. **Compute the next fire time.**
   - Call `cron.nextFireTime(expr=spec.schedule, after=now)` → `nextFire`.
   - If `status.nextFireTime != nextFire`, `patch` status with the new value
     using SSA (the harness sets the field manager automatically).

3. **Decide whether to fire now.**
   - If `now < nextFire`, call `requeueAfter(time.secondsUntil(nextFire), "not yet due")`.
   - Otherwise, continue.

4. **Idempotency check.**
   - Call `time.toUnix(nextFire)` → `scheduledAtUnix`.
   - `list` Pods in our namespace with labels:
     `agentic.io/scheduledtask=<our metadata.name>` AND
     `agentic.io/scheduled-at=<scheduledAtUnix>`.
   - If any Pods are returned, skip to step 6.

5. **Create the Pod.** Build a Pod with:
   - `metadata.generateName`: `<our name>-<scheduledAtUnix>-`
   - `metadata.labels`: include the two dedup labels above, plus any labels
     from `spec.podSpec` metadata if present.
   - `metadata.ownerReferences`: one entry pointing to us, with
     `controller: true` and `blockOwnerDeletion: true`.
   - `spec`: deep copy of our `spec.podSpec`.
   Then call `create(pod)`.

6. **Append to status.fires.**
   - Build entry: `{scheduledAt: <nextFire RFC3339>, scheduledAtUnix: <nextFire Unix int>, podName: <created pod's name>, phase: "Pending", note: ""}`.
   - `patch` status by appending this entry to `status.fires` using SSA.

7. **Schedule the next reconcile.**
   - Compute `nextNextFire = cron.nextFireTime(spec.schedule, after=nextFire)`.
   - Call `requeueAfter(time.secondsUntil(nextNextFire), "next fire time")`.

# Errors

If `create` returns AlreadyExists, treat it as success and continue to step 6 —
this means another reconcile beat us, but the desired state holds.
````

### 5.5 Storage and discovery

- Default directory: `~/.controllerless/skills/`. Overridable via `--skills-dir`.
- Layout: one directory per skill, mirroring Claude Code (`<skills-dir>/<skill-name>/SKILL.md`).
- A repo-local default `./skills/` directory ships with the demo skills.
- Companion files (e.g., longer examples, schemas) can live alongside `SKILL.md` for future use.

### 5.6 Hot-reload

- `fsnotify` watches the configured skills directory recursively.
- On any change (create / modify / delete) to a `SKILL.md`, the loader re-parses + lints the affected skill and atomically swaps it into the live registry.
- Lint failure on reload: the previous version remains active; a WARN is logged.
- In-flight reconciles complete with the old skill body; new reconciles use the new body.

---

## 6. CRD: `ScheduledTask`

The demo CRD. Designed to be agentic-native: minimal typed fields, append-only status, label-based dedup.

### 6.1 Spec

```yaml
apiVersion: agentic.io/v1alpha1
kind: ScheduledTask
metadata:
  name: nightly-backup
  namespace: default
spec:
  schedule: "0 2 * * *"                    # cron expression (standard 5-field)
  podSpec:                                 # direct PodSpec — no Job wrapper
    containers:
      - name: backup
        image: my-org/pg-backup:latest
  historyLimit: 5                          # how many entries to keep in status.fires
  suspend: false                           # pause firing
```

### 6.2 Status (event-log shape)

```yaml
status:
  nextFireTime: "2026-06-12T02:00:00Z"
  fires:                                   # append-only log
    - scheduledAt: "2026-06-11T02:00:00Z" # RFC3339 — human-readable, list-map key
      scheduledAtUnix: 1717984800          # Unix int — matches Pod label agentic.io/scheduled-at
      podName: nightly-backup-1717984800-x7yz
      phase: Succeeded                     # mirrors Pod.status.phase
      note: "exit 0"                       # free-form, LLM-friendly
  conditions:
    - type: Ready
      status: "True"
      reason: Scheduled
      lastTransitionTime: "2026-06-11T02:00:05Z"
```

**Key shape decisions:**
- `status.fires` is an **append-only event log**, not a condition state machine. Skills only ever *append*, never *transition*. SSA with `listType=map, listMapKeys=[scheduledAt]` lets two skills append concurrently without racing.
- `scheduledAt` (RFC3339) is the list-map key and the human-readable field. `scheduledAtUnix` (int) matches the Pod label `agentic.io/scheduled-at` directly — no format conversion needed in skill prose.
- Only one or two `conditions` total. Heavy condition state machines are exactly what 12B models get wrong.
- `status.nextFireTime` is denormalized for ease-of-prose ("read status.nextFireTime, compare to now").

### 6.3 Labels and naming

Pods created by the agent carry:

```yaml
metadata:
  generateName: <task-name>-<scheduledAtUnix>-     # k8s appends a random suffix
  labels:
    agentic.io/scheduledtask: <task-name>          # dedup label 1
    agentic.io/scheduled-at: "<scheduledAtUnix>"   # dedup label 2 (string)
  ownerReferences:
    - apiVersion: agentic.io/v1alpha1
      kind: ScheduledTask
      name: <task-name>
      uid: <task-uid>
      controller: true
      blockOwnerDeletion: true
```

**Idempotency contract**: before any `create` of a Pod for a given `(scheduledtask, scheduled-at)`, the skill MUST `list` by the label combo and skip on non-empty.

This is weaker than name-based dedup (the list-then-create is racy under concurrent reconciles), but the workqueue's per-key serialization closes the race for a single ScheduledTask. Sufficient for the POC; documented as a limitation.

### 6.4 Comparison to kubebuilder CronJob — what we dropped and why

| kubebuilder CronJob field | controllerless ScheduledTask | Rationale |
|---|---|---|
| `spec.jobTemplate` (→ Job → Pod) | `spec.podSpec` (direct Pod) | Job exists for retries/parallelism. For cron, "retry" means "the next fire"; one less GVK to walk. |
| `spec.startingDeadlineSeconds` | dropped | Expressible as 2 lines of skill prose. |
| `spec.concurrencyPolicy` | dropped (default = Allow) | Same; "skip fire if any owned Pod is Running" is prose. |
| `spec.successfulJobsHistoryLimit` + `failedJobsHistoryLimit` | collapsed → `spec.historyLimit` | Two limits, ~no behavioral gain. |
| `status.active` (object refs) | embedded in `status.fires[].podName` | Single denormalized place. |
| `status.lastScheduleTime`, `lastSuccessfulTime` | embedded in `status.fires` | Same. |
| Full condition state machine | one or two simple conditions + event log | Conditions are a state machine; LLMs are bad at state machines. |

### 6.5 CRD manifest

The CRD YAML lives at `internal/crd/scheduledtask/crd.yaml` and is applied via `make install-crd`.

(Manifest itself to be authored at scaffolding time. Use `apiextensions.k8s.io/v1` `CustomResourceDefinition` with OpenAPI v3 schema, `subresources.status: {}`, scope `Namespaced`. Schema for `status.fires` uses `x-kubernetes-list-type: map` with `x-kubernetes-list-map-keys: [scheduledAt]` to enable SSA-friendly merging.)

---

## 7. Tool Surface

### 7.1 K8s primitives (always available, subject to skill `allowedTools`)

```
Read:
  get(gvk, namespace, name) -> object
  list(gvk, namespace, labelSelector?, fieldSelector?) -> []object

Write:
  patch(gvk, namespace, name, patch, type=SSA, fieldManager?, subresource?) -> object
  create(object) -> object
  delete(gvk, namespace, name, propagationPolicy=Background) -> ok
```

Deferred to v2: `getEvents`, `getLogs`. Both are useful (log-reading makes `status.fires[].note` richer; events help diagnostic skills) but no demo skill exercises them. Add when a skill that actually uses them is ready.

**Write semantics:**
- `patch` defaults to **server-side apply** (`Content-Type: application/apply-patch+yaml`).
- `fieldManager` defaults to `skill/<name>` if not provided. The harness fills it in.
- `subresource` of `status` targets the `/status` subresource. Skills writing to status MUST specify this; the dispatcher rejects status field writes on the main resource.
- `delete` propagationPolicy defaults to `Background`. `Foreground` is opt-in.

### 7.2 Pure-function helpers (deterministic, no LLM call)

```
cron.nextFireTime(expr string, after time.Time) -> time.Time

time.parseDuration(s string) -> time.Duration
time.secondsUntil(t time.Time) -> int
time.since(t time.Time) -> int
time.now() -> time.Time
time.toUnix(t time.Time) -> int64
time.fromUnix(unix int64) -> time.Time

(extensible — add as needed)
```

Deferred to v2: `cron.missedRuns`. Required for startup catch-up (processing schedules missed during agent downtime), which is itself a v2 item.

These are tools (the LLM invokes them via tool calls) but they have **no I/O and no side effects**. Implemented via `github.com/robfig/cron/v3` and the stdlib `time`.

**The principle behind these tools:** *anything the LLM would get wrong deterministically should be a tool, not a prompt instruction.* Cron parsing, time math, regex matching, JSON Path — all candidates.

### 7.3 Meta tools (control flow)

```
done(summary string) -> terminates the reconcile session successfully
requeueAfter(seconds int, reason string) -> schedules a re-enqueue at now+seconds; terminates the session
```

Both terminate the LLM tool loop. The first of `done()` or `requeueAfter()` to arrive ends the session; any subsequent call to the other is ignored. A skill should never call both — `requeueAfter` is already terminal. Errors during the reconcile session are signalled by the harness via tool-error messages; the LLM doesn't have a `fail` tool.

### 7.4 Why no composite tools (e.g., `createOwnedScheduledJob`)

An earlier branch of the design considered shipping K8s-aware composite tools (e.g., `createOwnedScheduledJob` that encapsulates name + ownerRef + annotations + create in one atomic operation). **Rejected.** Rationale:

- Composite tools push *opinions about K8s patterns* into the harness, making the harness less general.
- The thesis is "business logic lives in prose." If the prose says "call `createOwnedScheduledJob(...)`", the interesting logic has moved out of prose and into Go.
- **The invariant problem was instead solved by reshaping the CRD** (see §6) so that the surviving invariants are small enough for prompt discipline to hold (set two labels, set one owner ref, use SSA for status).

If a future expansion needs to handle resources where the invariant cost is too high to express in prose, **user-defined custom tools** become the v2 escape hatch — not built-in composites.

### 7.5 Tool descriptions to the LLM

Each tool is described to the LLM via the OpenAI/Ollama tool schema (function name, description, JSON Schema for parameters). The harness owns the tool registry; the LLM provider serializes the relevant subset (filtered by skill `allowedTools`) into the `tools` field of each `Chat` request.

```go
type ToolDef struct {
    Name        string
    Description string
    Schema      json.RawMessage  // JSON Schema for parameters
}
```

Schemas are written by hand alongside each tool's Go implementation (no codegen for POC).

---

## 8. Watch and Dispatch Machinery

### 8.1 Dynamic informer factory

- Built on `k8s.io/client-go/dynamic/dynamicinformer.NewDynamicSharedInformerFactory`.
- One shared informer per GVR across all skills that watch that GVK.
- **GVK → GVR translation** happens once in `kube/client.go` at startup (and again on hot-reload when new GVKs appear from skill changes). The REST mapper (`meta.RESTMapper`) resolves each skill's `gvk` string to a `schema.GroupVersionResource`. The result is cached as a `map[schema.GroupVersionKind]schema.GroupVersionResource`. Skills, the dispatcher, and all tool calls use the cached GVR — the REST mapper is never called at dispatch time.
- **Internal GVK type**: `schema.GroupVersionKind` from `k8s.io/apimachinery/pkg/runtime/schema`. Comparable, usable as a map key. The YAML string `"agentic.io/v1alpha1/ScheduledTask"` is parsed into this struct at skill load time.
- **Unknown GVK at startup**: if the REST mapper lookup fails (GVK not present on the cluster), the affected skill is dropped with a WARN log. The agent continues with the remaining skills. On hot-reload, the same rule applies.
- Resync period: 10 minutes. Catches missed events on watch disconnect.
- **Tombstone handling (mandatory):** the `OnDelete` handler in `kube/informers.go` MUST type-switch on `cache.DeletedFinalStateUnknown` and extract the inner `Obj` before enqueuing. If the inner object cannot be cast to `*unstructured.Unstructured`, log WARN and drop — never pass a raw tombstone to the dispatcher. Skills never see a `DeletedFinalStateUnknown`.

### 8.2 Workqueue per GVK

- `workqueue.NewNamedRateLimitingQueue` per GVK.
- Key format: `<namespace>/<name>`. Per-key serialization is built in.
- Two distinct enqueue paths — never mix them:
  - **`AddAfter(key, delay)`** — used by `requeueAfter`. Exact delay, bypasses backoff state. Signals successful reconcile that needs a future retry.
  - **`AddRateLimited(key)`** — used on error paths (max turns exceeded, hard panic, LLM timeout). Exponential backoff accumulates per key.
- A single key can be enqueued by (a) informer events (uses `Add`), (b) `requeueAfter` from skills (uses `AddAfter`), (c) error recovery (uses `AddRateLimited`), (d) cross-GVK owner propagation (uses `Add`).

### 8.3 Dispatch flow (the agentic tool loop)

```
For each popped (gvk, key) from a workqueue:
  1. Fetch the object from the informer cache (not the API server).
  2. Evaluate each skill's `conditions` against the object.
  3. Collect matching skills.
  4. For each matching skill, in sequence (mirroring the workqueue's per-key
     serialization guarantee — this is how kube-controller-manager works too).
     Each skill is wrapped in its own error boundary (deferred recover). A panic
     or hard failure in one skill is caught, logged, and traced; remaining skills
     still run. Skills never share fate.
     a. Start a new reconcile session (ULID = reconcileId).
     b. Open the JSONL trace file.
     c. Build the prompt:
          - harness system prompt (fixed)
          - skill body (from SKILL.md)
          - event payload + cached object (YAML)
     d. Run the agentic loop:
          - Call provider.Chat(messages, tools=allowedToolDefs).
          - For each tool_call in response:
            * Validate name against allowedTools.
            * Validate args against the tool's JSON Schema.
            * Execute the tool (K8s call, helper, meta).
            * Append a tool message with the result (or error).
          - Loop until: done() / requeueAfter() / max turns / hard error.
     e. Close the trace; emit K8s Events.
     f. Apply any requeueAfter scheduling to the workqueue.
  5. After all skills for this (gvk, key) have run, the harness walks the
     triggering object's `ownerReferences` and enqueues each owner key into
     its GVK's workqueue (if that GVK is being watched). This is unconditional
     — no skill opt-in needed — mirroring controller-runtime's `Owns()`.
     Rate limit S4 bounds re-trigger noise.
```

### 8.4 Multi-skill orchestration: independent skills, controller-style convergence

**Principle:** Skills do not coordinate with each other. They are independent reconcile units, like Kubernetes controllers themselves. Composition emerges from **shared cluster state**, not from a planner or DAG.

For the CronJob demo:

```
On Modified(ScheduledTask foo):
   workqueue/ScheduledTask picks up key foo.
   dispatcher finds 4 matching skills, runs each independently:
      → scheduledtask-fire-due-pod         (creates Pod if due)
      → scheduledtask-update-fire-status   (no-op if no Pod state change)
      → scheduledtask-prune-history        (no-op if under limit)
      → scheduledtask-auto-suspend-on-failures  (judgment overlay)

On Modified(Pod bar) where labels include agentic.io/scheduledtask=foo:
   workqueue/Pod picks up key bar.
   dispatcher routes to skills that match Pod events:
      → scheduledtask-update-fire-status   (patches matching entry in status.fires)
   AND enqueues owner key (ScheduledTask foo) for downstream re-reconcile.
```

No planner LLM. No DAG. No meta-skill. The four-skill cascade is the entire orchestration story.

### 8.5 The four safeguards (mandatory for §8.4 to work)

| # | Safeguard | Where | Why |
|---|---|---|---|
| S1 | **Idempotency-as-prose-convention** | Lint rule L1 (Step 1 = stability check) | Skills must check "already converged?" first. Otherwise they loop on their own writes. |
| S2 | **SSA with per-skill `fieldManager`** | `patch` tool default | Two skills writing to status race on optimistic-concurrency `Update`. SSA with `fieldManager=skill/<name>` lets each skill own a subset and merge cleanly. |
| S3 | **Per-(resource, skill) rate limit** | Dispatcher token bucket | Workqueue gives backoff per *key*; not enough if skills bounce off each other's writes (keys keep changing). Token bucket per `(namespacedName, skill name)` — default 1 fire per 5s — is the circuit breaker. |
| S4 | **Trigger conditions** | Frontmatter `triggers[].conditions` (CEL) | Skills declare the conditions under which they are relevant. Once their convergence action makes the condition false, they stop re-firing. |

---

## 9. LLM Provider

### 9.1 Interface

Provider-agnostic, mirroring the OpenAI/Ollama/Anthropic chat-with-tools shape:

```go
package llm

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

type Role string
const (
    RoleSystem    Role = "system"
    RoleUser      Role = "user"
    RoleAssistant Role = "assistant"
    RoleTool      Role = "tool"
)

type Message struct {
    Role       Role
    Content    string
    ToolCalls  []ToolCall    // populated on assistant turns
    ToolCallID string        // populated on tool turns
    Name       string        // tool name, populated on tool turns
}

type ToolDef struct {
    Name        string
    Description string
    Schema      json.RawMessage // JSON Schema for parameters
}

type ToolCall struct {
    ID        string
    Name      string
    Arguments json.RawMessage // raw JSON args (unmarshaled by dispatcher, not provider)
}

type StopReason string
const (
    StopReasonStop     StopReason = "stop"
    StopReasonToolUse  StopReason = "tool_use"
    StopReasonMaxTokens StopReason = "max_tokens"
    StopReasonError    StopReason = "error"
)

type ChatResponse struct {
    Message    Message
    StopReason StopReason
    Usage      Usage
}

type Usage struct {
    PromptTokens     int
    CompletionTokens int
    TotalMs          int64
}
```

Notable shape decisions:
- `Arguments` stays opaque at the interface boundary. The **dispatcher** unmarshals against the tool's declared schema and emits defensive errors on failure. This keeps providers thin.
- `RoleTool` is distinct from `RoleUser`. Both Ollama and OpenAI distinguish them.
- No streaming in v1. Tool-call payloads are atomic; partial tool calls aren't useful. Add `ChatStream` later if needed.

### 9.2 Native Ollama implementation

- Uses `github.com/ollama/ollama/api` (native client), not OpenAI-compat endpoint.
- Rationale: Ollama's OpenAI-compat shim has known gaps in tool-call forwarding for some models (ref: ollama/ollama#9941). Native path is most likely to expose `gemma4`'s tool calling correctly.
- Endpoint: `POST /api/chat` with `tools: [...]`.
- Default model for the POC: `gemma4:12b-mxfp8` (Apple Silicon MLX build, text-only, 256K context, native function-calling). Released June 2026.

### 9.3 Environment variable contract

```
# Provider selection
LLM_PROVIDER=ollama                       # only "ollama" supported in v1
LLM_BASE_URL=http://localhost:11434       # Ollama daemon
LLM_MODEL=gemma4:12b-mxfp8
LLM_API_KEY=                              # unused for Ollama; reserved

# Tuning
LLM_TEMPERATURE=0.2                       # low but not zero (zero can produce loops)
LLM_MAX_TOKENS=4096
LLM_TIMEOUT=120s                          # per chat call
LLM_NUM_CTX=16384                         # Ollama-specific; passed through OptionsMap

# Agentic loop bounds
AGENT_MAX_TOOL_TURNS=10                   # per reconcile session
AGENT_RECONCILE_TIMEOUT=5m                # wall-clock cap per reconcile
AGENT_PER_SKILL_RATE_LIMIT=5s             # min interval between fires of same (resource, skill)

# Other
KUBECONFIG=$HOME/.kube/config
SKILLS_DIR=./skills
TRACES_DIR=~/.controllerless/traces
LOG_LEVEL=info                            # debug | info | warn | error
```

### 9.4 Prompt structure (system + skill + event)

**Harness-owned system prompt (fixed, applies to every reconcile):**

```
You are an autonomous Kubernetes reconciler.
You will be given (a) a single triggering event and (b) one skill describing what to do.
Use only the provided tools to gather state and make changes.
Before acting, check current state — if the desired outcome is already true, call done().
When all work is finished, call done(). If you need to be retried later, call requeueAfter().
Tool errors are real — if a tool returns an error, do not retry the same tool with the same arguments.
```

**Skill body** is appended as a second `system` message (or concatenated into the first; provider-dependent — Ollama accepts multiple system messages natively).

**User message** contains:

```
Event:
  type: Modified
  object:
    apiVersion: agentic.io/v1alpha1
    kind: ScheduledTask
    metadata:
      name: nightly-backup
      namespace: default
      generation: 3
    spec:
      schedule: "0 2 * * *"
      ...
    status:
      ...
Cluster context:
  cluster: <kubeconfig context name>
  timestamp: 2026-06-12T01:23:45Z
```

**Why split**: invariants like "call done when finished" apply to every skill. Duplicating them in every skill body is waste and drift. Skill prose stays focused on *what makes this skill different*.

### 9.5 Defensive parsing

Four failure modes, all handled in the dispatcher (not in the provider):

| Failure | Detection | Recovery |
|---|---|---|
| Malformed JSON in `Arguments` | Unmarshal into declared schema fails | Append a tool message: `"error: arguments were not valid JSON matching schema: <details>"`. Continue loop. |
| Unknown / disallowed tool name | Name not in allowed set | Same: append tool error message. Continue. |
| Same `(tool, argsHash)` called >3 times | Per-reconcile counter | Force-stop with explanatory tool message; requeue with backoff. |
| Exceeded `AGENT_MAX_TOOL_TURNS` | Loop counter | Requeue with backoff; full transcript logged at WARN. |

**Pattern**: never silently retry, always tell the model what went wrong as a tool result, let it self-correct, but cap the loop hard.

---

## 10. Observability

Three tiers, each serving a different consumer.

### 10.1 Tier 1 — structured logs (stdout)

- `slog` with JSON handler → stdout.
- Levels: `debug`, `info` (default), `warn`, `error`.
- INFO: high-signal events only (`dispatch`, `skill_start`, `skill_complete`, `requeue`, errors).
- DEBUG: adds tool calls.
- TRACE (custom): adds full prompts/completions.
- Tier 2 JSONL writer subscribes to ALL events regardless of stdout log level.

### 10.2 Tier 2 — per-reconcile JSONL traces (forensics & demo material)

**This is the load-bearing observability tier.** Each reconcile gets one file:

```
~/.controllerless/traces/<YYYY-MM-DD>/<namespace>__<gvk>__<name>__<unix>.jsonl
```

Each line is a self-contained JSON event. Schema:

```jsonl
{"t":"...","reconcileId":"01HXY...","phase":"dispatch","event":{"type":"Modified","gvk":"agentic.io/v1alpha1/ScheduledTask","ns":"default","name":"nightly-backup","resourceVersion":"4218"},"matchedSkills":["scheduledtask-fire-due-pod","scheduledtask-prune-history"]}
{"t":"...","reconcileId":"01HXY...","phase":"skill_start","skill":"scheduledtask-fire-due-pod"}
{"t":"...","reconcileId":"01HXY...","phase":"llm_request","model":"gemma4:12b-mxfp8","temperature":0.2,"seed":42,"messages":[...full prompt...],"tools":[...allowed tools with schemas...]}
{"t":"...","reconcileId":"01HXY...","phase":"llm_response","content":"...","toolCalls":[{"id":"tc_1","name":"cron.nextFireTime","args":{...}}],"usage":{"promptTokens":1842,"completionTokens":67,"totalMs":1340}}
{"t":"...","reconcileId":"01HXY...","phase":"tool_call","id":"tc_1","name":"cron.nextFireTime","args":{...},"result":"...","durationMs":1}
{"t":"...","reconcileId":"01HXY...","phase":"k8s_mutation","verb":"create","gvk":"v1/Pod","namespace":"default","name":"nightly-backup-1717984800-x7yz","requestBody":{...},"responseStatus":201,"durationMs":89}
{"t":"...","reconcileId":"01HXY...","phase":"skill_complete","outcome":"done","summary":"Created Pod ...; appended fire to status","totalDurationMs":6421,"llmTurns":4,"toolCalls":5,"k8sMutations":2}
{"t":"...","reconcileId":"01HXY...","phase":"requeue","afterSeconds":86400,"reason":"nextFireTime"}
```

Properties that matter:
- One file per reconcile = `cat <file>` is the entire debug session.
- `reconcileId` (ULID) ties together everything across skills for a single dispatch.
- Full prompt + completion captured verbatim → reconciles can be replayed (approximately) by re-feeding to Ollama.
- `k8s_mutation` captures request body + response status → forensic record of what changed in the cluster.

### 10.3 Tier 3 — Kubernetes Events (sparing)

```
Type     Reason            Source                                      Message
─────    ──────            ──────                                      ───────
Normal   SkillStarted      skill/scheduledtask-fire-due-pod            Reconciling ScheduledTask nightly-backup
Normal   PodCreated        skill/scheduledtask-fire-due-pod            Created Pod nightly-backup-1717984800-x7yz
Normal   SkillCompleted    skill/scheduledtask-fire-due-pod            Reconcile complete in 6.4s, next fire in 24h
Warning  SkillFailed       skill/scheduledtask-explain-failure         LLM exceeded max tool turns; trace at /traces/.../01HXY....jsonl
```

- Source field: `skill/<name>` → `kubectl get events --field-selector source=skill/...` filters by skill.
- Failure events include the trace file path so operators can jump from "kubectl told me this broke" → "here's the JSONL with the prompt and tool calls."
- Use sparingly: K8s Events have rate limits and 1-hour TTL by default. Per-tool-call granularity goes to JSONL, not Events.

### 10.4 Repeatability — honest limits

Even with full prompt capture + pinned seed + temperature 0.2:
- Ollama's seed support is best-effort; server restart / KV cache state / quant rounding can diverge outputs.
- Different Ollama versions produce different outputs from identical inputs.
- MLX `mxfp8` quantization specifically has subtle reproducibility issues vs GGUF quants.

**The trace is a witness, not a re-runner.** You can always see *what happened*; you can usually approximately *replay*; you can rarely bit-exact reproduce. Bake this expectation into the demo narrative.

### 10.5 The trace as the primary demo artifact

When presenting this to the audience, **walk through one JSONL trace file** — don't walk through code. The narrative: "here's the event, here's the skill the LLM saw, here's the prose, here's the tool calls it chose, here's the cluster change it made, here's the outcome." That narrative is the paradigm shift. Build the trace format with that audience in mind.

---

## 11. Skill Set for the Demo

Four skills total. The first three reimplement what the kubebuilder CronJob controller does (proving "the harness can replace a controller"). The fourth is the judgment overlay (proving "skills can do what controllers can't easily do").

| Skill | Triggers on | Purpose | Replaces / adds |
|---|---|---|---|
| `scheduledtask-fire-due-pod` | `ScheduledTask` Add/Modify; timer (via requeueAfter) | Compute next fire time; create Pod when due; idempotent via label combo | Replaces controller fire logic |
| `scheduledtask-update-fire-status` | `Pod` Modify where `labels[agentic.io/scheduledtask]` is set | Patch matching `status.fires[]` entry on owning ScheduledTask with current Pod phase + note | Replaces controller status-update logic |
| `scheduledtask-prune-history` | `ScheduledTask` Modify where `len(status.fires) > spec.historyLimit` | Sort by `scheduledAt`, drop oldest until ≤ limit; delete corresponding Pods; patch status | Replaces controller history-cleanup logic |
| `scheduledtask-auto-suspend-on-failures` | `ScheduledTask` Modify where last 3 `fires` are all `Failed` | Patch `spec.suspend=true`; append condition `Suspended` with reason `RepeatedFailures` | **New capability** — judgment overlay |

Each skill body is ~20–60 lines of prose. Combined, the four skills replace what would be a ~200-line Go controller, plus add a behavior that doesn't exist in the kubebuilder version.

### Cascade example

```
t+0      User applies ScheduledTask nightly-backup.
t+0.1s   Informer Add event → workqueue.
t+0.2s   Dispatcher → 4 matching skills.
t+1s     scheduledtask-fire-due-pod runs:
         - computes next fire (now+1h),
         - calls requeueAfter(3600s), done().
t+1s     scheduledtask-update-fire-status runs:
         - sees no Pods yet, done().
t+1s     scheduledtask-prune-history runs:
         - status.fires is empty, done().
t+1s     scheduledtask-auto-suspend-on-failures runs:
         - no fires yet, done().
... (1 hour later) ...
t+3600s  requeueAfter fires → workqueue → dispatcher again.
t+3601s  scheduledtask-fire-due-pod runs:
         - now >= nextFire,
         - lists Pods by label combo (empty),
         - creates Pod,
         - patches status.fires (append),
         - requeueAfter(next next fire), done().
t+3602s  Informer Add(Pod) → workqueue/Pod.
t+3602s  scheduledtask-update-fire-status runs:
         - finds matching status.fires[] entry, patches phase=Pending, done().
... (Pod runs, eventually succeeds) ...
t+3700s  Informer Modify(Pod, phase=Succeeded) → workqueue/Pod.
t+3700s  scheduledtask-update-fire-status runs:
         - patches phase=Succeeded, note="exit 0", done().
         - enqueues owner key.
t+3701s  Dispatcher on ScheduledTask:
         - scheduledtask-prune-history: 1 fire, under limit, done().
         - scheduledtask-auto-suspend-on-failures: 1 success, done().
         - (others: stability checks pass, done quickly).
```

---

## 12. Project Layout

### Directory tree

```
controllerless/
├── cmd/controllerless/main.go     # entry: config + wiring + signals
├── internal/
│   ├── config/                    # env vars + flags → Config struct
│   ├── skill/                     # loader, frontmatter parse, lint, trigger matching
│   │   ├── loader.go
│   │   ├── parse.go
│   │   ├── lint.go
│   │   ├── trigger.go
│   │   └── types.go
│   ├── llm/
│   │   ├── provider.go            # interface from §9.1
│   │   ├── types.go
│   │   └── providers/
│   │       ├── ollama/
│   │       │   └── ollama.go
│   │       └── mock/
│   │           └── mock.go
│   ├── tools/
│   │   ├── registry.go            # name → handler lookup
│   │   ├── k8s.go                 # get/list/patch/create/delete + getLogs/getEvents
│   │   ├── cron.go                # cron.nextFireTime etc.
│   │   ├── timefn.go              # time.secondsUntil etc.
│   │   └── meta.go                # done, requeueAfter
│   ├── dispatch/
│   │   ├── dispatcher.go          # event → matched skills → run each
│   │   ├── reconcile.go           # the tool-call loop
│   │   ├── safeguards.go          # rate limit, idempotency hash, max turns
│   │   └── prompt.go              # prompt assembly
│   ├── kube/
│   │   ├── client.go              # kubeconfig, REST mapper, dynamic client
│   │   ├── informers.go           # dynamic informer factory per GVK
│   │   ├── workqueue.go           # named rate-limiting queue per GVK
│   │   └── events.go              # K8s Events recorder
│   ├── trace/
│   │   ├── jsonl.go               # JSONL writer (Tier 2)
│   │   └── log.go                 # slog setup (Tier 1)
│   └── crd/
│       └── scheduledtask/
│           └── crd.yaml           # CRD manifest, versioned with the code
├── skills/                        # default --skills-dir, hot-reloaded
│   ├── scheduledtask-fire-due-pod/SKILL.md
│   ├── scheduledtask-update-fire-status/SKILL.md
│   ├── scheduledtask-prune-history/SKILL.md
│   └── scheduledtask-auto-suspend-on-failures/SKILL.md
├── examples/
│   └── nightly-backup.yaml
├── go.mod
├── Makefile
├── README.md
├── DESIGN.md                      # this file
└── .gitignore
```

**Boundary rationale:**
- Packages follow **domains**, not layers. Don't introduce `service/` or `repository/`.
- `tools/` is a sibling of `llm/`, not under it. Tools are dispatcher-invoked Go functions that the LLM is *told about*; they're not LLM-owned.
- `skills/` lives at repo root, not under `internal/`. It's data, not code; users will fork the repo and edit this directory.

### `go.mod`

```go
module github.com/starbops/controllerless

go 1.25

require (
    k8s.io/apimachinery           v0.35.3
    k8s.io/client-go              v0.35.3
    k8s.io/api                    v0.35.3
    k8s.io/apiextensions-apiserver v0.35.3  // for CRD installation in envtest

    github.com/google/cel-go      v0.x.x       // CEL condition evaluation
    github.com/ollama/ollama      v0.x.x       // pin at scaffold time
    github.com/robfig/cron/v3     v3.0.x
    github.com/fsnotify/fsnotify  v1.7.x
    gopkg.in/yaml.v3              v3.0.x
    github.com/oklog/ulid/v2      v2.1.x
)

require (
    sigs.k8s.io/controller-runtime v0.19.x   // test-only: envtest API server
)
```

**Production code**: no `sigs.k8s.io/controller-runtime` imports — dynamic client + raw workqueue only. Typed reconcilers don't fit runtime-declared GVKs; manager/leader election are out of scope for a single-replica POC.

**Test-only**: `controller-runtime` appears in `go.mod` solely for `sigs.k8s.io/controller-runtime/pkg/envtest`. It is imported only in `_test.go` files and compiled into no production binary.

### `cmd/controllerless/main.go` wiring sketch

```go
func main() {
    cfg := config.FromEnvAndFlags()

    kube      := kube.MustNewDynamicClient(cfg.Kubeconfig)
    informers := kube.NewInformerFactory(cfg.ResyncPeriod)
    queues    := kube.NewQueueRegistry()
    events    := kube.NewEventRecorder(kube)

    provider  := llm.MustNew(cfg.LLM)                  // dispatches on cfg.LLM.Provider
    toolReg   := tools.NewRegistry(kube)               // wires k8s + cron + time + meta
    tracer    := trace.NewJSONLTracer(cfg.TracesDir)
    logger    := trace.NewLogger(cfg.LogLevel)

    skills, err := skill.LoadAndLint(cfg.SkillsDir)
    if err != nil {
        logger.Error("skill load failed", "err", err)
        os.Exit(1)
    }
    skill.WatchForChanges(cfg.SkillsDir, skills.Reload)

    d := dispatch.New(dispatch.Deps{
        Skills:    skills,
        Provider:  provider,
        Tools:     toolReg,
        Tracer:    tracer,
        Logger:    logger,
        Events:    events,
        Informers: informers,
        Queues:    queues,
    })

    for _, gvk := range skills.WatchedGVKs() {
        d.WatchGVK(gvk)
    }

    ctx, stop := signal.NotifyContext(context.Background(),
        syscall.SIGINT, syscall.SIGTERM)
    defer stop()

    if err := d.Run(ctx); err != nil {
        logger.Error("dispatcher exited with error", "err", err)
        os.Exit(1)
    }
}
```

Two design opinions baked in:
- **No global state.** Every dependency is passed via `Deps`. Makes unit-testing the dispatcher trivial with mocked providers/tools.
- **`MustNew*` constructors panic at startup, not at first use.** Misconfiguration fails loudly on `make run`, not 5 minutes later when the first event arrives.

### `Makefile`

```make
.PHONY: build test lint install-crd run example-task clean

OLLAMA_HOST ?= http://localhost:11434
LLM_MODEL   ?= gemma4:12b-mxfp8
SKILLS_DIR  ?= ./skills
KUBECONFIG  ?= $(HOME)/.kube/config

build:
	go build -o bin/controllerless ./cmd/controllerless

test:
	go test ./...

lint:
	golangci-lint run

install-crd:
	kubectl apply -f internal/crd/scheduledtask/crd.yaml

run: build
	KUBECONFIG=$(KUBECONFIG) \
	LLM_PROVIDER=ollama \
	LLM_BASE_URL=$(OLLAMA_HOST) \
	LLM_MODEL=$(LLM_MODEL) \
	./bin/controllerless --skills-dir=$(SKILLS_DIR)

example-task:
	kubectl apply -f examples/nightly-backup.yaml

clean:
	rm -rf bin/
```

`make install-crd && make run && make example-task` is the full demo loop.

---

## 12a. Testing Strategy

Three tiers, matching scope to cost:

### Tier 1 — Unit tests (`go test ./...`)

- **LLM**: `providers/mock` returns scripted `ChatResponse` values. Tests are deterministic and offline.
- **K8s client**: `k8s.io/client-go/kubernetes/fake` for CRUD operations.
- **Coverage target**: every package in `internal/` — especially `skill/` (lint rules, frontmatter parsing), `tools/` (each tool in isolation), `dispatch/` (prompt assembly, tool loop termination, safeguard enforcement, per-skill error boundary).
- **Run**: `make test`

### Tier 2 — Integration tests (`go test ./... -tags integration`)

- **LLM**: `providers/mock` (same as unit tests — no real Ollama needed).
- **K8s**: `sigs.k8s.io/controller-runtime/pkg/envtest` spins up a real API server binary. The `ScheduledTask` CRD is installed at `TestMain`. Tests exercise real watch/informer/workqueue behaviour, SSA field-manager merging, and owner-reference walking against a real etcd.
- **Coverage target**: `kube/` and `dispatch/` end-to-end — skill triggers a real informer event → dispatcher fires → tool calls hit real API server → status is patched and verified.
- **Run**: `make test-integration` (separate target; requires `KUBEBUILDER_ASSETS` env var pointing to envtest binaries)

### Tier 3 — E2E (deferred to v2)

Real kind/k3d cluster + real Ollama. Not part of the POC CI. Demo itself serves as the manual E2E gate.

---

## 13. Deferred to v2

Items deliberately out of scope for the POC. Flag for future-you / future-agent.

| Item | What's deferred | Why |
|---|---|---|
| Skills-as-CRDs | `kind: Skill` resource in-cluster instead of filesystem | Cleaner production story (RBAC, namespacing, audit), but circular for POC. v2 once the harness is stable. |
| Leader election | Multi-replica deployment via `coordination.k8s.io/Lease` | Single replica is fine for local-against-remote POC. |
| Custom user-defined tools | Go plugins / wasm / structured skill-local declarations | The escape hatch for skills handling resources where invariant cost can't be expressed in prose. Q9 deliberately rejected built-in composite tools; this is the v2 alternative. |
| Other LLM providers | OpenAI / Anthropic / Together / etc. | `Provider` interface is ready. Add when needed. |
| Startup catch-up | Re-trigger missed schedules on agent restart | v1 relies on informer resync; missed cron fires during downtime are lost. v2 should reconcile by walking `status.fires` and computing missed runs. |
| OpenTelemetry traces / Prometheus metrics | OTLP exporter, metric instrumentation | JSONL traces + slog are enough for POC. Real ops needs OTel. |
| Streaming LLM responses | `ChatStream` method on Provider | Useful for long reconciles or UI; tool-call payloads are atomic so blocking is fine for v1. |
| Per-skill RBAC | Skills inherit the agent's kubeconfig RBAC | v1: one set of permissions for the whole agent. v2: skills could carry per-namespace / per-resource RBAC declarations the harness enforces. |
| `onlyOnGenerationChange` trigger filter | Frontmatter opt-in to skip reconciles when only `status` changed | The standard K8s mechanism: CRDs with `subresources: status: {}` increment `metadata.generation` only on spec writes, not status writes. A skill opts in with `onlyOnGenerationChange: true`; the harness fires it only when `metadata.generation != status.observedGeneration`. After acting, the skill patches `status.observedGeneration = metadata.generation`. Implementation is Option A (standard single `status.observedGeneration` field per object — no per-skill tracking needed). Dropped from POC because the four demo skills are already protected by CEL trigger conditions. Required for production harnesses managing CRDs that follow the standard controller pattern. |

---

## 14. Demo Narrative

### How to present this to an audience

1. **Open with the thesis.** "Business logic in controllers is code today. What if it were prose?"
2. **Show one `SKILL.md` file.** Have them read it. Acknowledge that it reads like a runbook.
3. **`make install-crd && make example-task && make run`**, live.
4. **Watch the JSONL trace file** scroll. This is the demo — not the running terminal, the trace.
5. **Edit a skill file live**, save, watch hot-reload pick it up.
6. **Drop in the judgment skill** (`scheduledtask-auto-suspend-on-failures`). Force a few failed Jobs. Watch the agent auto-suspend.
7. **Show the corresponding Go controller** (the kubebuilder tutorial's ~200 lines). Contrast.
8. **Take grilling.**

### Counterarguments and prepared responses

| Audience question | Honest answer |
|---|---|
| "Isn't this just controller-runtime with extra steps?" | Yes — the *plumbing* is. The point is that the *business logic* (the part that varies per controller) moved from Go to prose. |
| "It's 1000× slower than a Go controller." | Yes. This is for slow control loops, not data-path operations. The latency is the cost of expressiveness. |
| "The LLM is non-deterministic. You can't run this in production." | Correct — this is a paradigm-shift demo, not a production replacement. The interesting question is whether the cost is worth the gain *somewhere*, not whether it's worth the gain *everywhere*. |
| "What stops the LLM from doing something destructive?" | Skill `allowedTools` is enforced at dispatch (not in prompt). The skill literally cannot call `delete` if it's not in the list. Plus the four safeguards in §8.5. |
| "Why not just use controller-runtime + an LLM for the hard cases?" | That's a legitimate alternative (the "judgment overlay" idea generalized). controllerless takes the maximalist position to clarify the tradeoff space. Both directions are defensible. |
| "What about RBAC?" | The agent uses the kubeconfig's RBAC. Per-skill RBAC is a v2 item (see §13). |
| "How do you debug it?" | The JSONL trace is the debug artifact. Walk through one. |

---

## 15. References

- **Kubebuilder CronJob tutorial** — https://book.kubebuilder.io/cronjob-tutorial/cronjob-tutorial.html (reference behavior; not the implementation we replicate)
- **Kubebuilder CronJob controller implementation** — https://book.kubebuilder.io/cronjob-tutorial/controller-implementation.html
- **Kubernetes 1.35 release** — https://kubernetes.io/releases/1.35/
- **client-go releases** — https://github.com/kubernetes/client-go/releases (v0.35.3 target)
- **Ollama API — `/api/chat` with tools** — https://github.com/ollama/ollama/blob/main/docs/api.md
- **Ollama tool-calling docs** — https://github.com/ollama/ollama/blob/main/docs/capabilities/tool-calling.mdx
- **Gemma 4 on Ollama** — https://ollama.com/library/gemma4 (model card; native function-calling, 256K context, native `system` role)
- **Gemma 4 tags** — https://ollama.com/library/gemma4/tags (12b-mxfp8 is MLX, text-only, 12GB)
- **robfig/cron** — https://github.com/robfig/cron (`cron.ParseStandard`)
- **Server-side apply** — https://kubernetes.io/docs/reference/using-api/server-side-apply/
- **client-go dynamic informers** — https://pkg.go.dev/k8s.io/client-go/dynamic/dynamicinformer
- **client-go workqueue** — https://pkg.go.dev/k8s.io/client-go/util/workqueue

---

## Implementation order (suggested for the next session)

A reasonable Phase 1 (per the author's SOP):

1. **Scaffold the repo** — directory tree, `go.mod`, `Makefile`, placeholder `main.go` that `make build` succeeds on.
2. **Author the `ScheduledTask` CRD YAML** — `make install-crd` works against a kind/k3d cluster.
3. **Implement `kube/`** — kubeconfig loading, dynamic client, REST mapper, informer factory, workqueue registry. Smoke test: print events as they arrive.
4. **Implement `skill/`** — loader, frontmatter parser, lint, trigger matching. Smoke test: load the four demo skills.
5. **Implement `tools/`** — primitives + helpers + meta. Unit-test each tool in isolation.
6. **Implement `llm/`** — `Provider` interface, mock provider, then Ollama provider. Test with a trivial skill that just calls `done()`.
7. **Implement `dispatch/`** — the agentic loop, safeguards, prompt assembly. This is the hardest piece.
8. **Implement `trace/`** — JSONL writer, slog setup. Plumb into the dispatcher.
9. **Author the four demo skill bodies** — start with `scheduledtask-fire-due-pod`; iterate prose until reliable on `gemma4:12b-mxfp8`.
10. **End-to-end demo** — `make install-crd && make example-task && make run`; observe traces; demo.

Follow TDD per author CLAUDE.md SOP throughout. Use `make` targets — don't run raw `go build` / `go test` in commands.
