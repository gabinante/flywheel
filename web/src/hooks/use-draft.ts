import { useState, type Dispatch, type SetStateAction } from 'react'

// Preserve edits until the authoritative value changes, then reset before
// children receive a frame of another project's draft.
export function useDraft<T>(source: T): [T, Dispatch<SetStateAction<T>>] {
  const [draft, setDraft] = useState({ source, value: source })
  const changed = !Object.is(source, draft.source)
  if (changed) setDraft({ source, value: source })
  return [changed ? source : draft.value, (next) => setDraft((previous) => ({
    source,
    value: typeof next === 'function' ? (next as (value: T) => T)(previous.value) : next,
  }))]
}
