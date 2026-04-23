import { useState } from 'react'
import { Prism as SyntaxHighlighter } from 'react-syntax-highlighter'
import { vscDarkPlus } from 'react-syntax-highlighter/dist/esm/styles/prism'
import {
  ChevronDown,
  ChevronRight,
  Code2,
  ExternalLink,
  FileText,
} from 'lucide-react'

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { cn } from '@/lib/utils'

function isPlainObject(v: unknown): v is Record<string, unknown> {
  return v !== null && typeof v === 'object' && !Array.isArray(v)
}

/** Detect if a string looks like a URL. */
function isUrl(s: string): boolean {
  return /^https?:\/\/\S+/.test(s.trim())
}

/** Detect if a string looks like JSON. */
function looksLikeJson(s: string): boolean {
  const trimmed = s.trim()
  return (
    (trimmed.startsWith('{') && trimmed.endsWith('}')) ||
    (trimmed.startsWith('[') && trimmed.endsWith(']'))
  )
}

function SyntaxBlock({ content, language }: { content: string; language: string }) {
  return (
    <div className="overflow-hidden rounded-lg border border-white/[0.06]">
      <SyntaxHighlighter
        language={language}
        style={vscDarkPlus}
        PreTag="div"
        customStyle={{
          margin: 0,
          borderRadius: 0,
          fontSize: '0.75rem',
          lineHeight: 1.6,
          background: 'rgba(255,255,255,0.02)',
          padding: '0.75rem 1rem',
        }}
        codeTagProps={{
          className: 'font-mono',
        }}
      >
        {content}
      </SyntaxHighlighter>
    </div>
  )
}

function CollapsibleOutput({
  label,
  children,
  defaultOpen = true,
}: {
  label: string
  children: React.ReactNode
  defaultOpen?: boolean
}) {
  const [open, setOpen] = useState(defaultOpen)

  return (
    <div className="flex flex-col gap-2">
      <button
        type="button"
        className="flex items-center gap-1.5 text-left transition-colors hover:text-foreground"
        onClick={() => setOpen(!open)}
      >
        {open ? (
          <ChevronDown className="text-muted-foreground size-3" />
        ) : (
          <ChevronRight className="text-muted-foreground size-3" />
        )}
        <h3 className="text-foreground text-xs font-medium tracking-wide uppercase">
          {label}
        </h3>
      </button>
      {open ? children : null}
    </div>
  )
}

