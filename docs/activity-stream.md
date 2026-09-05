# Live activity updates

Flywheel's UI uses one same-origin `EventSource` at `GET /api/activity/events`.
REST responses remain the source of truth. SSE carries invalidations, not domain
objects, transcript text, tool arguments, or private reasoning.

```text
retry: 2000

event: activity
data: {"topics":["reviews","runs"],"resync":false,"ready":true}

event: heartbeat
data: {}
```

The endpoint inherits the local operator's Host/Origin boundary. It is separate
from the authenticated MCP transports and the durable workflow event bus.

## Delivery and recovery

- Database triggers publish topic names through Postgres `NOTIFY` **after commit**.
  Rollbacks emit nothing. Timestamp-only/no-op updates are ignored. This covers
  REST, MCP, background jobs, and other writers without duplicating hooks in each
  service. See migration `20260905120000_activity_stream` for the table mapping.
- `runstatus` publishes in-memory lifecycle, output, and usage changes. Dispatch,
  review polling, and the session collector notify after changing runtime status.
  Runtime callbacks are wired in `cmd/server/main.go` before starting services.
- `internal/activity.Hub` batches notices every 500 ms. Each client has one pending
  event; missed topics are merged. Producers never wait for browser writes.
  Writes have a 10-second deadline and a heartbeat is sent every 15 seconds.
- Every browser connection gets `resync: true`. The Postgres listener also emits
  a resync after reconnecting. There is no event history or `Last-Event-ID` replay;
  clients recover by reading current snapshots, including after a server restart.
- `ready: false` means the database notification source is disconnected. The UI
  displays “Reconnecting” and reconciles every 15 seconds until it recovers.
  A silent browser connection is replaced after 45 seconds without a notice or
  heartbeat. Returning online, focusing, or reopening a hidden tab resyncs data.
- Connected clients reconcile once a minute as a backstop for time-derived state
  and failed REST reads. GitHub/Linear changes still arrive on their configured
  upstream polling/sync schedule; SSE delivers them when Flywheel observes them.

## Frontend usage

`ActivityProvider` lives above the routes. It invalidates affected React Query
keys through `queryAffected` in `web/src/lib/activity.ts`. Fetches are serialized
**per query**: a notice during a slow read schedules another read, and slow GitHub
requests do not block the tray. Inactive queries are marked stale for next use.

Existing effect-based views use a topic revision as a dependency:

```tsx
const activityVersion = useActivityVersion('sessions')
useEffect(() => {
  // Load the current REST snapshot; cancel or ignore superseded responses.
}, [client, sessionId, activityVersion])
```

This is wired into the global tray, review queue/details/conversations, My PRs,
session lists/details, project summaries, tickets, work streams, workflow library
and controls, execution traces, dispatch dashboard, schedule, and orchestrator.
The orchestrator UI shares the global connection; its older SSE endpoint remains
available for compatibility. Activity refreshes must preserve drafts and pending
actions. Do not attach a revision to an effect that initializes an editor form.

For new data, add a topic/table mapping (with a new migration), or wire a cheap
runtime notification after an in-memory change. Add the topic to the frontend
type and query map or view hook. Avoid adding another EventSource or page poller.

## Verification

`scripts/test-hardening.sh` builds the app and runs isolated database and
Playwright tests. Activity cases cover committed updates with polling disabled,
browser disconnection, database listener recovery, a notice arriving during a
slow REST read, and the real review queue/fake harness lifecycle. Unit/race tests
cover coalescing, bounded subscriber queues, unsubscribe/shutdown, heartbeat
recovery, and the HTTP boundary.
