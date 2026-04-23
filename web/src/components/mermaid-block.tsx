import mermaid from 'mermaid'
import { useEffect, useId, useRef, useState } from 'react'

let mermaidReady = false

function ensureMermaidInit() {
  if (mermaidReady) return
  mermaid.initialize({
    startOnLoad: false,
    securityLevel: 'strict',
    theme: 'neutral',
  })
  mermaidReady = true
}

type MermaidBlockProps = {
  chart: string
}

function MermaidBlockMounted({ chart }: { chart: string }) {
  const reactId = useId().replace(/:/g, '')
  const containerRef = useRef<HTMLDivElement>(null)
  const [err, setErr] = useState<string | null>(null)

  useEffect(() => {
    ensureMermaidInit()
    const el = containerRef.current
    if (!el) return
    el.replaceChildren()
    const renderId = `mmd-${reactId}-${Math.random().toString(36).slice(2, 9)}`
    let cancelled = false
    void mermaid
      .render(renderId, chart)
      .then(({ svg }) => {
        if (cancelled || !containerRef.current) return
        setErr(null)
        containerRef.current.innerHTML = svg
      })
      .catch((e: unknown) => {
        if (cancelled) return
        setErr(e instanceof Error ? e.message : String(e))
      })
    return () => {
      cancelled = true
    }
  }, [chart, reactId])

  if (err) {
    return (
      <div className="my-2 rounded-xl border border-white/10 bg-white/[0.03] p-2 text-xs backdrop-blur-sm">
        <p className="text-destructive mb-1 font-medium">Mermaid</p>
        <p className="text-muted-foreground mb-2">{err}</p>
        <pre className="text-foreground overflow-x-auto font-mono whitespace-pre-wrap">
          {chart}
        </pre>
      </div>
    )
  }

  return (
    <div
      ref={containerRef}
      className="my-2 overflow-x-auto rounded-xl border border-white/10 bg-white/[0.03] p-2 backdrop-blur-sm [&_svg]:max-w-none"
      aria-label="Mermaid diagram"
    />
  )
}

/** Renders a single ```mermaid fenced block; invalid syntax shows source + error. */
export function MermaidBlock({ chart }: MermaidBlockProps) {
  const instanceId = useId()
  const trimmed = chart.trim()
  if (!trimmed) {
    return (
      <p className="text-destructive text-xs">Empty mermaid block.</p>
    )
  }
  return (
    <MermaidBlockMounted key={`${instanceId}-${trimmed}`} chart={trimmed} />
  )
}
