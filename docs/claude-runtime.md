# Claude Code runtime

Writing and General agents, including their custom agents, can select Claude Code in the Agents page. Game and other specialized agents retain their existing runtimes. The conversation model picker changes the current session; applying an agent's engine configuration remains explicit.

Install Claude Code **2.1.259 or newer**, authenticate locally with `claude auth login` when using CLI models, then check the connection in Agents. Denova discovers the executable on the host PATH or in `~/.local/bin`. On Windows it also resolves global npm installations directly to their native executable, or a legacy JavaScript entrypoint with Node, without executing a shell wrapper. Denova does not install the CLI or copy credentials into user projects.

The CLI model choices are aliases: `default`, `sonnet`, `opus`, and `haiku`. Availability depends on the account/provider. The selected model and effort are saved independently from Codex and Native settings; switching engines preserves inactive preferences. User configuration and per-session overrides use the existing settings and conversation configuration APIs.

## Denova API models (Codex and Claude Code)

Agents and the conversation model picker also offer compatible Denova API model profiles. Select an existing profile from Settings; CLI account login is unnecessary for this source, but the executable must still be installed. Codex requires an OpenAI Responses endpoint; Claude Code requires an Anthropic Messages endpoint. Chat Completions endpoints are not translated. A gateway must implement the selected protocol, streaming, and tool calling correctly; protocol selection alone does not guarantee model compatibility.

Only `profile_id` is saved in runtime preferences and session journals. Each operation resolves the profile's current model, endpoint, API key, and custom headers into an isolated connection. Updating a key affects subsequent operations; concurrent operations keep their own routing snapshot. Missing profiles, incompatible protocols, endpoint protocol options, and session-key mappings fail explicitly rather than falling back to another account. Profile sampling parameters, token budgets, and Native thinking settings are not transferred; the CLI owns those settings.

Claude receives process-local `ANTHROPIC_*` variables and the selected model argument, including auxiliary model aliases. Codex receives a process-local key/header environment and `-c` overrides for a dedicated Responses provider. Secret values never appear in Codex command-line arguments. Neither path edits CLI configuration files or the parent process environment. Native CLI-account selection retains its existing behavior.

## Execution and persistence

Each attempt creates a fresh CLI process, disposable working directory, and authenticated loopback MCP endpoint using the official Go MCP SDK. Structured JSON streaming carries the current request, canonical history, text output, images, and usage. Historical turns are quoted data in one input turn, so importing history does not submit old requests again.

Only the scoped Denova MCP tools are exposed. Built-in CLI tools, ambient MCP servers, hooks, automatic memory, and CLI session persistence are disabled. Initialization is checked against the expected tool manifest. Tool calls go through the existing external Host, with Claude's `claudecode/toolUseId` as their stable identity, so committed effects and answers retain the existing transaction and recovery guarantees.

Product session JSONL remains the canonical record. A restart or engine switch reconstructs context from that journal; no Claude session ID or host-local launch path is stored as recovery state. Cancellation terminates and reaps the owned process and closes its MCP endpoint. Private thinking blocks are not copied into product history. Interactive permission prompts, subagents, goal, queue, steer, and pause are not advertised for this engine.

Denova imposes no total attempt or turn-count limit. Connection/authentication probes have a 15-second infrastructure timeout. MCP idle/background execution is disabled; upstream CLI/provider limits, including its hard MCP tool timeout, still apply. Oversized protocol frames or tool arguments fail explicitly rather than being silently truncated.

## Verification

Windows native verification uses the real Claude Code 2.1.259 executable against an isolated local Anthropic-compatible fixture, without paid model requests or access to the user's credential home. It covers streamed text, image input, waiting for an answer, file tools, cancellation, all four supported product/agent combinations, and continuing the same conversation with Native. Journal tests additionally cover persisted answers after reopening the session. Browser tests cover both languages, themes, viewport widths, and independent model preferences.

To repeat the installed CLI integration checks in PowerShell:

```powershell
$env:DENOVA_TEST_CLAUDE_EXE = 'C:\absolute\path\to\claude.exe'
go test ./internal/agents/external/claude ./internal/app/conversation -count=1
```

macOS/Linux execution and real account/provider behavior require separate host verification. The temporary CLI used in development is not a product installation.
