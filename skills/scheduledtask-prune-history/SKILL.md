---
name: scheduledtask-prune-history
description: Prunes status.fires entries beyond historyLimit, deleting the associated Pods.
triggers:
  - gvk: agentic.io/v1alpha1/ScheduledTask
    eventTypes: [Modified]
    conditions:
      - 'size(resource.status.fires) > resource.spec.historyLimit'
allowedTools:
  - get
  - list
  - patch
  - delete
  - done
---

# What this skill does

Keeps `status.fires` from growing unbounded by removing the oldest entries
beyond `spec.historyLimit` and deleting their associated Pods.

# Procedure

1. **Stability check.** If `status.fires` has no more entries than `spec.historyLimit`, call `done()` — the history is already within bounds.

2. **Fetch the current state.** Call `get(resource=scheduledtasks, namespace=<namespace>, name=<name>)` to obtain the latest `status.fires` array.

3. **Sort the fires array.** Order `status.fires` by `scheduledAt` ascending so the oldest entries come first.

4. **Remove excess entries.** While the fires array exceeds `spec.historyLimit` in length: take the oldest entry, call `delete(resource=pods, namespace=<namespace>, name=<podName>)` (ignore NotFound), and drop that entry from the working set.

5. **Patch the pruned status.** Call `patch` to replace `status.fires` with the trimmed array using SSA.

6. **Signal completion.** Call `done()`.

# Errors

If `delete` returns NotFound for a Pod, ignore the error and continue — the Pod
was already removed by another process or the ownerReference GC collected it.
