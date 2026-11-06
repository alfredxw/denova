# App product analytics

Release builds send optional, content-free product events to PostHog US Cloud.
The public capture token and ingestion endpoint are in
`internal/analytics/project.go`. A capture token grants ingestion only; never
ship a personal or project secret management API key. End users do not need an
`.env` file, Node.js, an SDK setup wizard, or a PostHog account.

The App no longer sends anything to Umami. The separately hosted public website
is unchanged and still has its own analytics integration. Website download
clicks and App activity do not share an attribution identity.

## Collection and identity

- Source builds and `--dev-mode` never read host analytics state or send events.
- The notice explains what Denova collects, how it counts return visits, and
  where to turn off sharing. Collection starts only after acknowledgement.
- On upgrading from the last Umami release, an enabled instance must acknowledge
  the new collector. A previous opt-out remains disabled without a new prompt.
- The existing random `app_<UUID>` identity, preference and first-active date
  remain in `os.UserConfigDir()/Denova/analytics.json`. This host-user state is
  outside portable `.denova` data. Browsers and LAN clients of a host share one
  identity. Reinstallation preserving this state preserves identity; deleting
  the state starts a new observation. Counts describe instances, not natural people.
- `cohort_origin=migrated` and `first_app_version=unknown` identify previously
  active instances. `new_instance` means first observed participating activity,
  not proof of a new download or a new person. Historical activity is not replayed.

Only the typed allowlist in `internal/analytics/events.go` is serialized.
Common properties are `source=app`, `schema_version=2`, `app_version`, `os`,
and bounded cohort dates/origin/version where known. `distinct_id` is the local
random identity. `cohort_date` retains its original meaning: first effective
activity, while `first_seen_date` is first observation by the new collector.
For new instances, `first_app_version` is the first PostHog-observed version;
it is not an inferred original installation version.

No manuscripts, prompts, AI replies, images, project names/IDs, file paths,
model names, API endpoints/keys, error text, stacks, browser URLs, DOM content,
or clipboard data are uploaded. Settings analytics accepts theme and language
only. Exception details remain in existing local logs. No browser PostHog SDK,
autocapture, session replay, automatic exception capture, or LLM tracing is enabled.
Cloud receives the outbound connection IP. The random persistent ID makes this
pseudonymous analysis, not absolute anonymity.

## Events and counting rules

| Event | Trigger and deduplication | Specific properties |
| --- | --- | --- |
| `app_first_seen` | Once when a participating instance is first observed by PostHog | Cohort metadata |
| `app_opened` | First authenticated UI opening per backend process, after acknowledgement | Cohort metadata |
| `app_first_active` | First effective activity in the instance's lifetime; existing instances do not emit it again | Cohort metadata |
| `app_active_day` | First effective user activity per UTC date | Cohort metadata |
| `feature_active` | First effective activity per feature per UTC date | `feature_name` |
| `feature_used` | Every 30-second focused reading milestone, changed chapter save, admitted execution, or completed explicit action | `feature_name`, `operation`, `trigger`; runs also carry `runtime`, `run_id` |
| `feature_completed` | One successfully ended execution | Run fields, `outcome=success`, numeric `duration_ms` |
| `feature_failed` | One failed execution | Run fields, `outcome=failure`, numeric `duration_ms`, `error_code` |
| `feature_ended` | One cancelled, suspended, or preempted execution | Run fields, actual `outcome`, numeric `duration_ms` |
| `app_activated` | First successful user-initiated execution or changed chapter save in each feature | `feature_name` |
| `core_active_day` | First such successful user work per feature per UTC date | `feature_name` |
| `action_completed` | Successful book creation, book export, or project version restore | `feature_name`, `action` |
| `settings_changed` | Successfully changed effective theme or language | `setting_name`, enum `value` |
| `app_error` | Registered authenticated API error, fixed browser exception kind, or recovered API panic | `error_source`, `error_code`; API errors include HTTP `status_code` and a fixed feature when known |

Features are `writing`, `game`, `chat`, `image`, `library`, `versions`,
`automation`, `projects`, and `resources`. A read is 30 seconds with the page
visible, focused, and relevant content present for writing/game. An empty page,
server startup, automatic save without changes, model tool round, or sub-Agent
does not independently create daily activity. Settings changes and diagnostic
events alone also do not create creative activity.

`feature_used.operation` distinguishes `read`, `edit`, `run`, and the
three explicit actions. These are qualified milestones/operations, not every
mouse click or a measurement of total reading time. Manual edit activation
requires a changed Markdown file in a Book Project's `chapters/` directory;
editing config/code or saving an unchanged file does not qualify.

