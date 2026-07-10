# Contributing to Denova

Thanks for helping improve Denova. This project is in beta, so contribution work should favor clear product improvements, simple implementation, and readable code over broad backwards compatibility.

## Setup

Denova requires Go 1.26.5+, Node.js 20+, and pnpm.

```bash
corepack enable
go mod tidy
./bootstrap.sh
```

Useful local commands:

```bash
./bootstrap.sh fe
./bootstrap.sh be
./bootstrap.sh fe --lan
./build.sh
```

If a frontend command is missing, try the project script first, then use `npx` when appropriate. If port `5173` is already occupied by the user's Vite process, do not kill or replace it; use the existing hot-reload page.

## Development Principles

- Keep the main writing and reading surfaces stable; controls belong in the shell layer unless the workflow needs otherwise.
- Keep Writing Mode and Interactive Mode separate. Shared menus must not switch modes automatically, and only one primary menu item should be active at a time.
- User-facing copy, empty states, errors, buttons, and settings should support both Chinese and English.
- Model-visible context must have explicit source, purpose, and hard size limits. Do not inject unbounded history, logs, sessions, directories, or full files by default.
- Prefer existing components, abstractions, and mature dependencies over custom implementations for common UI and editor behavior.
- Add configuration only when users reasonably need control over the new behavior. Define its default, scope, and persistence location.
- Protect workspace content. Automated migration, rename, delete, or overwrite behavior needs a reversible path or a clear backup.

## Code Style

- Simple and readable code wins.
- Keep packages and files focused. If a high-churn file approaches 500 lines, consider splitting by responsibility; avoid adding non-mechanical feature code to files over 800 lines.
- Do not add one-off helpers just to shorten code. Extract only when the name clarifies a concept, isolates complexity, matches an existing boundary, or enables useful reuse.
- Prefer private modules and narrow exported APIs.
- Avoid ambiguous booleans and unclear `nil` parameters at call sites. Use named config structs, enums, options, or explicit methods.
- Use early returns to reduce nesting.
- Handle finite states, event types, message types, and menu modes exhaustively instead of hiding future cases behind broad defaults.
- Recover goroutines so a panic does not crash the whole service.
- Log enough detail to debug failures, including what is happening and where.

## Testing and Validation

Choose validation based on the risk of the change:

- Agent main flow, tool calls, context trimming, version restore, session context, and workspace state changes should prefer integration tests.
- Pure functions, parsers, boundary conditions, and small state transitions are good unit-test targets.
- Frontend-visible changes should be verified in a browser, including narrow and wide layouts when layout is affected.
- If the frontend is already running, use the existing hot-reload page instead of restarting it.
- If backend behavior changes, restarting the backend is acceptable when needed.
- Run `./build.sh` before release or broad integration changes.

When fixing a bug, add a failing test first when practical. If automation is not practical, document the manual verification scope in the change summary.

## Documentation and Changelog

- Update `CHANGELOG.md` under `[Unreleased]` for user-facing changes, new features, repo-facing documentation, and notable fixes.
- Keep `README.md` and `README.en.md` semantically aligned when public capability or setup information changes.
- Release work must update the frontend version, `CHANGELOG.md`, `README.md`, `README.en.md`, and create the matching Git tag.

## Commit Messages

Use concise Conventional Commits-style messages:

```text
type(scope): short imperative summary
```

Common types:

- `feat`: user-facing feature or capability.
- `fix`: bug fix.
- `docs`: documentation-only change.
- `refactor`: behavior-preserving code restructure.
- `test`: test-only change.
- `chore`: tooling, dependency, release, or maintenance change.

Guidelines:

- Keep the subject under 72 characters when practical.
- Use an imperative summary, such as `fix(editor): prevent stale autosave overwrite`.
- Add a body when the change has behavior tradeoffs, migration risk, compatibility impact, or non-obvious validation.
- Mention breaking beta behavior explicitly in the body with `BREAKING CHANGE:` when users or stored workspace data must adapt.
- Keep one commit focused on one purpose.

## Pull Request Checklist

- The change is scoped to one clear purpose.
- User-facing behavior is covered in both Chinese and English where applicable.
- Model context changes have source and size limits.
- Workspace content is not silently destroyed or overwritten.
- Tests or manual validation match the risk of the change.
- `CHANGELOG.md` is updated when the change is notable.
