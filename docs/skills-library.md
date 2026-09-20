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

## Remote updates

Remote installs record their original URL, ref, subdirectory, candidate path, and
content digest in `.denova-source.json` inside the installed Skill. ZIP uploads
and existing installations without provenance are not guessed to be remote;
reinstall them from the original remote source to enable update tracking.

Each remote Skill has an auto-update switch, off by default. While Denova runs,
an application-owned worker checks opted-in Skills when their last attempt is at
least 24 hours old. Starting the app catches up overdue checks. Failed checks
also retain their attempt time, so a failed upstream does not create a retry loop.
No OS service or external scheduler is installed.

“Check all” only reports updates. “Update” and “Update all” install upstream
changes. Both manual and automatic installation skip Skills with locally changed
files or names. Updating one failed Skill does not roll back successful peers.

Before replacement, the whole previous directory is retained under
`<skill root>/.denova-backups/<skill name>/<timestamp>/`. To restore, close Denova,
preserve the current directory separately, and copy the desired backup back to
`<skill root>/<skill name>/`. A `pending` backup marks an interrupted replacement
and is recovered when the library is next opened. Backups are not automatically
deleted. Library metadata is excluded from editable files and content revisions.
