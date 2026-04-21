import { useEffect } from 'react'

type Shortcut = {
  key: string
  label: string
}

type ShortcutGroup = {
  title: string
  shortcuts: Shortcut[]
}

const SHORTCUT_GROUPS: ShortcutGroup[] = [
  {
    title: 'Navigation',
    shortcuts: [
      { key: 'j / ↓', label: 'Next ticket' },
      { key: 'k / ↑', label: 'Previous ticket' },
      { key: 'Enter', label: 'Expand / collapse details' },
    ],
  },
  {
    title: 'Actions',
    shortcuts: [
      { key: 'a', label: 'Approve current ticket' },
      { key: 'r', label: 'Reject current ticket' },
      { key: 'o', label: 'Reopen current ticket' },
      { key: 'n', label: 'Focus notes field' },
    ],
  },
  {
    title: 'App',
    shortcuts: [
      { key: ']', label: 'Toggle side panel' },
      { key: '?', label: 'Toggle this help' },
      { key: 'Esc', label: 'Close overlay / blur input' },
    ],
  },
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
        className="w-full max-w-sm rounded-2xl border border-white/10 bg-background/80 p-6 backdrop-blur-xl"
        onClick={(e) => e.stopPropagation()}
      >
        <h2 className="text-foreground mb-4 text-lg font-semibold tracking-tight">
          Keyboard shortcuts
        </h2>
        <div className="flex flex-col gap-4">
          {SHORTCUT_GROUPS.map((group) => (
            <div key={group.title}>
              <h3 className="text-muted-foreground/70 mb-1.5 text-xs font-medium uppercase tracking-wider">
                {group.title}
              </h3>
              <dl className="flex flex-col gap-1.5">
                {group.shortcuts.map((s) => (
                  <div key={s.key} className="flex items-center justify-between gap-4">
                    <dt className="text-muted-foreground text-sm">{s.label}</dt>
                    <dd>
                      <kbd className="bg-white/5 text-foreground rounded-md border border-white/10 px-2 py-0.5 font-mono text-xs">
                        {s.key}
                      </kbd>
                    </dd>
                  </div>
                ))}
              </dl>
            </div>
          ))}
        </div>
        <p className="text-muted-foreground mt-4 text-center text-xs">
          Press <kbd className="font-mono">?</kbd> or <kbd className="font-mono">Esc</kbd> to close
        </p>
      </div>
    </div>
  )
}
