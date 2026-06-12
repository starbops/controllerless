---
name: scheduledtask-auto-suspend-on-failures
description: Automatically suspends a ScheduledTask when its last three fires have all failed.
triggers:
  - gvk: agentic.io/v1alpha1/ScheduledTask
    eventTypes: [Modified]
    conditions:
      - 'size(resource.status.fires) >= 3 && resource.status.fires[size(resource.status.fires)-1].phase == "Failed" && resource.status.fires[size(resource.status.fires)-2].phase == "Failed" && resource.status.fires[size(resource.status.fires)-3].phase == "Failed"'
allowedTools:
  - patch
  - done
---

# What this skill does

Detects when the last three consecutive fires of a ScheduledTask all failed and
automatically sets `spec.suspend = true` to stop further firing, then appends a
Suspended condition to `status.conditions`.

# Procedure

1. **Stability check.** If `spec.suspend == true`, call `done()` — the task is already suspended.

2. **Suspend the task.** Call `patch` to set `spec.suspend = true` using SSA.

3. **Append a Suspended condition.** Call `patch` to append to `status.conditions` an entry with:
   - `type`: `Suspended`
   - `status`: `True`
   - `reason`: `RepeatedFailures`
   - `message`: `Last 3 fires failed`
   - `lastTransitionTime`: current time in RFC3339.

4. **Signal completion.** Call `done()`.

# Errors

If the ScheduledTask is modified concurrently and `spec.suspend` is already set
by the time our SSA field write arrives, treat the result as success — call `done()`.
