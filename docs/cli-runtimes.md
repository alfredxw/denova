# Grok Build and Antigravity CLI runtimes

Writing, General Chat, and Game can select `grok` or `antigravity` as an execution
engine in Settings → Agents. A terminal shortcut only launches a terminal;
runtime selection has its own model catalog, connection check, and saved settings.
Existing conversations keep their recorded engine until explicitly switched.

## Discovery and model selection

The supported baselines are Grok Build 1.0.62 and Antigravity CLI 1.4.2.

Denova discovers native `grok` and `agy` executables from the host environment.
It also checks Grok's `GROK_BIN_DIR` and `~/.grok/bin`, Antigravity's
`~/.local/bin/agy`, and Windows `%LOCALAPPDATA%\agy\bin\agy.exe` installation.
The Antigravity IDE launcher is not the standalone `agy` CLI.

The adapters use the CLI's existing local account or configured API credentials.
Denova does not save credentials in conversation selections or journals. The new
engines use CLI models, without Denova API profile overrides. Each engine retains
its own inactive settings when another engine is selected.

Grok models come from its native ACP model catalog, including supported effort
values. Inert model routing settings from the user's Grok configuration are copied
into a private execution cache; hooks, plugins, and unrelated MCP servers are
excluded. Antigravity models come from the documented `agy models` text catalog.
Its model slugs already identify the available variants; Denova does not invent
a separate effort list.

## Protocols and product tools

Grok Build uses JSON-RPC ACP over `grok agent --no-leader stdio`. Its extensions
use `_x.ai/...` wire method names. Host tools use Grok's native MCP-over-ACP
reverse requests. Antigravity uses its own `--input-format stream-json` and
`--output-format stream-json` protocol: `init`, `step_update`, and `result`.
Antigravity does not accept another CLI's control-request messages.

Both adapters expose scoped Denova tools through the existing Host boundary.
Product mutations, questions, and tool receipts remain in the canonical product
journal. CLI transcripts are disposable caches. An accepted provider session is
reused only when its canonical boundary and instruction/tool prefix still align.
Cancellation interrupts the provider process, and transport replays reuse the
settled tool result instead of executing the same delivery again.

Antigravity uses a uniquely named temporary definition in its global custom-agent
directory, with `excludeDefaultComponents: true` and `inheritCustomizations: false`,
and a per-attempt authenticated loopback MCP server. The temporary definition is
removed after the process and tool callbacks settle. Existing agent definitions
are preserved. Its initial tool list must match the scoped dispatcher. Active global
hooks that cannot be isolated produce a localized connection error; Denova does
not rewrite the user's CLI configuration. Headless execution uses
`--print-timeout 0` so there is no fixed model turn deadline.

## Capability boundaries

Both engines support text conversations, scoped product tools, cancellation,
queued input, and host-managed pause/recovery. Steering interrupts the attempt
and retains the accepted guidance for reconstruction. Grok supports ACP image
content; Antigravity's stream input accepts text only, so attachment-bearing
input or history returns an explicit error instead of losing image content.

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
streaming, scoped Host callbacks, duplicate delivery, failure, session reuse,
and cancellation. Installed-binary checks are opt-in and report exactly which
handshakes or local fixture round trips ran; an unauthenticated handshake does
not establish a successful paid-account model invocation.

The released Grok executable is also exercised against a local model fixture for
ACP session creation, real Host tool/result round trips, and session loading.
Antigravity's installed-binary check validates its native agent discovery and
model-list command; a signed-in model/tool round trip requires account access.
