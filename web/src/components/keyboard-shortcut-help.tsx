import { useEffect } from 'react'

type Shortcut = {
  key: string
  label: string
}

const SHORTCUTS: Shortcut[] = [
  { key: 'j / ↓', label: 'Next ticket' },
  { key: 'k / ↑', label: 'Previous ticket' },
  { key: 'a', label: 'Approve current ticket' },
  { key: 'r', label: 'Reject current ticket' },
  { key: 'o', label: 'Reopen current ticket' },
  { key: 'n', label: 'Focus notes field' },
  { key: 'Enter', label: 'Expand / collapse details' },
  { key: 'Esc', label: 'Close overlay / blur input' },
  { key: '?', label: 'Toggle this help' },
]

type KeyboardShortcutHelpProps = {
  open: boolean
  onClose: () => void
}

export function KeyboardShortcutHelp({ open, onClose }: KeyboardShortcutHelpProps) {
  useEffect(() => {
    if (!open) return
    function handleKey(e: KeyboardEvent) {
      if (e.key === 'Escape' || e.key === '?') {
        e.preventDefault()
        onClose()
      }
    }
    window.addEventListener('keydown', handleKey)
    return () => window.removeEventListener('keydown', handleKey)
  }, [open, onClose])

  if (!open) return null

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm"
      onClick={onClose}
      role="dialog"
      aria-label="Keyboard shortcuts"
    >
      <div
        className="w-full max-w-sm rounded-2xl border border-white/10 bg-card p-6 backdrop-blur-md"
        onClick={(e) => e.stopPropagation()}
      >
        <h2 className="text-foreground mb-4 text-lg font-semibold tracking-tight">
          Keyboard shortcuts
        </h2>
        <dl className="flex flex-col gap-2">
          {SHORTCUTS.map((s) => (
            <div key={s.key} className="flex items-center justify-between gap-4">
              <dt className="text-muted-foreground text-sm">{s.label}</dt>
              <dd>
                <kbd className="rounded-lg border border-white/10 bg-white/[0.06] px-2 py-0.5 font-mono text-xs text-foreground backdrop-blur-sm">
                  {s.key}
                </kbd>
              </dd>
            </div>
          ))}
        </dl>
        <p className="text-muted-foreground mt-4 text-center text-xs">
          Press <kbd className="font-mono">?</kbd> or <kbd className="font-mono">Esc</kbd> to close
        </p>
      </div>
    </div>
  )
}
