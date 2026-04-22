import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

function isPlainObject(v: unknown): v is Record<string, unknown> {
  return v !== null && typeof v === 'object' && !Array.isArray(v)
}

function formatArtifactItem(item: unknown): string {
  if (typeof item === 'string') return item
  try {
    return JSON.stringify(item, null, 2)
  } catch {
    return String(item)
  }
}

type TicketOutputsCardProps = {
  /** API `outputs` — typed loosely at runtime (OpenAPI uses an empty object schema). */
  outputs: unknown
}

export function TicketOutputsCard({ outputs }: TicketOutputsCardProps) {
  if (!isPlainObject(outputs)) return null
  const keys = Object.keys(outputs)
  if (keys.length === 0) return null

  const summaryVal = outputs.summary
  const artifactsVal = outputs.artifacts

  const summaryText =
    typeof summaryVal === 'string' && summaryVal.trim() !== ''
      ? summaryVal.trim()
      : null

  const artifactsItems = Array.isArray(artifactsVal) ? artifactsVal : null

  const otherEntries = Object.entries(outputs).filter(([k, v]) => {
    if (k === 'summary') {
      return !(typeof v === 'string' && v.trim() !== '')
    }
    if (k === 'artifacts') {
      if (Array.isArray(v)) return false
      if (v !== undefined && v !== null && !Array.isArray(v)) return false
      return false
    }
    return true
  })

  const hasArtifactsFallback =
    artifactsVal !== undefined &&
    artifactsVal !== null &&
    !Array.isArray(artifactsVal)

  const hasVisibleBody =
    !!summaryText ||
    !!(artifactsItems && artifactsItems.length > 0) ||
    hasArtifactsFallback ||
    otherEntries.length > 0

  if (!hasVisibleBody) return null

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-sm">Outputs</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {summaryText ? (
          <div>
            <h3 className="text-foreground mb-1.5 text-xs font-medium tracking-wide uppercase">
              Summary
            </h3>
            <p className="text-muted-foreground whitespace-pre-wrap text-sm">
              {summaryText}
            </p>
          </div>
        ) : null}

        {artifactsItems && artifactsItems.length > 0 ? (
          <div>
            <h3 className="text-foreground mb-1.5 text-xs font-medium tracking-wide uppercase">
              Artifacts
            </h3>
            <ul className="flex flex-col gap-1 rounded-xl border border-white/10 bg-white/[0.03] px-3 py-2 backdrop-blur-sm">
              {artifactsItems.map((item, i) => (
                <li
                  key={i}
                  className="text-foreground font-mono text-xs break-all"
                >
                  {formatArtifactItem(item)}
                </li>
              ))}
            </ul>
          </div>
        ) : null}

        {hasArtifactsFallback ? (
          <div>
            <h3 className="text-foreground mb-1.5 text-xs font-medium tracking-wide uppercase">
              Artifacts
            </h3>
            <pre className="max-h-48 overflow-auto rounded-xl border border-white/10 bg-white/[0.03] p-3 font-mono text-xs backdrop-blur-sm">
              {typeof artifactsVal === 'string'
                ? artifactsVal
                : JSON.stringify(artifactsVal, null, 2)}
            </pre>
          </div>
        ) : null}

        {otherEntries.length > 0 ? (
          <div>
            <h3 className="text-foreground mb-1.5 text-xs font-medium tracking-wide uppercase">
              Other
            </h3>
            <dl className="flex flex-col gap-2 text-sm">
              {otherEntries.map(([key, val]) => (
                <div key={key}>
                  <dt className="text-muted-foreground font-mono text-xs">
                    {key}
                  </dt>
                  <dd className="mt-0.5">
                    {typeof val === 'string' || typeof val === 'number' ? (
                      <span className="text-foreground whitespace-pre-wrap">
                        {String(val)}
                      </span>
                    ) : (
                      <pre className="mt-1 max-h-40 overflow-auto rounded-xl border border-white/10 bg-white/[0.03] p-2 font-mono text-xs backdrop-blur-sm">
                        {JSON.stringify(val, null, 2)}
                      </pre>
                    )}
                  </dd>
                </div>
              ))}
            </dl>
          </div>
        ) : null}
      </CardContent>
    </Card>
  )
}
