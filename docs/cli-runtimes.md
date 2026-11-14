# Grok Build and Antigravity CLI runtimes

Writing, General Chat, and Game can select `grok` or `antigravity` as an execution
engine in Settings → Agents. A terminal shortcut only launches a terminal;
runtime selection has its own model catalog, connection check, and saved settings.
Existing conversations keep their recorded engine until explicitly switched.

## Discovery and model selection

The supported baselines are Grok Build 1.0.62 and Antigravity CLI 1.4.2
(1.4.4 on Windows).

Denova discovers native `grok` and `agy` executables from the host environment.
It also checks Grok's `GROK_BIN_DIR` and `~/.grok/bin`, Antigravity's
`~/.local/bin/agy`, and Windows `%LOCALAPPDATA%\agy\bin\agy.exe` installation.
The Antigravity IDE launcher is not the standalone `agy` CLI.

The adapters use the CLI's existing local account or configured API credentials.
Denova does not save credentials in conversation selections or journals. The new
engines use CLI models, without Denova API profile overrides. Each engine retains
its own inactive settings when another engine is selected.

Grok models come from the CLI's own `grok models` text catalog, resolved on the
headless channel's chat proxy catalog. Inert model routing settings from the
user's Grok configuration, including supported effort values, are copied into a
private execution cache; hooks, plugins, and unrelated MCP servers are excluded.
Antigravity models come from the documented `agy models` text catalog.
Its model slugs already identify the available variants; Denova does not invent
a separate effort list.

## Protocols and product tools

Each Grok turn runs as a single headless process launched with `--prompt-file`
and `--output-format streaming-json`. The CLI reports an NDJSON event stream
(`text`, `plan`, `usage`, `tool_call`, `error`, `end`) rather than an ACP
session; image turns use a `.json` prompt file of ACP content blocks, a borrowed
block format, not the ACP protocol. Product tools are reached through the
`use_tool` built-in over a per-attempt loopback MCP server that executes on the
host. Antigravity uses its own `--input-format stream-json` and
`--output-format stream-json` protocol: `init`, `step_update`, and `result`.
Antigravity does not accept another CLI's control-request messages.

Both adapters expose scoped Denova tools through the existing Host boundary.
Product mutations, questions, and tool receipts remain in the canonical product
journal. CLI transcripts are disposable caches. An accepted provider session is
reused only when its canonical boundary and instruction/tool prefix still align.
Cancellation interrupts the provider process, and transport replays reuse the
settled tool result instead of executing the same delivery again.

Antigravity uses a uniquely named temporary definition in its global custom-agent
directory with `inheritCustomizations: false` and `commandExecutionPolicy: off`.
`excludeDefaultComponents` stays unset on purpose: headless stream-json input
accepts text blocks only, so the "@name" reference channel depends on the default
file-reading component, and the declaration would disable it. A per-attempt
authenticated loopback MCP server exposes the scoped host tools. The temporary
definition is removed after the process and tool callbacks settle. Existing agent
definitions are preserved. Session-scoped `--settings` force request-review
permission (allowing only `mcp(denova/*)` when host tools exist, and denying
`write_file(*)`, `command(*)`, `execute_url(*)`, `read_url(*)`, plus `mcp(*)`
when no host tools exist), with `--sandbox` as defense-in-depth for the native
layer. Active global hooks that cannot be isolated
produce a localized connection error; Denova does not rewrite the user's
CLI configuration. Headless execution uses `--print-timeout 0` so there is no
fixed model turn deadline.

## Capability boundaries

Both engines support text conversations, scoped product tools, cancellation, and
queued input. Antigravity additionally supports pause and steering: pause suspends
the operation for later resume, and steering interrupts the running attempt while
retaining the accepted guidance and drafts for reconstruction on a later attempt.
Grok runs each turn as a single headless process with no live channel into the
running turn, so it can be cancelled or queued but neither paused nor steered.

Images travel through the text protocols. Grok encodes attachments as ACP content
blocks in the `.json` prompt file, including historical images collected in
record order. Antigravity stages request and historical images as short "@name"
reference files in the attempt working directory and replays them next to their
records, so an attachment-bearing input or history is supported instead of losing
image content; non-image historical media stays inside the reconstructed text
records.

Goal and manual native compaction are unavailable for these engines. Goal
evaluation requires an isolated provider-session fork, which these adapters do
not expose. Game never supports Goal, regardless of engine.

## Protocol evidence and tests

- [Grok headless and ACP documentation](https://docs.x.ai/build/cli/headless-scripting)
- [Grok ACP reference source](https://github.com/xai-org/grok-build/tree/2bdd1d6a6369de0e8c68132ea4539e9abd9e14a8/crates)
- [Antigravity headless protocol](https://www.antigravity.google/docs/cli/headless/)
- [Antigravity custom agents](https://www.antigravity.google/docs/subagents)
- [Antigravity customization release notes](https://www.antigravity.google/docs/changelog?tab=cli)

Adapter tests use independent process fixtures for each protocol. They cover
streaming events, scoped Host callbacks, duplicate delivery, failure, session
resume, and cancellation. Installed-binary checks are opt-in and report exactly
which handshakes or local fixture round trips ran; an unauthenticated handshake
does not establish a successful paid-account model invocation.

The released Grok executable (opt-in via `DENOVA_TEST_GROK`) is also exercised
against a local model fixture for headless streaming turns, including an image
turn whose picture travels inside the `.json` prompt file, plus private
configuration and session resume through the `-r` flag. Antigravity's opt-in
installed check (via `DENOVA_TEST_ANTIGRAVITY_EXE`) validates native custom-agent
discovery and the model-list command's unauthenticated path; a signed-in
model/tool round trip requires account access.
