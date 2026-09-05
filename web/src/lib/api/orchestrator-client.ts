export type OrchestratorMessage = {
  id: string
  project_id: string
  role: 'user' | 'assistant'
  content: string
  created_at: string
}

export type OrchestratorRunEvent = {
  id: string
  run_id: string
  kind: 'status' | 'worker_output' | 'error' | 'tool_call' | 'tool_result' | 'phase_change'
  payload: Record<string, unknown>
  created_at: string
}

export type OrchestratorPhase =
  | 'queued'
  | 'connecting'
  | 'investigating'
  | 'authoring'
  | 'composing'
  | 'complete'
  | 'failed'
  | 'cancelled'

export type OrchestratorRun = {
  id: string
  project_id: string
  user_message_id: string
  assistant_message_id?: string
  status: 'running' | 'completed' | 'failed' | 'cancelled'
  phase?: OrchestratorPhase
  worker_id?: string
  worker_name?: string
  runner?: string
  driver?: string
  model?: string
  error?: string
  started_at: string
  completed_at?: string
  events: OrchestratorRunEvent[]
}

export type OrchestratorWorkerLane = {
  name: string
  purpose: string
  default_style: string
}

export type OrchestratorPlaybook = {
  name: string
  summary: string
  principles: string[]
  ticket_sop: string[]
  worker_lanes: OrchestratorWorkerLane[]
  starter_prompts: string[]
}

export type OrchestratorThread = {
  project_id: string
  messages: OrchestratorMessage[]
  runs: OrchestratorRun[]
  playbook: OrchestratorPlaybook
}

async function orchestratorFetch<T>(
  path: string,
  init?: RequestInit,
): Promise<{ data: T | null; error: string | null }> {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
  }
  try {
    const res = await fetch(path, {
      ...init,
      headers: { ...headers, ...init?.headers },
    })
    if (!res.ok) {
      const body = await res.json().catch(() => null)
      const message = (body as { error?: string } | null)?.error ?? `HTTP ${res.status}`
      return { data: null, error: message }
    }
    const data = (await res.json()) as T
    return { data, error: null }
  } catch (error) {
    return { data: null, error: String(error) }
  }
}

export function getOrchestratorThread(projectId: string) {
  return orchestratorFetch<OrchestratorThread>(
    `/api/command-center/projects/${encodeURIComponent(projectId)}/orchestrator`,
  )
}

export function sendOrchestratorMessage(
  projectId: string,
  content: string,
) {
  return orchestratorFetch<OrchestratorThread>(
    `/api/command-center/projects/${encodeURIComponent(projectId)}/orchestrator/messages`,
    {
      method: 'POST',
      body: JSON.stringify({ content }),
    },
  )
}

export function cancelOrchestratorRun(
  projectId: string,
  runId: string,
) {
  return orchestratorFetch<{ status: string }>(
    `/api/command-center/projects/${encodeURIComponent(projectId)}/orchestrator/runs/${encodeURIComponent(runId)}`,
    { method: 'DELETE' },
  )
}

export function subscribeOrchestratorEvents(
  projectId: string,
  onEvent: (event: OrchestratorRunEvent) => void,
  onError?: () => void,
): { close: () => void } {
  const url = `/api/command-center/projects/${encodeURIComponent(projectId)}/orchestrator/events`
  const controller = new AbortController()
  let attempt = 0
  const maxAttempts = 5
  const maxDelay = 10_000

  async function connect() {
    while (attempt < maxAttempts && !controller.signal.aborted) {
      try {
        const headers: Record<string, string> = {
          Accept: 'text/event-stream',
        }
        const res = await fetch(url, {
          headers,
          signal: controller.signal,
        })
        if (!res.ok || !res.body) {
          throw new Error(`HTTP ${res.status}`)
        }
        attempt = 0 // reset on successful connection
        const reader = res.body.getReader()
        const decoder = new TextDecoder()
        let buffer = ''

        while (true) {
          const { done, value } = await reader.read()
          if (done) break
          buffer += decoder.decode(value, { stream: true })
          const lines = buffer.split('\n')
          buffer = lines.pop() ?? ''
          for (const line of lines) {
            if (line.startsWith('data: ')) {
              try {
                const event = JSON.parse(line.slice(6)) as OrchestratorRunEvent
                onEvent(event)
              } catch {
                // skip malformed JSON
              }
            }
          }
        }
        // Stream ended naturally (server closed) — reconnect
      } catch {
        if (controller.signal.aborted) return
        attempt++
        if (attempt >= maxAttempts) {
          onError?.()
          return
        }
        const delay = Math.min(1000 * 2 ** (attempt - 1), maxDelay)
        await new Promise((r) => setTimeout(r, delay))
      }
    }
  }

  void connect()

  return {
    close: () => controller.abort(),
  }
}
