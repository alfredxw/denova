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

Games declare `views`, `game.viewId`, and `game.storage` with `kind: "self"` or
`kind: "story"`, and an optional stable `saveFormat`. Story games also declare
`game.story: { modelSlot: "writer" }` referencing a text slot. They reuse the
existing Story engine; the game still owns its entire view and presentation.
Private characters and tools live in `definitions`
and are referenced as `local:id`. Public tool references are `pluginId/localId`;
declare `requires` and `game.uses.toolsets` when using another package's tools.
Public Agent definitions are limited to `builtin/assistant`; games can also use
their own private characters. Model slots use `kind: "text"` or `kind: "image"`
and select host profiles, never credentials. A game's model binding uses
`local:slotId`; an image generation request uses the bare declared `slotId`.

## Connection

The game scaffold's `client.mjs` exports `connect()`. It validates the parent
window and origin plus a random nonce, then returns `{ context, request }`.
Use `request(path, { method, body: JSON.stringify(value) })` for the paths below.
Use `{ responseType: "stream" }` to obtain the Response for SSE or binary assets.
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
| GET /library/items?offset=0&limit=50&query=... | Project library summaries; library.read |
| GET /library/items/{id} | Complete selected library item; library.read |
| GET /assets/content?kind=project&path=... | Authenticated raster bytes; assets.read |
| POST /images/generations | Generate one image using a declared image slot; images.generate |
| GET /images/generations/{commandId} | Image status and persistent result; images.generate |
| POST /images/generations/{commandId}/cancel | Cancel the scoped generation; images.generate |
| GET /story | Bound Story's player-visible snapshot; stories.read |
| GET /story/events?operationId=... | Provisional root prose SSE: reset/delta/settled; stories.read |
| GET /story/history?branchId=...&beforeCursor=...&limit=50 | Older turns; stories.read |
| POST /story/commands | Existing Story actions; stories.write |
| GET /story/records?key=... | Extension-owned JSON; stories.read |
| PUT /story/records | Versioned extension JSON with concurrency guard; stories.write |

Game data requires `gameData`; plugin data uses the corresponding `/plugin-data`
paths and `pluginData`. All paths are portable `/`-separated relative paths.
Use `expectedRevision: null` only when creating a new file, and the revision read
from the server when changing one. On conflict, reread and reconcile. Backends
that own their dataDir must not concurrently write it through these file APIs.
For Story games, use Story records for all application recovery facts. dataDir
contains separate assets, not a second canonical Story database.

## Library and raster assets

All resource APIs use the Project already bound to the credential. Library lists
return `{ items, total, nextOffset? }`, with page limits 1..100. Items contain
`id, type, name, tags, briefDescription, updatedAt, enabled, keywords?, image?`.
Detail reads add `content`; content above 1 MiB is rejected, never truncated.
Select relevant items and budget model input explicitly instead of injecting the
entire library. Images use `{ kind: "project", path: "assets/.../image.png" }`.

Generated images use `{ kind: "generated", path: "opaque-name.png" }`, visible
only to the calling package and scope. Project images must be within `assets/`.
Raster content is limited to PNG, JPEG, WebP or GIF and 32 MiB per file. Reject
unsafe paths; do not store credentials, local runtime URLs or absolute paths.
With the template client, resolve an asset as follows and revoke URLs when done:

```javascript
const response = await request(`/assets/content?${new URLSearchParams(asset)}`,
  { responseType: 'stream' });
const displayURL = URL.createObjectURL(await response.blob());
// Display displayURL, persist only asset, then release it when no longer used.
URL.revokeObjectURL(displayURL);
```

## Direct image generation

POST `/images/generations` with
`{ commandId, modelSlot: "illustrator", prompt, aspectRatio?: "16:9", size?, quality? }`.
Use a 1..128-byte command ID without slashes or surrounding whitespace, and a
nonempty prompt up to 64 KiB. The user selects the image profile; the caller
cannot pass arbitrary profile IDs, credentials or endpoints.

The response is `{ commandId, status, images, error? }`; each image contains
`{ asset: {kind, path}, mimeType, sizeBytes, revisedPrompt? }`. Handle `running`,
`completed`, `failed`, `cancelled`, and `interrupted`. POST returns 202 while
running and 200 when replaying a settled operation. Poll the GET route and keep
cancellation available. The same command and input return the same operation;
changed input conflicts. A disconnected response does not cancel generation.

