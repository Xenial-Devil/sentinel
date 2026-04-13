# Sentinel vs Watchtower: Deep Code Comparison (Current State)

## Scope

Code-verified comparison against the current repository state.

Date: 2026-04-13

Watchtower references:

- https://containrrr.dev/watchtower/
- https://containrrr.dev/watchtower/arguments/
- https://containrrr.dev/watchtower/container-selection/
- https://containrrr.dev/watchtower/http-api-mode/
- https://containrrr.dev/watchtower/lifecycle-hooks/
- https://containrrr.dev/watchtower/notifications/

## Executive Summary

Sentinel has moved forward again and now includes several controls that were previously missing versus Watchtower.

1. Include/exclude by container name is now active in [watcher/filter.go](watcher/filter.go#L36) and [watcher/filter.go](watcher/filter.go#L48).
2. Run-once mode is active in [watcher/watcher.go](watcher/watcher.go#L96).
3. Include-restarting and revive-stopped flows are wired through [docker/container.go](docker/container.go#L68) and [watcher/watcher.go](watcher/watcher.go#L94).
4. Lifecycle hooks are now implemented globally and per-container via [config/config.go](config/config.go#L138), [watcher/watcher.go](watcher/watcher.go#L181), and [hooks/hooks.go](hooks/hooks.go#L154).
5. Compose stack update endpoint is now exposed in [api/routes.go](api/routes.go#L323).
6. Notifier templating and richer channel payloads are now present via [notifier/notifier.go](notifier/notifier.go#L45) and [notifier/template.go](notifier/template.go#L29).
7. Earlier fixes remain in place: shared approval manager, digest-key approvals, rollback result propagation, and better host:port parsing.

The biggest remaining differences versus Watchtower are now mostly behavioral correctness and operational maturity: hook execution correctness, semver policy effectiveness, no-pull semantics, cleanup side effects, async API result visibility, partial private-registry auth flow, and incomplete telemetry/event coverage.

## Status Legend

- Active: implemented and wired in runtime path
- Partial: wired, but semantics or coverage are incomplete
- Present: module/config exists but mostly not used by runtime path
- Missing: not implemented

## Feature Matrix

### 1) Selection and Targeting

| Capability                          | Sentinel | Watchtower | Notes                                                                                           |
| ----------------------------------- | -------- | ---------- | ----------------------------------------------------------------------------------------------- |
| Watch all containers                | Active   | Active     | Default remains opt-in label mode in [config/config.go](config/config.go#L111)                  |
| Label opt-in                        | Active   | Active     | Configurable key/value in [config/config.go](config/config.go#L113)                             |
| Disable by label                    | Active   | Active     | [watcher/filter.go](watcher/filter.go#L42)                                                      |
| Scope partitioning                  | Active   | Active     | [watcher/filter.go](watcher/filter.go#L62)                                                      |
| Legacy label support                | Active   | N/A        | [watcher/filter.go](watcher/filter.go#L85)                                                      |
| Typo-label compatibility            | Active   | N/A        | [watcher/filter.go](watcher/filter.go#L13) and [watcher/filter.go](watcher/filter.go#L123)      |
| Include stopped containers          | Active   | Active     | [docker/container.go](docker/container.go#L67)                                                  |
| Include/exclude by container name   | Active   | Active     | [watcher/filter.go](watcher/filter.go#L36) and [watcher/filter.go](watcher/filter.go#L48)       |
| Include restarting / revive stopped | Active   | Active     | [docker/container.go](docker/container.go#L68) and [watcher/watcher.go](watcher/watcher.go#L94) |

### 2) Scheduling and Triggering

| Capability                      | Sentinel | Watchtower | Notes                                                                                                                            |
| ------------------------------- | -------- | ---------- | -------------------------------------------------------------------------------------------------------------------------------- |
| Interval polling                | Active   | Active     | [scheduler/scheduler.go](scheduler/scheduler.go#L78)                                                                             |
| Cron scheduling                 | Active   | Active     | [scheduler/scheduler.go](scheduler/scheduler.go#L51)                                                                             |
| Startup initial cycle           | Active   | Active     | [scheduler/scheduler.go](scheduler/scheduler.go#L85)                                                                             |
| Run once and exit               | Active   | Active     | [watcher/watcher.go](watcher/watcher.go#L96)                                                                                     |
| HTTP trigger update all         | Active   | Active     | [api/routes.go](api/routes.go#L92)                                                                                               |
| HTTP trigger targeted container | Partial  | N/A        | Targeting exists, but response remains async-only in [api/routes.go](api/routes.go#L134) and [api/routes.go](api/routes.go#L143) |
| API auth token middleware       | Active   | Active     | [api/middleware.go](api/middleware.go#L21)                                                                                       |
| API-only execution model        | Partial  | Active     | Scheduler still runs when API is enabled in [main.go](main.go#L75) and [main.go](main.go#L84)                                    |

### 3) Update Execution Controls

| Capability               | Sentinel | Watchtower | Notes                                                                                                                                                                                   |
| ------------------------ | -------- | ---------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Digest-based detection   | Active   | Active     | [updater/updater.go](updater/updater.go#L84)                                                                                                                                            |
| Pull + recreate update   | Active   | Active     | [updater/updater.go](updater/updater.go#L147)                                                                                                                                           |
| Stop timeout control     | Active   | Active     | [config/config.go](config/config.go#L126)                                                                                                                                               |
| Cleanup old image        | Partial  | Active     | Called after any updated result in [watcher/watcher.go](watcher/watcher.go#L310) and [watcher/watcher.go](watcher/watcher.go#L326)                                                      |
| No-restart mode          | Active   | Active     | Pull without restart path in [updater/updater.go](updater/updater.go#L123)                                                                                                              |
| Rolling restart          | Partial  | Active     | Implemented per-container in [updater/updater.go](updater/updater.go#L230), not service-aware orchestration                                                                             |
| No-pull mode             | Partial  | Active     | Still executes restart paths without freshness check in [updater/updater.go](updater/updater.go#L67)                                                                                    |
| Remove anonymous volumes | Partial  | Active     | Config and runtime path exist in [config/config.go](config/config.go#L129) and [watcher/watcher.go](watcher/watcher.go#L322), but behavior differs from Watchtower's recreate semantics |

### 4) Safety and Governance

| Capability                 | Sentinel | Watchtower      | Notes                                                                                                                                                                                                                                          |
| -------------------------- | -------- | --------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Approval workflow          | Partial  | Not core        | Shared manager and digest-key approvals are wired, but caveats remain                                                                                                                                                                          |
| Health gating after update | Active   | Partial         | [updater/rollback.go](updater/rollback.go#L53)                                                                                                                                                                                                 |
| Automatic rollback         | Active   | Not core        | [updater/updater.go](updater/updater.go#L199)                                                                                                                                                                                                  |
| Semver policy gate         | Partial  | Different model | Logic exists but is effectively non-restrictive for current flow                                                                                                                                                                               |
| Lifecycle hooks            | Partial  | Active          | Hook pipeline exists in [watcher/watcher.go](watcher/watcher.go#L187) and [watcher/watcher.go](watcher/watcher.go#L271), but runtime caveats remain in [watcher/watcher.go](watcher/watcher.go#L183) and [hooks/hooks.go](hooks/hooks.go#L128) |

### 5) API and Control Plane

| Capability                    | Sentinel | Watchtower | Notes                                                                                                                                                                     |
| ----------------------------- | -------- | ---------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Health endpoint               | Active   | Active     | [api/routes.go](api/routes.go#L55)                                                                                                                                        |
| Info endpoint                 | Active   | Active     | [api/routes.go](api/routes.go#L64)                                                                                                                                        |
| Update all                    | Active   | Active     | [api/routes.go](api/routes.go#L92)                                                                                                                                        |
| Targeted update               | Partial  | N/A        | Async accepted, runtime result not returned to caller                                                                                                                     |
| Approvals list/approve/reject | Active   | N/A        | Shared singleton path in [api/routes.go](api/routes.go#L149)                                                                                                              |
| Compose stacks list/details   | Active   | N/A        | [api/routes.go](api/routes.go#L241) and [api/routes.go](api/routes.go#L296)                                                                                               |
| Compose stack update endpoint | Partial  | N/A        | Exposed via [api/routes.go](api/routes.go#L323) and [api/routes.go](api/routes.go#L366), but implementation is restart-based in [compose/stack.go](compose/stack.go#L123) |

### 6) Notifications and Webhooks

| Capability                        | Sentinel | Watchtower       | Notes                                                                                                                                                                                                         |
| --------------------------------- | -------- | ---------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Slack                             | Active   | Active           | [notifier/notifier.go](notifier/notifier.go#L52)                                                                                                                                                              |
| Email                             | Active   | Active           | [notifier/notifier.go](notifier/notifier.go#L57)                                                                                                                                                              |
| Teams                             | Active   | Active           | [notifier/notifier.go](notifier/notifier.go#L62)                                                                                                                                                              |
| Startup notification              | Active   | Active           | [watcher/watcher.go](watcher/watcher.go#L94)                                                                                                                                                                  |
| Webhook events                    | Partial  | Active ecosystem | Event catalog is broader than emitted runtime subset                                                                                                                                                          |
| Rich notifier ecosystem/templates | Partial  | Active           | Template engine and custom templates are present in [notifier/template.go](notifier/template.go#L29), [notifier/template.go](notifier/template.go#L88), and [notifier/notifier.go](notifier/notifier.go#L103) |

### 7) Metrics and Observability

| Capability                  | Sentinel | Watchtower | Notes                                                                                           |
| --------------------------- | -------- | ---------- | ----------------------------------------------------------------------------------------------- |
| Prometheus endpoint         | Active   | Active     | [metrics/metrics.go](metrics/metrics.go#L91)                                                    |
| Watched/pending gauges      | Active   | Active     | [watcher/watcher.go](watcher/watcher.go#L132) and [watcher/watcher.go](watcher/watcher.go#L152) |
| Success/failure counters    | Active   | Active     | [watcher/watcher.go](watcher/watcher.go#L286) and [watcher/watcher.go](watcher/watcher.go#L312) |
| Pull failed counter         | Partial  | Active     | Defined in [metrics/metrics.go](metrics/metrics.go#L129), not wired                             |
| Duration histograms         | Partial  | Active     | Defined in [metrics/metrics.go](metrics/metrics.go#L146), not wired                             |
| Machine-readable run report | Missing  | Active     | Watchtower has stronger reporting outputs                                                       |

### 8) Registry and Connectivity

| Capability                          | Sentinel | Watchtower | Notes                                                                                                   |
| ----------------------------------- | -------- | ---------- | ------------------------------------------------------------------------------------------------------- |
| Docker host override                | Active   | Active     | [docker/client.go](docker/client.go#L31)                                                                |
| Docker TLS verify/cert path         | Active   | Active     | [docker/client.go](docker/client.go#L38)                                                                |
| Image ref parsing host:port support | Active   | Active     | [registry/registry.go](registry/registry.go#L41) and [updater/semver.go](updater/semver.go#L156)        |
| Private registry auth maturity      | Partial  | Active     | Private flow currently returns auth-not-configured in [registry/registry.go](registry/registry.go#L138) |
| Credential helper integration       | Partial  | Active     | Helper exists in [registry/auth.go](registry/auth.go#L30), but is not integrated                        |

## Deep Findings (Current High Impact)

1. Lifecycle hooks can duplicate and become non-deterministic across cycles.
   Per-container hooks are registered on every check in [watcher/watcher.go](watcher/watcher.go#L181) and [watcher/watcher.go](watcher/watcher.go#L183), while registration appends into persistent hook lists in [hooks/hooks.go](hooks/hooks.go#L58). This can multiply hook executions over time.

2. Hook execution shell is hardcoded and portability is limited.
   Hook commands are executed via `sh -c` in [hooks/hooks.go](hooks/hooks.go#L128), which is not portable to all host environments.

3. Semver policy is present but effectively non-restrictive in current update flow.
   The updater sets candidate tag equal to current tag in [updater/updater.go](updater/updater.go#L106), and policy helper returns allow when tags are equal in [updater/semver.go](updater/semver.go#L110). Result: policy does not constrain digest-driven updates.

4. No-pull behavior can force periodic restarts and bypass approval mode.
   No-pull enters restart paths directly in [updater/updater.go](updater/updater.go#L67), and approval gate only applies when noPull is false in [watcher/watcher.go](watcher/watcher.go#L214).

5. Cleanup can run in restart-only/no-pull paths where no new image was pulled.
   Cleanup is executed for any updated result in [watcher/watcher.go](watcher/watcher.go#L310) and [watcher/watcher.go](watcher/watcher.go#L326), including restart-only updates from [updater/updater.go](updater/updater.go#L301).

6. Targeted update API remains asynchronous with accepted-only response.
   Route immediately returns 202 in [api/routes.go](api/routes.go#L143) while update runs in goroutine in [api/routes.go](api/routes.go#L134), so caller does not receive execution outcome inline.

7. Approval manager has a concurrency correctness issue in pending listing.
   GetPending acquires read lock in [approval/approval.go](approval/approval.go#L201) and mutates request status in [approval/approval.go](approval/approval.go#L207), which is unsafe under read locking.

8. Private registry support is improved but still shallow.
   Registry parsing and transport are better, but private registries currently fail with explicit auth-not-configured path in [registry/registry.go](registry/registry.go#L138), and credentials loader in [registry/auth.go](registry/auth.go#L30) is not integrated.

9. Event and metrics coverage are still partial.
   Webhook declares events like [webhook/events.go](webhook/events.go#L9), [webhook/events.go](webhook/events.go#L12), and [webhook/events.go](webhook/events.go#L14), but watcher emits only subset events in [watcher/watcher.go](watcher/watcher.go#L258). Duration/pull-failure metrics are defined in [metrics/metrics.go](metrics/metrics.go#L129) and [metrics/metrics.go](metrics/metrics.go#L146), but not wired by updater/watcher.

10. Documentation drift exists in compatibility label note.
    Code supports typo-label compatibility via [watcher/filter.go](watcher/filter.go#L13) and [watcher/filter.go](watcher/filter.go#L123), while README compatibility line is currently misleading in [README.md](README.md#L98).

## What Sentinel Now Does Better Than Before

1. Include/exclude by container name is now first-class in watcher filtering.
2. Run-once execution mode is now available.
3. Include-restarting and revive-stopped capabilities are now wired in runtime.
4. Lifecycle hooks are now implemented at global and per-container levels.
5. Compose stack update endpoint now exists in the API.
6. Notification templating and richer multi-channel payload support are now implemented.
7. Shared approval manager between API and watcher.
8. Update-first approval gating with digest-specific request keys.
9. Rollback outcome propagation into update result handling.
10. Better image reference parsing and host:port handling.

## Watchtower Areas Still Ahead

1. More complete API-mode ergonomics (result visibility/job style tracking).
2. Broader notifier/reporting ecosystem maturity and integrations.
3. More complete private-registry auth ergonomics and battle-tested edge behavior.
4. More mature hook behavior guarantees and portability.

## Recommended Priority Order

1. Fix hook registration growth by de-duplicating per-container hook registration across cycles.
2. Make hook execution portable (avoid hardcoded `sh -c`, or adapt by runtime/OS).
3. Fix semver effectiveness by introducing a real candidate version signal or remove ineffective policy enforcement path.
4. Redesign no-pull semantics to avoid forced restart loops and enforce consistent approval behavior.
5. Gate cleanup and remove-volumes behavior by actual image replacement event rather than generic updated status.
6. Add synchronous status option or job-tracking endpoint for targeted update requests.
7. Fix approval locking in GetPending (no mutation under read lock).
8. Integrate credential helper into private registry auth flow.
9. Expand emitted webhook event coverage and wire remaining metrics.
