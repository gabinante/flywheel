# Contributing

Flywheel is a personal project under active development. Small, focused changes with a reproducible problem and relevant tests are easiest to review. Read [AGENTS.md](AGENTS.md) for the architecture and runtime conventions, and [README.md](README.md) for setup.

Use synthetic repositories, issues, and credentials in fixtures. Do not commit your MCP configuration, environment files, local session data, or screenshots of private work. Run `make secret-scan` before publishing a change.

For code changes, run the relevant Go tests and frontend lint/build. For user-facing changes, run the disposable Playwright suite. Explain what you verified and any remaining gaps; expected-failure tests do not demonstrate working functionality.

Start pull-request descriptions with:

```markdown
## Abstract
One to three sentences explaining the change and why it is useful.

**Components:** The systems or modules affected, in plain language.

**Before:** The observable behavior before the change.

**After:** The observable behavior after the change.
```

Keep security findings private until they can be handled safely; see [SECURITY.md](SECURITY.md). The existing [license](LICENSE) applies to contributions.
