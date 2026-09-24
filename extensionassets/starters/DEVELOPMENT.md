# Develop this extension

This is an ordinary source Project. Use Workbench for Agent conversations, files,
terminal and source version history. The extension-development Skill provides
implementation guidance and the current public HTTP API reference.

## Start with the scaffold

The project kind selects one neutral scaffold, not a genre or a fixed feature set.

- Plugin: denova.plugin.json declares a Node backend and one replaceable echo
  tool. server.mjs handles requests, tools/echo.json describes its input/output,
  and runtime.mjs handles the host handshake, authentication and shutdown.
  Test with {"text":"Hello"}. Replace the tool and its declaration with your
  own behavior. Model configuration and extension settings are not prerequisites.
- Game: denova.game.json declares a static view. index.html, game.mjs and
  style.css form an independent UI; client.mjs provides a scoped host connection.
  locales/ contains separate Chinese and English strings. The starter has no
  characters, game rules, model slots, settings or persistent state.

The starter is runnable so you can verify the integration before adding features.
Read the existing files before changing them. You may replace the whole game UI
or plugin implementation without selecting another template or host mode.
Keep model-facing descriptions, prompts and tool feedback in English.

## Add only the capabilities your extension needs

All platform operations are ordinary HTTP. For a game, await connect() from
client.mjs, then use client.request('/context'), '/capabilities' and '/openapi.json'
to inspect the bound scope and precise contracts. Reuse the existing handshake;
never persist credentials, runtime ports or absolute host paths.

- Tools/services belong to plugins. Add tool definitions and toolsets to
  contributes, include their files in distribution.files, and implement the
  declared backend endpoints. Skills and public Agent profiles remain in their
  own management pages and are not distributed by plugin manifests.
- A game owns its UI and interactions. Use game.storage.kind = "self" for
  independent state, or "story" to reuse the existing Story engine through
  /story and /story/commands. The host does not impose characters or presentation.
- Add modelSlots only when invoking model capabilities. Model bindings are
  selected for a test run or by the consumer; provider credentials stay in the host.
- Declare permissions for APIs actually used. Add extension-wide settings or
  per-story starting options only when consumers need control; see below.
- Keep Chinese and English UI resources independent and react to
  denova:appearance when the host language or theme changes.

## Test, publish and export

Test run validates the current source and opens isolated test data. It owns model
selection and test configuration. It does not install the extension. Source edits
require a new test run; existing saves and installed releases remain frozen.
Build and test feedback is attached to later development Agent messages. Games
have separate test saves; close a running preview before resetting its save.

Static code needs no build. If your implementation needs a build, declare
development.build.command and development.build.args and run it through More
development actions > Build in the existing visible terminal before testing or
publishing. The Node plugin starter requires Node.js on the host.

Publish locally asks for the basic details, saves source editors, validates the
package and installs the checked bytes on this device. This action enables the
manifest's required capabilities. Existing optional grants are retained only
while still declared; consumers manage optional grants in extension settings.
It never asks for a model or stores test configuration in the package.

Use the extension's Export package action in Extensions to share the installed
release. Export uses those exact frozen bytes even after source edits or with no
source Project present. Only package files are exported, not local settings,
credentials, test saves or conversations. Keep distribution.files restricted to
runtime assets; exclude source-only guides, caches and node_modules.
Keep the package ID stable across renames. Use semantic versions to communicate
compatibility. Changed source bytes can be published with the same version.
Both Chinese and English names and descriptions must be maintained in the source;
the initial Project name is used for both until translated.

## Persistence and recovery

Persist each costly request's commandId and exact input before sending it.
Recover with the same ID after refresh; query the existing operation before
offering an explicit new attempt. Keep cancellation available. An interrupted
image request is not automatically safe to repeat. Bound model input explicitly
when selecting library content; reject oversized content instead of silently
truncating it. A branch switch or revision change must not adopt stale results.

For self-managed games, dataDir remains the backend or file API's own data. For
Story games, dataDir is for separate assets: all binding and application recovery
facts belong to the canonical Story journal through public APIs. The platform
stores direct image request receipts and generated assets beside dataDir and
includes them in the scope's backup/export. Do not edit these managed files or
any Agent journal directly. A Story branch change does not implicitly rewind an
independent private Agent; scope derived-work session keys to their source.

After installing a game, open Game and create a storyline with that game type.
Enable or disable installed games and plugins in Extensions. Disabling blocks
new starts while active journeys and tasks continue until stopped.

Extension settings use manifest.settings with JSON Schema, TOML defaults and
optional declarative uiSchema. Include all files in distribution.files. Every
field uses x-titleKey and optional x-descriptionKey backed by both locale files;
keep defaults only in TOML. The host renders the form and owns validation and
persistence. One settings.toml belongs to the extension across releases. Read the
next-start snapshot from context.settings; never write the managed file directly.
Game starting options use game.setup with the same declaration format, persist
with the instance, and appear in context.setup. Preview uses package defaults and
explicit test values, isolated from installed settings. Keep schemas compatible
with older releases still pinned by saves. Do not embed credentials in settings.
