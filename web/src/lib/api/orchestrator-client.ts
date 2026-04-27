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
  kind: 'status' | 'worker_output' | 'error'
  payload: Record<string, unknown>
  created_at: string
}

export type OrchestratorRun = {
  id: string
  project_id: string
  user_message_id: string
  assistant_message_id?: string
  status: 'running' | 'completed' | 'failed'
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
  token: string | null,
  init?: RequestInit,
): Promise<{ data: T | null; error: string | null }> {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
  }
  if (token) {
    headers.Authorization = `Bearer ${token}`
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

export function getOrchestratorThread(token: string | null, projectId: string) {
  return orchestratorFetch<OrchestratorThread>(
    `/api/command-center/projects/${encodeURIComponent(projectId)}/orchestrator`,
    token,
  )
}

export function sendOrchestratorMessage(
  token: string | null,
  projectId: string,
  content: string,
) {
  return orchestratorFetch<OrchestratorThread>(
    `/api/command-center/projects/${encodeURIComponent(projectId)}/orchestrator/messages`,
    token,
    {
      method: 'POST',
      body: JSON.stringify({ content }),
    },
  )
}
