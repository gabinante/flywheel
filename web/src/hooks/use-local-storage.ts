import { useCallback, useSyncExternalStore } from 'react'

/**
 * A React hook that reads/writes a value to localStorage and stays
 * in sync across tabs (via the `storage` event) and within the same
 * tab (via a module-level notify mechanism).
 */
export function useLocalStorage<T>(
  key: string,
  initialValue: T,
): [T, (value: T | ((prev: T) => T)) => void] {
  const subscribe = useCallback(
    (onStoreChange: () => void) => {
      const handler = (e: StorageEvent) => {
        if (e.key === key) onStoreChange()
      }
      window.addEventListener('storage', handler)
      window.addEventListener(`local-storage:${key}`, onStoreChange)
      return () => {
        window.removeEventListener('storage', handler)
        window.removeEventListener(`local-storage:${key}`, onStoreChange)
      }
    },
    [key],
  )

  const getSnapshot = useCallback((): T => {
    try {
      const raw = localStorage.getItem(key)
      return raw === null ? initialValue : (JSON.parse(raw) as T)
    } catch {
      return initialValue
    }
  }, [key, initialValue])

  const value = useSyncExternalStore(subscribe, getSnapshot, () => initialValue)

  const setValue = useCallback(
    (next: T | ((prev: T) => T)) => {
      const prev = getSnapshot()
      const resolved = next instanceof Function ? next(prev) : next
      localStorage.setItem(key, JSON.stringify(resolved))
      window.dispatchEvent(new Event(`local-storage:${key}`))
    },
    [key, getSnapshot],
  )

  return [value, setValue]
}