Automation executions use `trigger=automation`. They create execution events
but never daily user activity, activation or core activity. Select
`trigger=user` when analyzing user execution counts. Runs are whole admitted
executions, including external runtimes, not every model or tool round.
`duration_ms` includes the whole run and is not first-token latency.

Success is a technical outcome, not creative quality or proof that changes were
kept. Core work is a proxy based on successful user-originated work. Overnight
runs can finish on a different day without a user currently present; completion
day core counts are not necessarily a subset of foreground DAU.

The same random `run_id` joins a `feature_used` event with `operation=run`
to exactly one terminal event. A run that never finishes is not a success or a
failure. Suspensions/cancellations/preemptions are reported separately. Some
admission/configuration failures occur before a run handle exists; HTTP failures
are diagnosed separately rather than invented as failed executions.

## Errors and crashes

Execution errors are classified from structured provider/network/filesystem
errors into `authentication`, `rate_limit`, `timeout`, `network`,
`provider_unavailable`, `invalid_request`, `permission_denied`, `not_found`,
`cancelled`, or `unknown`. Raw text is never used as a cloud property.
Adapters that lose structured causes may produce `unknown`.

Browser kinds are `react_error`, `window_error`, `unhandled_rejection`,
`white_screen`, and `startup`. The API accepts only those enums, rejecting
messages, stacks and extra fields. Browser error delivery failures are ignored
and cannot recursively generate more error reports.

API errors use `error_source=api`, `error_code=http_error`, a status, and a
locally derived feature. No URL, route parameter or response text is serialized.
A 4xx may be validation or an expected conflict; do not treat all API errors as
bugs. An execution failure may also lead to an HTTP error: do not sum both
series and call it a count of distinct incidents.

`panic_recovered` means Hertz recovered a request panic. It is not a process
crash. There is deliberately no misleading `app_crashed` counter. Hard crashes,
forced kills, power loss, frontend failures without a working backend, and
failures before acknowledgement may have no report. These custom events support
product error analysis; they are not native PostHog Error Tracking issues with
uploaded exception stacks.

## PostHog dashboard recipes

Apply `source=app` and `schema_version=2` to every App insight. Exclude
`source=app-test` smoke data. Set calendar reporting to UTC to align with the
client's per-day counters. Unique-user counts here mean unique random instances.

| Question | Insight configuration |
| --- | --- |
| Effective DAU | Trends: `feature_used`, `trigger=user`, Unique users, daily |
| Rolling WAU/MAU | Same qualified user-use event, Weekly active users / Monthly active users; do not sum daily counts |
| Opens vs effective use | Separate trends for `app_opened` and `app_active_day`; a process can remain running for days, so opening count is not DAU |
| Feature reach | `feature_used`, `trigger=user`, Unique users, breakdown `feature_name` |
| Feature usage frequency | `feature_used`, Total count, `trigger=user`, breakdown `feature_name` and `operation` |
| User execution results | Terminal events, filter `trigger=user`, breakdown feature/runtime/outcome; show cancellation and suspension separately |
| Completed-run failure proportion | `feature_failed / (feature_failed + feature_completed)` with identical filters; label as observed ended-run failure proportion |
| Full admitted-run outcomes | SQL: join starts with terminals by `run_id`, selecting a start cohort and observation cutoff; keep unmatched starts separate as unfinished/unknown |
| Latency | `feature_completed` or `feature_failed`, numeric property `duration_ms`, Median/P95, with the same feature/runtime/trigger filters |
| Initial activation | Funnel `app_first_seen → app_activated`, final step filtered to writing or game; choose and label a conversion window |
| Activation retention | Retention start `app_activated` (first-ever), return `core_active_day`; filter the same feature on both steps, exact calendar days, UTC |
| Writing/game return visits | Start/return `feature_active`, with the same feature filter, choosing first-ever or recurring deliberately |
| Errors | `app_error`, Total count and Unique users, breakdown error_source/error_code/status_code; run failures are a separate chart |
| Preferences | `settings_changed`, breakdown setting_name/value |

Use qualified `feature_used` events for primary DAU/reach: multiple received
observations give PostHog more opportunities to observe an active instance.
The deduplicated `app_active_day` and `feature_active` markers remain useful
cross-checks, but a lost once-per-day marker must not erase otherwise received
activity from the main report. Do not use all events for DAU: automated runs
and diagnostics would inflate it.

