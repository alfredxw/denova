# Platform test fixtures

These minimal packages exercise the public manifest, settings, HTTP runtime,
Agent and storage contracts. They have no dependency on `templates/` and are
never embedded in the application. Tests copy `runtime/` and a package directory
into their own temporary Project before importing it through normal APIs.

The backend is a deterministic protocol stub, and the page is a storage probe.
Neither is a product example or a game to distribute.
