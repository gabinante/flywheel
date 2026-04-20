# Cursor-initiated OAuth for MCP (no token paste)

When you add an **HTTP/SSE MCP server by URL** (not stdio), Cursor can **start the sign-in flow itself**: if the server returns **401** with the right headers, Cursor discovers your auth server and opens a browser for the user to sign in. No copying tokens.

## How it works

1. **User adds Flywheel MCP in Cursor** with a **URL** (e.g. `https://flywheel.example.com/mcp` or `http://localhost:8080/mcp`), not a `command`.
2. **Cursor connects** to that URL (no token yet).
3. **Flywheel MCP server** responds with **401 Unauthorized** and:
   ```http
   WWW-Authenticate: Bearer resource_metadata="https://flywheel.example.com/.well-known/oauth-protected-resource"
   ```
4. **Cursor** fetches that `resource_metadata` URL and gets a **Protected Resource Metadata (PRM)** JSON (RFC 9728) with:
   - `resource` – the MCP endpoint URL
   - `authorization_servers` – URL of the auth server (us)
   - `scopes_supported` – e.g. `["flywheel:mcp"]`
5. **Cursor** fetches the **authorization server metadata** (RFC 8414) at e.g. `https://flywheel.example.com/.well-known/oauth-authorization-server` and gets:
   - `authorization_endpoint` – where to send the user (we redirect to GitHub)
   - `token_endpoint` – where Cursor exchanges the code for a token
   - Optional: `registration_endpoint` for Dynamic Client Registration
6. **Cursor** opens a **browser** to our `authorization_endpoint` (with `client_id`, `redirect_uri`, `state`, PKCE). We redirect to **GitHub**; user signs in; our callback creates user/agent, creates a short-lived **authorization code**, and redirects back to **Cursor's redirect_uri** with that code.
7. **Cursor** calls our **token_endpoint** with the code and gets **access_token** (our JWT). It then uses that as `Authorization: Bearer <token>` on all MCP requests.

So: **OAuth is used end-to-end**; Cursor drives the flow and stores the token. The user only "signs in with GitHub" in the browser.

## What we need to implement

| Piece | Purpose |
|-------|--------|
| **MCP over HTTP** | Expose the existing MCP server at a URL (SSE or Streamable HTTP via mark3labs/mcp-go). Run it in the same process as the REST server or a separate port. |
| **401 + WWW-Authenticate** | On the first MCP request without a valid `Authorization: Bearer`, return 401 with `resource_metadata` pointing to our PRM URL. |
| **GET /.well-known/oauth-protected-resource** | Return RFC 9728 PRM JSON: `resource` (MCP URL), `authorization_servers` (our base URL), `scopes_supported`. |
| **GET /.well-known/oauth-authorization-server** | Return RFC 8414 metadata: `issuer`, `authorization_endpoint`, `token_endpoint`, `scopes_supported`, and optionally `registration_endpoint`. |
| **GET /oauth/authorize** | Accept `client_id`, `redirect_uri`, `state`, `code_challenge`, `scope`. Redirect to GitHub OAuth; our callback URL will receive the GitHub code. |
| **GET /auth/github/callback** (extend) | After exchanging the GitHub code and provisioning user/agent: instead of showing the token page, redirect to **Cursor's redirect_uri** with `?code=ONE_TIME_CODE&state=...`. Store the one-time code with the agent_id and short TTL (e.g. 5 min). |
| **POST /oauth/token** | Accept `grant_type=authorization_code`, `code`, `redirect_uri`, `client_id`, `code_verifier`. Validate code, return `{ "access_token": "<jwt>", "token_type": "Bearer", "expires_in": 604800 }`. |

**Cursor as OAuth client**
Cursor will use PKCE and a redirect URI (e.g. `http://127.0.0.1:PORT/callback` or a Cursor-specific scheme). We either:

- Support **Dynamic Client Registration** (Cursor POSTs to `registration_endpoint` and gets `client_id`), or
- **Pre-register** Cursor's redirect URI(s) and use a fixed `client_id` (e.g. `cursor`) so we accept their redirects.

## Cursor config after this is built

User adds the server by **URL** (no `command`, no `FLYWHEEL_TOKEN`):

```json
{
  "mcpServers": {
    "flywheel": {
      "url": "https://flywheel.example.com/mcp"
    }
  }
}
```

Or for local dev:

```json
"url": "http://localhost:8080/mcp"
```

On first use, Cursor will prompt for sign-in and open the browser; after GitHub OAuth, Cursor stores the token and uses it automatically.

## Implemented

- **Discovery:** `GET /.well-known/oauth-protected-resource` and `GET /.well-known/oauth-authorization-server` return RFC 9728 / RFC 8414 metadata.
- **Authorize:** `GET /oauth/authorize?client_id=&redirect_uri=&state=&code_challenge=` stores state in Redis and redirects to GitHub.
- **Callback:** When GitHub redirects to `/auth/github/callback?code=&state=`, if `state` is found in Redis (from `/oauth/authorize`), we create a one-time code, redirect to the client's `redirect_uri?code=...&state=...`.
- **Token:** `POST /oauth/token` with `grant_type=authorization_code` and `code` exchanges the code for a JWT and returns `access_token`, `token_type`, `expires_in`.
- **MCP HTTP:** The server mounts the MCP Streamable HTTP handler at `/mcp`. Unauthenticated requests get `401` with `WWW-Authenticate: Bearer resource_metadata="<base>/.well-known/oauth-protected-resource"`. Authenticated requests (Bearer JWT or `X-API-Key`) have `agent_id` set in context; tools like `claim_ticket` and `start_ticket` use it when the client omits `agent_id`.

## References

- [MCP Authorization spec](https://modelcontextprotocol.io/specification/2025-11-25/basic/authorization) – 401, PRM, auth server discovery
- [RFC 9728](https://datatracker.ietf.org/doc/html/rfc9728) – OAuth 2.0 Protected Resource Metadata
- [RFC 8414](https://datatracker.ietf.org/doc/html/rfc8414) – Authorization Server Metadata
- [Cursor MCP docs](https://cursor.com/docs/mcp) – SSE/Streamable HTTP use OAuth; stdio uses manual auth
- [Truefoundry: MCP Authentication in Cursor](https://www.truefoundry.com/blog/mcp-authentication-in-cursor-oauth-api-keys-and-secure-configuration) – flow description
