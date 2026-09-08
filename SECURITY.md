# Security

Flywheel is a local, single-operator tool. The web UI and operator REST API trust the local machine; they do not have an internet-facing login or multi-user authorization boundary. The server binds to loopback and checks Host/Origin headers. Do not bypass those checks or expose it through a public reverse proxy or tunnel.

## Credentials and local data

- Keep real values out of tracked files. `.env.example` and `.env.schema` describe configuration; `.env`, MCP connection files, private keys, databases, logs, and transcripts are ignored.
- Initial setup writes `.env` with mode 0600 and generates a signing secret without printing it. Keep any existing secret files and database backups private as well.
- Operator settings in Postgres can contain a Linear API key and the worker MCP key. They are not encrypted at rest by Flywheel. Protect the database volume and backups with your operating system's access controls and disk encryption.
- Coding harnesses and `gh` use their own local authentication. A worker can act with those accounts' permissions, execute commands, and change checked-out repositories. Flywheel is not a sandbox for untrusted code.
- Local MCP keys authenticate harness clients; they are separate from GitHub, Linear, and model-provider credentials. A local-only key is not an internet service credential, but avoid committing new ones.
- Session imports, traces, screenshots, and error logs may contain private repository content even when they contain no access token. Inspect artifacts before sharing them.
- Compose's predictable Postgres credentials are development defaults. Postgres and Redis ports are bound to `127.0.0.1`; do not make them publicly reachable.

## Repository checks

Run `make secret-scan` with Gitleaks installed. It scans a temporary snapshot of tracked working-tree files and all locally available Git refs, with values redacted. Fetch remote branches and tags first if you need to include newly published history. CI repeats the check.

The narrow entries in `.gitleaksignore` document known historical local MCP credentials and a documentation placeholder. They refer to exact commits, files, rules, and lines; they do not exempt those files from future scanning. The local-only MCP findings are retained as accepted history, not presented as proof that history contains no credentials.

A clean automated scan is not a guarantee. Before making a repository public, also inspect remote branches, tags, releases, issue/PR attachments, Actions artifacts, and screenshots. Deleting a branch is not a guarantee that GitHub has removed every cached commit or pull-request ref.

If an external credential is exposed, revoke or rotate it first; deleting a file or rewriting history does not revoke a credential. Do not paste a suspected secret into a public issue or PR. Use GitHub's private vulnerability reporting if enabled, or contact the maintainer privately before sharing sensitive details.
