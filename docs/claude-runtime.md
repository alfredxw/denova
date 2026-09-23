# Claude Code runtime

Writing, General, and Game agents can select Claude Code in the Agents page. Defaults apply to new conversations; existing conversations and game branches retain their saved runtime selection until explicitly switched. Runtime switching requires an idle session with no pending work that blocks the switch. Other specialized agents retain their existing runtimes.

Install Claude Code **2.1.259 or newer**, authenticate locally with `claude auth login` when using CLI models, then check the connection in Agents. Denova discovers the executable on the host PATH or in `~/.local/bin`. On Windows it also resolves global npm installations directly to their native executable, or a legacy JavaScript entrypoint with Node, without executing a shell wrapper. Denova does not install the CLI or copy credentials into user projects.

CLI model choices and supported effort levels come from the installed runtime's initialize response, rather than a hard-coded alias list. Discovery does not start a model turn; failures are reported instead of substituting a static catalog. The selected model and effort are saved independently from Codex and Native settings; switching engines preserves inactive preferences. User configuration and per-session overrides use the existing settings and conversation configuration APIs.

## Denova API models (Codex and Claude Code)

Agents and the conversation model picker also offer compatible Denova API model profiles. Select an existing profile from Settings; CLI account login is unnecessary for this source, but the executable must still be installed. Codex requires an OpenAI Responses endpoint; Claude Code requires an Anthropic Messages endpoint. Chat Completions endpoints are not translated. A gateway must implement the selected protocol, streaming, and tool calling correctly; protocol selection alone does not guarantee model compatibility.

Only `profile_id` is saved in runtime preferences and session journals. Each operation resolves the profile's current model, endpoint, API key, and custom headers into an isolated connection. Updating a key affects subsequent operations; concurrent operations keep their own routing snapshot. Missing profiles, incompatible protocols, endpoint protocol options, and session-key mappings fail explicitly rather than falling back to another account. Profile sampling parameters, token budgets, and Native thinking settings are not transferred; the CLI owns those settings.

Claude receives process-local `ANTHROPIC_*` variables and the selected model argument, including auxiliary model aliases. Codex receives a process-local key/header environment and `-c` overrides for a dedicated Responses provider. Secret values never appear in Codex command-line arguments. Neither path edits CLI configuration files or the parent process environment. Native CLI-account selection retains its existing behavior.

## Execution and persistence

Each attempt owns its CLI process and authenticated loopback MCP endpoint using the official Go MCP SDK. A compatible host-local session cache resumes the native Claude session; a missing or invalid cache is rebuilt from the product journal. Structured JSON streaming carries the current request, text output, images, and usage. Rebuilt historical turns are quoted data, so importing history does not submit old requests again.

Scoped Denova MCP tools handle product operations. Claude's native Task tools own Plan/Todo updates; Denova saves confirmed snapshots for display instead of injecting another writable Todo tool. Other built-in tools, ambient MCP servers, hooks, and automatic memory are disabled. Initialization is checked against the expected tool manifest. Host tool calls use Claude's `claudecode/toolUseId` as their stable identity, so committed effects and answers retain the existing transaction and recovery guarantees.

The product Session JSONL, or Story JSONL for Game, remains the canonical record. Native session IDs and private CLI files are disposable host caches, never a second recovery source. Cancellation terminates and reaps the owned process and closes its MCP endpoint. Private thinking blocks are not copied into product history.

Queue, pause, resume, and steer use the shared application control layer; Claude steering uses confirmed native interrupt followed by resume. Writing and General conversations support Goal through an external, read-only evaluator; Game does not support Goal. Native subagent tools and their coordination instructions are excluded. See [runtime session boundaries](agent-runtime-session-design-audit.md) for ownership and recovery details.

Denova imposes no total attempt or turn-count limit. Connection/authentication probes have a 15-second infrastructure timeout. MCP idle/background execution is disabled; upstream CLI/provider limits, including its hard MCP tool timeout, still apply. Oversized protocol frames or tool arguments fail explicitly rather than being silently truncated.

## Verification

The initial Windows acceptance used the real Claude Code 2.1.259 executable against an isolated local Anthropic-compatible fixture, without paid model requests or access to the user's credential home. It covered streamed text, image input, waiting for an answer, file tools, cancellation, Writing/General and their custom agents, and continuing the same conversation with Native. Journal tests also covered persisted answers after reopening. Later session, model-discovery, and Game verification is recorded in the [runtime acceptance notes](agent-runtime-session-design-audit.md); the initial acceptance is not a claim that every current capability was tested with the real CLI.

To repeat the installed CLI integration checks in PowerShell:

```powershell
$env:DENOVA_TEST_CLAUDE_EXE = 'C:\absolute\path\to\claude.exe'
go test ./internal/agents/runtime/external/claude ./internal/app/conversation -count=1
```

macOS/Linux execution and real account/provider behavior require separate host verification. The temporary CLI used in development is not a product installation.
