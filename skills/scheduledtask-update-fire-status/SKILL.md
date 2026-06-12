---
name: scheduledtask-update-fire-status
description: Updates the status.fires entry for a Pod owned by a ScheduledTask with the Pod's current phase.
triggers:
  - gvk: /v1/Pod
    eventTypes: [Modified]
    conditions:
      - 'has(resource.metadata.labels) && "agentic.io/scheduledtask" in resource.metadata.labels'
allowedTools:
  - get
  - patch
  - done
---

# What this skill does

Syncs the phase of a Pod owned by a ScheduledTask back into the owning task's
`status.fires` entry, so the task's history reflects the current Pod outcome.

# Procedure

1. **Stability check.** If the Pod's `status.phase` already matches the `status.fires` entry's `phase` field, call `done()` and return — no change needed.

2. **Identify the owning ScheduledTask.** Read `metadata.labels["agentic.io/scheduledtask"]` from the Pod — this label holds the owner name; the owner lives in the same namespace.

3. **Fetch the owning ScheduledTask.** Call `get(resource=scheduledtasks, namespace=<pod-namespace>, name=<owner-name>)` to retrieve the latest state.

4. **Find the matching fires entry.** Scan `status.fires` for the entry whose `podName` equals this Pod's `metadata.name`; if not found, call `done()` — the entry was already pruned.

5. **Patch the fires entry.** Call `patch` to update the matched entry's `status.phase` (and `note` if the Pod has a termination message) using SSA.

6. **Signal completion.** Call `done()`.

# Errors

If the owning ScheduledTask is not found (it was deleted before this event
arrived), call `done()` — there is nothing to update.
