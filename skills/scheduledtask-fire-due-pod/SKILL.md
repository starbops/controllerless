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

Ensures a ScheduledTask fires on its cron schedule by launching a Pod from
`spec.podSpec` at each scheduled time, with label-based deduplication so
re-reconciles don't produce duplicates.

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
   - Compute nextNextFire by calling `cron.nextFireTime(expr=spec.schedule, after=nextFire)`.
   - Call `requeueAfter(time.secondsUntil(nextNextFire), "next fire time")`.

# Errors

If `create` returns AlreadyExists, treat it as success and continue to step 6 —
this means another reconcile beat us, but the desired state holds.