function OutputValue({ value }: { value: unknown }) {
  if (typeof value === 'string') {
    const trimmed = value.trim()

    // URL detection
    if (isUrl(trimmed)) {
      return (
        <a
          href={trimmed}
          target="_blank"
          rel="noopener noreferrer"
          className="inline-flex items-center gap-1 text-sm text-blue-400 hover:text-blue-300 transition-colors"
        >
          {trimmed}
          <ExternalLink className="size-3" />
        </a>
      )
    }

    // JSON detection
    if (looksLikeJson(trimmed)) {
      let formattedJson: string | null = null
      try {
        formattedJson = JSON.stringify(JSON.parse(trimmed), null, 2)
      } catch {
        // Not valid JSON, fall through
      }
      if (formattedJson) {
        return <SyntaxBlock content={formattedJson} language="json" />
      }
    }

    // Multi-line code block
    if (trimmed.includes('\n') && (trimmed.includes('  ') || trimmed.includes('\t'))) {
      return <SyntaxBlock content={trimmed} language="plaintext" />
    }

    // Plain text
    return (
      <p className="text-muted-foreground whitespace-pre-wrap text-sm leading-relaxed">
        {trimmed}
      </p>
    )
  }

  if (typeof value === 'number' || typeof value === 'boolean') {
    return (
      <span className="font-mono text-sm text-emerald-400">{String(value)}</span>
    )
  }

  // Objects and arrays
  const formatted = JSON.stringify(value, null, 2)
  return <SyntaxBlock content={formatted} language="json" />
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
  const prUrl = outputs.pr_url
  const filesChanged = outputs.files_changed

  const summaryText =
    typeof summaryVal === 'string' && summaryVal.trim() !== ''
      ? summaryVal.trim()
      : null

  const artifactsItems = Array.isArray(artifactsVal) ? artifactsVal : null
  const filesChangedItems = Array.isArray(filesChanged) ? filesChanged : null

  const otherEntries = Object.entries(outputs).filter(([k, v]) => {
    if (k === 'summary' && typeof v === 'string' && v.trim() !== '') return false
    if (k === 'artifacts' && (Array.isArray(v) || v === undefined || v === null))
      return false
    if (k === 'pr_url') return false
    if (k === 'files_changed' && Array.isArray(v)) return false
    return true
  })

  const hasArtifactsFallback =
    artifactsVal !== undefined &&
    artifactsVal !== null &&
    !Array.isArray(artifactsVal)

  const hasVisibleBody =
    !!summaryText ||
    !!prUrl ||
    !!filesChangedItems ||
    !!(artifactsItems && artifactsItems.length > 0) ||
    hasArtifactsFallback ||
    otherEntries.length > 0

  if (!hasVisibleBody) return null

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center gap-2">
          <FileText className="text-muted-foreground size-4" />
          <CardTitle className="text-sm">Outputs</CardTitle>
        </div>
      </CardHeader>
      <CardContent className="flex flex-col gap-5">
        {/* PR URL - prominent link */}
        {typeof prUrl === 'string' && prUrl.trim() ? (
          <a
            href={prUrl.trim()}
            target="_blank"
            rel="noopener noreferrer"
            className="group flex items-center gap-2 rounded-lg border border-blue-500/20 bg-blue-500/[0.06] px-3 py-2 transition-colors hover:bg-blue-500/[0.1]"
          >
            <ExternalLink className="size-3.5 text-blue-400" />
            <span className="text-sm font-medium text-blue-400 group-hover:text-blue-300">
              Pull Request
            </span>
            <span className="ml-auto truncate font-mono text-xs text-muted-foreground">
              {prUrl.trim().replace(/^https?:\/\/github\.com\//, '')}
            </span>
          </a>
        ) : null}

        {summaryText ? (
          <CollapsibleOutput label="Summary">
            <p className="text-muted-foreground whitespace-pre-wrap text-sm leading-relaxed">
              {summaryText}
            </p>
          </CollapsibleOutput>
        ) : null}

        {filesChangedItems && filesChangedItems.length > 0 ? (
          <CollapsibleOutput label="Files Changed" defaultOpen={false}>
            <div className="flex flex-wrap gap-1.5">
              {filesChangedItems.map((file, i) => (
                <span
                  key={i}
                  className={cn(
                    'inline-flex items-center gap-1 rounded-md border border-white/[0.06] bg-white/[0.03] px-2 py-0.5 font-mono text-[11px] text-muted-foreground',
                  )}
                >
                  <Code2 className="size-2.5 shrink-0 text-muted-foreground/50" />
                  {String(file)}
                </span>
              ))}
            </div>
          </CollapsibleOutput>
        ) : null}

        {artifactsItems && artifactsItems.length > 0 ? (
          <CollapsibleOutput label="Artifacts">
            <div className="flex flex-col gap-1 rounded-lg border border-white/[0.06] bg-white/[0.02] px-3 py-2.5">
              {artifactsItems.map((item, i) => (
                <div
                  key={i}
                  className="text-foreground font-mono text-xs break-all leading-relaxed"
                >
                  {typeof item === 'string' && isUrl(item) ? (
                    <a
                      href={item}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="inline-flex items-center gap-1 text-blue-400 hover:text-blue-300 transition-colors"
                    >
                      {item}
                      <ExternalLink className="size-2.5" />
                    </a>
                  ) : (
                    formatArtifactItem(item)
                  )}
                </div>
              ))}
            </div>
          </CollapsibleOutput>
        ) : null}

        {hasArtifactsFallback ? (
          <CollapsibleOutput label="Artifacts">
            <SyntaxBlock
              content={
                typeof artifactsVal === 'string'
                  ? artifactsVal
                  : JSON.stringify(artifactsVal, null, 2)
              }
              language="json"
            />
          </CollapsibleOutput>
        ) : null}

        {otherEntries.length > 0 ? (
          <CollapsibleOutput label="Details" defaultOpen={otherEntries.length <= 5}>
            <dl className="flex flex-col gap-3 text-sm">
              {otherEntries.map(([key, val]) => (
                <div key={key} className="flex flex-col gap-1">
                  <dt className="text-muted-foreground/60 font-mono text-xs">
                    {key}
                  </dt>
                  <dd>
                    <OutputValue value={val} />
                  </dd>
                </div>
              ))}
            </dl>
          </CollapsibleOutput>
        ) : null}
      </CardContent>
    </Card>
  )
}
