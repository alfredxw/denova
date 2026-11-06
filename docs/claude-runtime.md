# Claude Code runtime

Writing, General, and Game agents can select Claude Code in the Agents page. Defaults apply to new conversations; existing conversations and game branches retain their saved runtime selection until explicitly switched. Runtime switching requires an idle session with no pending work that blocks the switch. Other specialized agents retain their existing runtimes.

Install Claude Code **2.1.314 or newer**, authenticate locally with `claude auth login` when using CLI models, then check the connection in Agents. Denova discovers the executable on the host PATH or in `~/.local/bin`. On Windows it also resolves global npm installations directly to their native executable, or a legacy JavaScript entrypoint with Node, without executing a shell wrapper. Denova does not install the CLI or copy credentials into user projects.

CLI model choices and supported effort levels come from the installed runtime's initialize response, rather than a hard-coded alias list. Discovery does not start a model turn; failures are reported instead of substituting a static catalog. The selected model and effort are saved independently from Codex and Native settings; switching engines preserves inactive preferences. User configuration and per-session overrides use the existing settings and conversation configuration APIs.

## Denova API models (Codex and Claude Code)

Agents and the conversation model picker also offer compatible Denova API model profiles. Select an existing profile from Settings; CLI account login is unnecessary for this source, but the executable must still be installed. Codex requires an OpenAI Responses endpoint; Claude Code requires an Anthropic Messages endpoint. Chat Completions endpoints are not translated. A gateway must implement the selected protocol, streaming, and tool calling correctly; protocol selection alone does not guarantee model compatibility.

Only `profile_id` is saved in runtime preferences and session journals. Each operation resolves the profile's current model, endpoint, API key, and custom headers into an isolated connection. Updating a key affects subsequent operations; concurrent operations keep their own routing snapshot. Missing profiles, incompatible protocols, endpoint protocol options, and session-key mappings fail explicitly rather than falling back to another account. Profile sampling parameters, token budgets, and Native thinking settings are not transferred; the CLI owns those settings.

Claude receives process-local `ANTHROPIC_*` variables and the selected model argument, including auxiliary model aliases. Codex receives a process-local key/header environment and `-c` overrides for a dedicated Responses provider. Secret values never appear in Codex command-line arguments. Neither path edits CLI configuration files or the parent process environment. Native CLI-account selection retains its existing behavior.

## Execution and persistence

Each attempt owns its CLI process and authenticated loopback MCP endpoint using the official Go MCP SDK. A compatible host-local session cache resumes the native Claude session; a missing or invalid cache is rebuilt from the product journal. Structured JSON streaming carries the current request, text output, images, and usage. Rebuilt historical turns are quoted data, so importing history does not submit old requests again.

Claude retains its native system prompt, with the product instructions appended. Native Read, Glob, Grep, WebFetch, WebSearch, and Task tools handle general work. Writing and General also expose native AskUserQuestion and the user's configured MCP servers. Native Bash uses the runtime's enforced sandbox on macOS/Linux; it is not exposed on native Windows. Sandbox startup fails explicitly if the host cannot enforce it. Native Write/Edit, subagents, hooks, automatic memory, and ambient Skill discovery remain excluded. Denova's selected Skills and product plugins retain their existing opt-in rules.

Scoped Denova MCP tools handle managed edits, virtual resource reads, and domain operations. The native process runs in a disposable scratch directory; its sandbox denies writes to the product workspace and data directory. Native filesystem permission escalation is rejected. External integrations are trusted capabilities configured by the user and are not enclosed by the command sandbox; they must not modify managed product content. General filesystem search, shell, web, and browser tools are no longer duplicated by Denova. Claude's Task tools own Plan/Todo updates, and Denova saves confirmed snapshots.

Native questions and approval requests use the existing persistent interaction cards. Only an explicit allow option grants an approval; cancellation and free text do not. The stream continues draining while the user answers. Game exposes only the native read/research/plan tools and its scoped product tools, with no ambient MCP, native Shell, or user-approval waits. Its state changes still require `submit_interactive_turn`. Host tool calls retain Claude's `claudecode/toolUseId` identity and the existing transaction and recovery guarantees.

The product Session JSONL, or Story JSONL for Game, remains the canonical record. Native session IDs and private CLI files are disposable host caches, never a second recovery source. Cancellation terminates and reaps the owned process and closes its MCP endpoint. Private thinking blocks are not copied into product history. Native tool observations enter the same canonical journal and tool-card projection. Managed absolute roots are replaced with descriptive labels. Each public input/output preview is bounded to 64 KiB with an explicit truncation notice. Embedded native image bytes stay with the engine; after cache loss the journal tells the model to capture/read the source again. Images returned by product tools retain their existing durable attachment storage. An unfinished native action is recorded as having unknown effects and is never automatically replayed.

Queue, pause, resume, and steer use the shared application control layer. In Writing and General conversations, steering sends priority input to the live Claude process. Denova records the input after Claude acknowledges it and before any tools can act on it. When a project tool or native user interaction is active, or when steering a Game turn, Denova interrupts and resumes through the product journal to preserve the tool or draft state.

Writing and General conversations support Goal through an external, read-only evaluator. Game does not support Goal. Native subagent tools and their coordination instructions are excluded. See [AGENTS.md](../AGENTS.md) and [Agent recovery](agent-recovery.md) for ownership and recovery boundaries.

Denova imposes no total attempt or turn-count limit. Connection/authentication probes have a 15-second infrastructure timeout. MCP idle/background execution is disabled; upstream CLI/provider limits, including its hard MCP tool timeout, still apply. Oversized protocol frames or tool arguments fail explicitly rather than being silently truncated.

## Verification

The initial Windows acceptance used the real Claude Code 2.1.259 executable against an isolated local Anthropic-compatible fixture, without paid model requests or access to the user's credential home. It covered streamed text, image input, waiting for an answer, file tools, cancellation, Writing/General and their custom agents, and continuing the same conversation with Native. Journal tests also covered persisted answers after reopening.

To repeat the installed CLI integration checks in PowerShell:

```powershell
$env:DENOVA_TEST_CLAUDE_EXE = 'C:\absolute\path\to\claude.exe'
go test ./internal/agents/runtime/external/claude ./internal/app/conversation -count=1
```

The native-tool integration was verified on macOS arm64 with Claude Code 2.1.318 and Codex 0.144.6 against isolated local model fixtures. Coverage includes native questions, native reads and Shell execution, blocked direct workspace writes, product tool writes, cancellation, steering, and canonical observations. Native Windows and Linux behavior require their own host verification; this does not establish availability of any specific desktop/browser driver or account integration.

## Codex native capabilities

Codex uses App Server's native base instructions and general tools, with the product contract in developer instructions. The user's available browser/computer-use, Apps, plugins, and MCP integrations are no longer globally disabled for Writing/General. Availability still depends on the installed runtime, configured integration, and host driver; embedding App Server does not install the desktop app's private services.

Native commands use a read-only filesystem sandbox and the native foreground Shell. Unified Exec/PTY is excluded because supported App Server versions do not consistently publish its complete item lifecycle. Product tools continue enforcing the session's selected execution permissions independently. File-write escalation is denied; network approvals and MCP elicitations can use durable user interactions. Game and context maintenance disable ambient integrations and Shell. Product writing review, game submissions, canonical journals, and host-local resume caches keep their existing ownership.