For new-user activation cohorts, select `cohort_origin=new_instance`; analyze
migration separately. PostHog's first received event is not the original
installation date. `first_app_version` is unknown for migrated instances;
filtering current `app_version` is not first-version cohort analysis.

Native PostHog retention and funnel reports can compute these from events.
Do not compare old Umami period-marker totals to PostHog unique instances.
Strict D7 means activity on the seventh calendar day, not any return within
seven days. Choose PostHog's calendar/relative and exact/unbounded options
explicitly; wait until each cohort has matured before comparing rates.

## Managed cloud dashboard

[Denova App product analytics](https://us.posthog.com/project/685522/dashboard/2256487)
contains 17 saved insights in three sections. The project timezone is UTC and
the dashboard remains private. The two wizard-generated dashboards are preserved.

- Activity: DAU/rolling WAU/MAU, selected-period feature reach and use counts,
  daily feature reach/frequency, and a seven-day new-instance activation funnel.
- Retention: separate writing/game first-activation heatmaps through D30, plus
  a D1/D7/D30 summary with explicit mature cohort sizes and return counts.
- Execution quality: observed ended-run success rate, outcome trends, admitted
  starts joined to observed ends, P50/P95 duration by feature/runtime/outcome,
  daily success duration, error trend and affected instances, application error
  classification, execution failure causes, and ingestion status.

The retention summary selects activation cohorts using the chosen date range
and observes same-feature returns through the present. A Dn denominator includes
an instance only after its entire nth UTC calendar day has ended. A missing
return counts as no observed return; an immature instance is excluded. A zero
denominator produces `NULL`, not a misleading zero-percent rate. The admitted-run
table similarly selects starts by date range and observes matching terminal
events through the present; unmatched starts remain unfinished or missing.

SQL tables use `{filters}` for the selected date range and fixed production
predicates for `source=app` / `schema_version=2`. Unique instances over a selected
period are computed directly rather than summed from daily counts. Automated
executions are excluded from user activity and user execution reports. The
native daily latency chart uses milliseconds; the grouped latency table uses
seconds and shows its sample size.

The reusable management script requires Node 22+ and a personal API key in the
process environment or ignored repository-root `.env`, named
`POSTHOG_PERSONAL_API_KEY`. It checks that the project's public capture token
matches the App before making changes. Never add the personal key to App source,
frontend environment variables, build inputs, documentation, or version control.
The key is needed only to administer/query Cloud; App capture and saved dashboards
do not depend on it remaining in this file.

```sh
node scripts/configure-posthog.mjs --verify
node scripts/configure-posthog.mjs --apply
```

Verification executes every query, checks saved definitions and layout, and
runs query-only fixtures for distinct counts, success denominators, P50, empty
denominators, strict retention days, immature cohorts, and missing returns.
Fixtures do not ingest synthetic events. `--apply` idempotently creates or
updates the tagged dashboard and insights, preserving unrelated dashboards.

At setup, Cloud contained only the excluded `analytics_smoke_test` event and no
schema 2 production App events. Query and configuration validation passed, but
real population metrics and retention cannot be validated until users upgrade,
acknowledge the new collector, and generate enough activity. Source/dev builds
remain excluded. API verification does not substitute for a signed-in visual
review of the Cloud dashboard.

## Delivery and verification

The existing in-memory queue remains bounded to 128 events. Requests have a
five-second deadline. Opt-out cancels in-flight requests, discards queued data,
and invalidates existing run handles. Preference persistence failure also fences
collection in memory. Source/test runtimes remain off Cloud.

Events are best effort. Daily markers are persisted before delivery and no
offline history is stored or backfilled. Shutdown cancels delivery and waits at
most one second. Network/queue failures can lose starts or ends independently.
These are successfully received samples, not a complete census. Requests do not
block user model execution. Capturing uses PostHog's public HTTP protocol rather
than a second SDK queue, preserving the existing cancellation guarantees.

The response must acknowledge success (`status: "Ok"` or `status: 1`), with no
quota-limited products. A successful response confirms collector acceptance,
not that a particular dashboard has already processed and displayed the event.

A live migration smoke event uses a synthetic `app_test_<UUID>` identity,
`event=analytics_smoke_test`, and `source=app-test`. Production code cannot
reclassify developer traffic as production; ordinary tests use local collectors,
temporary state and isolated services. No management key is required for capture.
Creating saved cloud dashboards or querying private cloud data requires a
separate authenticated account/API session.

Official references:
[Capture protocol](https://posthog.com/docs/api/capture),
[Retention](https://posthog.com/docs/product-analytics/retention),
[Aggregations](https://posthog.com/docs/product-analytics/trends/aggregations).
