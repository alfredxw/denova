# Develop this extension

This Project is the source workspace. Develop it in Workbench using its Agent,
files, terminal, source manifest and testing actions. Extensions manages installed
releases and their consumer settings. Use the built-in extension-development
Skill for implementation and platform API guidance.

The root manifest selects a plugin or game. Keep its ID stable when renaming the
Project. Update the version before installing changed release bytes. Both zh-CN
and en-US display names are required when checking a package; the template uses
your initial Project name for both until you translate it.

Use Testing to build, check, preview, export, or install a frozen package.
Build and test feedback is attached to subsequent Agent turns. Preview games
have separate test saves. Source edits take effect after
checking and opening a new preview; installed games and existing saves keep
their selected release. Close a preview before resetting its test save.

Static templates need no build. A Node backend requires Node.js on the host.
If a build is needed, declare development.build.command and development.build.args
in the manifest. Keep distribution.files restricted to runtime assets; do not
include credentials, generated caches, or node_modules.

Game pages connect through client.mjs and receive a scoped HTTP connection.
Persistent game data uses revision-checked file writes or the game's own backend.
NPC definitions stay private to this game. Plugins contribute executable tools
and toolsets; user Skills and Agent profiles are managed in their existing pages.

The detective-game template includes game.mjs, stream.mjs, style.css, two private characters,
and independent locale resources. It demonstrates session creation, paged history,
model runs, SSE reply deltas, cancellation, and revision-checked request recovery.
Its independent HTML/CSS scene owns evidence collection and the delivery ending;
model output cannot grant evidence or change the deterministic case state.
Each contact has a stable logical session key; the platform journal owns its chat
history. messages.json stores the version, evidence, ending and pending command, never a second
chat transcript or model credentials. Save exact input and commandId before POST;
reuse that ID after reconnecting. Add characters in the manifest, game.mjs and both
locale files together. Choose a new session key when changing a character definition.
The opening model is selected by the player through the local:writer slot. Input
is capped at 8000 UTF-16 code units by the composer; older history loads in pages
of 50 records. There is no total model runtime limit. Keep cancellation available.

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
