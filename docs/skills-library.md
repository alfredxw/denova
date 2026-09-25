# Skills library

The Skills page opens the Library. Cards show the source, category, availability,
and remote update state. Search and filters work in both grid and list views;
opening a card retains the existing document preview and editor.
The secondary navigation stays visible, with a Library entry and the scoped
Skill list. Opening a card reveals and selects its entry in that list. Empty
groups are hidden while searching or filtering.

## Availability

- Denova's built-in, user, and Project Skills are grouped separately from the
  shared library at `~/.agents/skills`.
- Shared Skills are discovered for management but excluded from Agent discovery,
  slash commands, Skill loading, and reference reads until explicitly enabled.
  Shared directories may be symlinks; supporting-file reads stay inside the
  selected Skill. Denova never edits the shared library.
- Name precedence is Project, user, built-in, then shared. Disabling the winning
  Skill does not silently activate a lower-priority version. Agent profiles can
  further restrict availability, but cannot bypass the Library's disable switch.
- Global preferences are stored in `<data directory>/skills/.denova-library.json`;
  Project Skill availability is stored in `<project>/skills/.denova-library.json`.
  These contain scope/name keys, never absolute host paths. Preferences apply to
  Writing, Game, and other Skill-capable Agents.
- External engines receive the same host catalog and loader. Codex's automatic
  Skills instruction block is suppressed for Denova-owned processes so its
  ambient library cannot bypass these preferences. Denova does not rewrite the
  user's CLI Skills preferences.

## Import, export and remote updates

The Skills import picker uses the shared resource preview and install plan. It
accepts standard Skill repositories and ZIPs as well as mixed resource packages.
User and Project scopes remain explicit; shared and built-in Skills are never
modified in place. Individual Skills can be exported as a standard ZIP or added
to a mixed package.

Sources, content baselines and update preferences live in
`<data directory>/resource-exchange/installations/`. The Library links each
tracked Skill to its installation in Marketplace. That page provides manual
checks, reviewed updates, policy changes, detaching and backup recovery. It does
not replace the Skill editor or availability controls.

New installs default to manual updates. Notify checks daily without replacing
content. Automatic apply is available for packages containing only Skills,
presets and style references; it requires unchanged local files, the original
selection and dependencies, and idle consumers. A locally added file also counts
as a modification. Failed checks retain their attempt time, avoiding retry loops.
Mixed packages containing plugins, games or Project content require confirmation.

The installer freezes upstream bytes before confirmation and commits the whole
selected package with its source record. Before-images are retained in durable
transaction records. The market can download backups or review a restore plan;
it refuses to overwrite edits made since the original commit. See
[Marketplace and resource exchange](resource-pack-import-export-design.md) for the
format, ownership rules, recovery boundary and current scope.

On upgrade from v0.5.0, `.denova-source.json` is backed up and moved into the shared
installation model. An explicitly enabled auto-update preference is retained;
other Skills remain manual. Skill content is unchanged. Downgrading requires
restoring the old source files from migration before-images to recover source
and update information. Existing legacy `.denova-backups` are not removed.
