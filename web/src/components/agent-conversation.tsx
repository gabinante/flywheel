import { useEffect, useRef, useState } from 'react'
import { Bot, Send, User } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Textarea } from '@/components/ui/textarea'
import { relativeTime } from '@/lib/sessions-format'

export type ConversationMessage = { id: string; role: string; content: string; created_at?: string }

/**
 * A minimal chat with an agent: prior turns, a composer, and a pending state while the
 * harness runs (these turns take tens of seconds to minutes).
 */
export function AgentConversation({
  title,
  description,
  messages,
  onSend,
  placeholder,
  emptyHint,
  disabled,
}: {
  title: string
  description?: string
  messages: ConversationMessage[]
  onSend: (text: string) => Promise<string | null>
  placeholder?: string
  emptyHint?: string
  disabled?: boolean
}) {
  const [text, setText] = useState('')
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState<string | null>(null)
  const endRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    endRef.current?.scrollIntoView({ block: 'nearest' })
  }, [messages.length, busy])

  const send = async () => {
    const t = text.trim()
    if (!t || busy) return
    setBusy(true)
    setErr(null)
    const e = await onSend(t)
    setBusy(false)
    if (e) {
      setErr(e)
      return
    }
    setText('')
  }

  return (
    <Card className="border-white/10 bg-white/5 backdrop-blur-md">
      <CardHeader className="py-3">
        <CardTitle className="flex items-center gap-2 text-sm">
          <Bot className="size-4 text-muted-foreground" />
          {title}
        </CardTitle>
        {description && <CardDescription>{description}</CardDescription>}
      </CardHeader>
      <CardContent className="flex flex-col gap-3 pt-0">
        <div className="flex max-h-[28rem] flex-col gap-2 overflow-y-auto pr-1">
          {messages.length === 0 && !busy && <p className="text-xs text-muted-foreground">{emptyHint ?? 'No messages yet.'}</p>}
          {messages.map((m) => (
            <div key={m.id} className={`flex gap-2 ${m.role === 'user' ? 'justify-end' : ''}`}>
              {m.role !== 'user' && <Bot className="mt-1 size-3.5 shrink-0 text-emerald-300" />}
              <div
                className={`max-w-[85%] whitespace-pre-wrap rounded-xl px-3 py-2 text-sm ${
                  m.role === 'user' ? 'bg-primary/15 text-foreground' : 'bg-white/[0.05] text-foreground/90'
                }`}
              >
                {m.content}
                {m.created_at && <div className="mt-1 text-[10px] text-muted-foreground">{relativeTime(m.created_at)}</div>}
              </div>
              {m.role === 'user' && <User className="mt-1 size-3.5 shrink-0 text-muted-foreground" />}
            </div>
          ))}
          {busy && (
            <div className="flex items-center gap-2 text-xs text-muted-foreground">
              <Bot className="size-3.5 animate-pulse text-emerald-300" />
              The agent is working — this can take a minute or two.
            </div>
          )}
          <div ref={endRef} />
        </div>
        {err && <p className="text-xs text-destructive">{err}</p>}
        <form
          className="flex items-end gap-2"
          onSubmit={(e) => {
            e.preventDefault()
            void send()
          }}
        >
          <Textarea
            value={text}
            onChange={(e) => setText(e.target.value)}
            onKeyDown={(e) => {
              if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') void send()
            }}
            placeholder={placeholder ?? 'Ask a question or give an instruction… (⌘↩ to send)'}
            rows={2}
            disabled={busy || disabled}
            className="min-h-[3rem]"
          />
          <Button type="submit" size="sm" disabled={busy || disabled || !text.trim()}>
            <Send className="mr-1.5 size-3.5" />
            Send
          </Button>
        </form>
      </CardContent>
    </Card>
  )
}
