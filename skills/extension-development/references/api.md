# Extension API, major version 1

## Source and packaging

Each package root has exactly one `denova.plugin.json` or `denova.game.json`.
Required fields include `manifestVersion: 1`, `apiMajor: 1`, a stable lowercase
package `id`, semantic `version`, `name` with `zh-CN` and `en-US`, and permissions.
Use the generated manifest as the schema example for the chosen package kind.
`distribution.files` is the runtime file/directory allowlist; include referenced
entry points and definitions, exclude credentials, caches and development logs.

Plugins contribute tools and toolsets, not Skills or public Agents. Tool schemas
use JSON Schema 2020-12 and English model-facing descriptions. A tool declares
its effect (`pure`, `read`, `propose`, `write`) and backend HTTP endpoint. The
provider receives the tool input as its JSON request body and returns
`{ content: "English model feedback", data: optionalStructuredResult }`.

Games declare `views`, `game.viewId`, and `game.storage` with `kind: "self"` and
an optional stable `saveFormat`. Private characters and tools live in `definitions`
and are referenced as `local:id`. Public tool references are `pluginId/localId`;
declare `requires` and `game.uses.toolsets` when using another package's tools.
Public Agent definitions are limited to `builtin/assistant`; games can also use
their own private characters. Model slots select host profiles, never credentials.

## Connection

The source template's `client.mjs` exports `connect()`. It validates the parent
window and origin plus a random nonce, then returns `{ context, request }`.
Use `request(path, { method, body: JSON.stringify(value) })` for the paths below.
The wrapper attaches the runtime's bearer token. Do not persist the token, print
it, put it in a URL, or call trusted `/api/platform/manage` routes from a package.
`context` contains source, scope, locale, theme, environment, settings and optional setup.
Handle `denova:appearance` for changes to the single displayed language and theme.

Node backends use `runtime.mjs` and the `denova-runtime-v1` bootstrap protocol.
The host sends bootstrap JSON through stdin, including `packageDir`, `dataDir`,
`tempDir`, connection and `hostToken`. Authenticate every backend endpoint using
that token. Print the ready protocol/port response, support `/__denova/ready`, and
stop on shutdown or stdin closure. Node code runs with the current OS account's
permissions. Scope credentials are not an OS sandbox.

## Consumer paths (relative to the runtime base URL)

| Method and path | Purpose |
| --- | --- |
| GET /context | Runtime scope, extension settings and game setup |
| GET /capabilities | Permissions, capacity limits and schema dialect |
| GET /openapi.json | Exact current request/response contracts |
| GET /contributions | Visible pinned contributions |
| GET /agents/definitions | Available Agent definitions |
| POST /tools/{providerId}/{toolId}/invoke | `{ input: {...} }`; requires tools.invoke |
| GET /game-data/files?directory=... | List this game's data files |
| GET /game-data/file?path=... | `{ content, revision }` |
| PUT /game-data/file | `{ path, content, expectedRevision }` |
| DELETE /game-data/file | `{ path, expectedRevision }` |

Game data requires `gameData`; plugin data uses the corresponding `/plugin-data`
paths and `pluginData`. All paths are portable `/`-separated relative paths.
Use `expectedRevision: null` only when creating a new file, and the revision read
from the server when changing one. On conflict, reread and reconcile. Backends
that own their dataDir must not concurrently write it through these file APIs.

## Agent runs (agents.run permission)

Create or recover a logical session with POST `/agents/sessions`:
`{ projectId: context.scope.projectId, definition: "local:character", key: "stable-character-key" }`.
The response has `ref`, `definition`, and `key`; use `ref.sessionId` thereafter.
A key cannot silently change its definition or model configuration.

POST `/agents/sessions/{sessionId}/runs` with
`{ commandId: "stable-request-id", input: { text: "User input" } }`.
Persist the command ID before sending. The response contains `run`, `status`,
`text`, optional `error`, and optional `completion`. Resume by querying
GET `/agents/sessions/{sessionId}/runs?commandId=...`, not by inventing another ID.
GET `/agents/runs/{runId}` returns the result; `/events` streams SSE.
POST `/agents/runs/{runId}/cancel` cancels without discarding history.

Handle accepted, running, waiting, completed, failed, aborted, and incomplete.
There is no fixed total time/iteration limit. After an interrupted process,
incomplete requests must not automatically replay side effects. Save completed
answers and completion references in game data as needed. Character journals
are not implicitly rewound when a game save is restored.

Errors return `{ code, messageKey, diagnostic }`. Use localized user errors and
English diagnostics in development logs. Read the live OpenAPI for additional
contracts; unsupported platform capabilities must not be presented as working.

## Declarative extension settings

Plugins and games may declare `settings: {schema, defaults, uiSchema?}`. Schema
and optional UI hints are JSON; defaults must be TOML. Distribute every referenced
file. The schema root is an object using JSON Schema 2020-12 with no external
references. Use TOML as the only default source, never schema.default. Defaults
must satisfy required fields. Each property needs x-titleKey; descriptions use
x-descriptionKey. Keys must resolve to strings in both manifest.locales files.
Localized enum labels use oneOf entries with const and x-titleKey. UI hints may
use ui:titleKey, ui:descriptionKey, ui:helpKey and ui:placeholderKey.

The trusted Extensions page provides the form, TOML editor, validation and save.
Do not implement a settings webview or call management APIs from an extension.
One user settings.toml per package is shared across releases. Tables merge with
release defaults recursively; arrays and scalars replace. Saved changes apply on
next start. Read context.settings from the handshake, GET /context or backend
bootstrap. Preview uses defaults and explicit test values, not installed settings.

Game creation parameters use game.setup with the same declaration format. They
are saved with the instance and exposed as context.setup, separately from shared
settings. Keep schema changes compatible with releases still pinned by saves;
the host rejects incompatible saves and installs without silently resetting data.
Supported values are strings, booleans, finite safe numbers, arrays and tables.
No null, nonfinite values or native TOML dates; use strings for dates. Settings
and each definition are limited to 1 MiB. Model credentials remain host-owned.