Direct provider requests have scope-owned receipts and create no Agent Session.
After an uncertain process interruption, a new command is required for an
explicit retry; never automatically repeat a possibly paid request. Adopt the
result into application data only after checking its original source revision.
Generation does not change library image bindings automatically.

## Story reuse and extension records

Story APIs require an explicitly bound `game-instance` scope with StoryID and
InstanceID. Dependencies need their own grants; ordinary host Agent tool scopes
do not gain whole-Story authority. Management creates or adopts the Story; a
package cannot switch to another Project or Story. Execution requires the bound
Project to be open, while snapshots and records can be read independently.

GET `/story` returns `storyId, branchId, title, turns, branches, beforeCursor?,
hasMore, status, operationId?, interruptionId?`. Turns contain `id, revision,
user, narrative, choices, versions`. Branches contain `id, title, current`.
Hidden plans, model messages and tool arguments are absent. Subscribe to `/story/events?operationId=...` for provisional root prose. Events carry `kind` and delta `text`; reset retracts display and settled requires reloading the snapshot. On disconnect, reload and resubscribe to the same operation without repeating generation. Tool, reasoning and subagent events are excluded. History pages contain at most 200 turns.

POST `/story/commands` uses `{ kind, commandId, branchId?, ... }`:

| kind | Additional fields |
| --- | --- |
| advance | message, locale? |
| resume | interruptionId, message?; default is the existing continue instruction |
| regenerate | turnId, message?, locale?; latest turn only, otherwise fork first |
| stop | operationId from the observed snapshot |
| fork | turnId, title? |
| switchBranch | branchId |
| switchVersion | turnId, versionTurnId |

Model advancement reuses the existing Story command and recovery mechanism.
Rendering a line, playing a transition or moving the reading cursor must never
call advance. Closing a game stops only the Story operation it owns. Removing
its binding preserves the underlying Story for the built-in interface.

Read records using `key, branchId?, turnId?, sourceRevision?` query fields.
Write the same address with `{ expectedRevision, schemaVersion, value }`.
Both return `{ revision, schemaVersion, value }`; missing is `0, 0, null`.
Use expectedRevision 0 for creation, the returned revision for subsequent writes,
and reconcile DOCUMENT_CONFLICT instead of overwriting. key is 1..256 bytes,
schemaVersion is positive, and value is valid JSON. The whole request must fit
within 1 MiB. Package ownership is assigned by the host, not by caller input.

Use records without turn/source fields for application data or preferences.
An omitted branchId always addresses main, not the current branch. Derived
presentation must supply the exact branchId, turnId and sourceRevision from its
snapshot. A changed revision or missing branch turn rejects stale writes.
Records are opaque, do not enter model context, and live in the canonical Story
journal. Never write the journal, Project Store or binding metadata directly.
Each private Agent keeps its own journal; a Story branch change does not rewind
that Agent automatically. For a derived transformation, use a session key scoped
to the source revision or another explicit strategy that avoids future memory.

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
answers and completion references in self-managed game data or Story records as needed. Character journals
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

The trusted Extensions page provides the form, validation and save.
Do not implement a settings webview or call management APIs from an extension.
Each release has its own settings/<releaseId>/settings.toml. Tables merge with
release defaults recursively; arrays and scalars replace. Saved changes apply on
next start. Read context.settings from the handshake, GET /context or backend
bootstrap. Preview uses defaults and explicit test values, not installed settings.

Game creation parameters use game.setup with the same declaration format. They
are saved with the instance and exposed as context.setup, separately from release
settings. Updates copy prior overrides only when the target has none. Invalid
inherited values remain available for form repair and block activation, not
installation. Save with {releaseId, expectedRevision, overrides}; older saves do
not constrain another release. Preview identity follows its release; do not send
a preview flag when creating an instance or activating a plugin.
Supported values are strings, booleans, finite safe numbers, arrays and tables.
No null, nonfinite values or native TOML dates; use strings for dates. Settings
and each definition are limited to 1 MiB. Model credentials remain host-owned.
