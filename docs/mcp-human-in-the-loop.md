# MCP human-in-the-loop: Notifications, Sampling, Elicitation

Flywheel’s MCP server can use standard MCP and MCP-Go features so the **agent asks the human** through the client (e.g. Cursor) instead of the agent only describing REST steps. This keeps “clear this for me” and similar requests inside the agent ↔ human loop.

## Three mechanisms

### 1. Notifications (MCP-Go)

**What:** Server sends one-way messages to the client (e.g. progress, alerts).  
**Docs:** [MCP-Go Advanced – Custom notifications](https://mcp-go.dev/servers/advanced#custom-notifications)

**Use in Flywheel:** When the agent is blocked (e.g. can’t submit a ticket because the lease is held elsewhere), the tool handler can get the server from context and send a notification so the client can surface it:

```go
srv := server.ServerFromContext(ctx)
srv.SendNotificationToAllClients("flywheel/need_human", map[string]interface{}{
    "message": "Please release the lease on ticket agent-reliability-2 so I can claim and submit it.",
    "ticket_id": "agent-reliability-2",
    "hint": "See docs/troubleshooting.md → Tickets: agent stuck or wrong state",
})
```

**Pros:** Simple, non-blocking. **Cons:** No structured response; the agent doesn’t get a “done” or “declined” back in the same tool call.

---

### 2. Sampling (MCP-Go)

**What:** Server requests an LLM completion from the client. The client is expected to do human-in-the-loop (user reviews/approves the request, can edit prompts).  
**Docs:** [MCP-Go Server sampling](https://mcp-go.dev/servers/advanced#sampling-advanced), [advanced-sampling](https://mcp-go.dev/servers/advanced-sampling), [Client sampling](https://mcp-go.dev/clients/advanced-sampling)

**Use in Flywheel:** A tool could call `RequestSampling(ctx, request)` with a “question” that is really for the human, e.g. “I need you to release the lease on ticket agent-reliability-2 (see docs/troubleshooting.md). Reply with ‘done’ when you have, or ‘no’ to skip.” The client shows this to the user; the user’s (or LLM’s) reply is returned to the server so the tool can branch (retry claim, or give up).

**Pros:** Bidirectional; server gets a text response. **Cons:** Requires enabling sampling and client support; semantics are “ask LLM” so UX depends on how the client maps that to “ask user.”

---

### 3. Elicitation (MCP spec)

**What:** Server sends an **elicitation request** to ask the user for structured data. The client shows the request; the user can **accept** (with data), **decline**, or **cancel**. Response goes back to the server.  
**Spec:** [MCP Elicitation](https://modelcontextprotocol.io/docs/concepts/elicitation) (form mode: JSON schema; URL mode: redirect for sensitive flows).  
**MCP-Go:** Client capabilities can include elicitation; servers can check and send elicitation requests (see [PR #491](https://github.com/mark3labs/mcp-go/pull/491)).

**Use in Flywheel:** When a tool is blocked (e.g. “ticket X is executing, I don’t have the lease”), the server sends an elicitation request: “Please release the lease on ticket `agent-reliability-2`. When done, confirm below.” with a response schema e.g. `{ "confirmed": boolean, "notes": string (optional) }`. The client shows this; the user confirms or declines; the tool receives the result and continues (e.g. try `claim_ticket` again) or stops.

**Pros:** Purpose-built for “server asks user for input”; structured response; accept/decline/cancel. **Cons:** Requires client to support elicitation and Wire/server support in mcp-go if not already present.

---

## Recommendation

- **Elicitation** is the best fit for “agent needs human to do something and then continue”: clear request/response, optional structured data, and explicit accept/decline/cancel.
- **Notifications** are useful for passive “please look at this” or “agent is blocked; see …” without blocking the tool.
- **Sampling** can be used if the client treats it as “ask the user” and returns their reply; implementation and UX are heavier than elicitation for this use case.

## Next steps

1. **Migration done:** Flywheel now uses [github.com/modelcontextprotocol/go-sdk](https://github.com/modelcontextprotocol/go-sdk) (Streamable HTTP, stdio, tools, resources). mark3labs/mcp-go has been removed.
2. **Elicitation (done):** When claim_ticket returns no ticket available, the server sends an elicitation that lists stuck tickets (claimed or executing) and lets the user choose one to force-release (ticket_id_to_release). On accept, the server force-releases that ticket and retries claim so the agent gets the lease in the same session. The user can also direct the agent in chat (e.g. "release agent-reliability-3 and claim it"); the agent then calls force_release_lease (ticket_id) and claim_ticket (project_id). See docs/troubleshooting.md.
3. **Notifications (optional):** If the SDK exposes a way to send notifications to the client, use them for passive “agent is blocked; see …” without blocking the tool.

This doc should be updated as we implement; link to it from **docs/cursor-mcp.md** and the agent guide once elicitation or notifications are in use.
