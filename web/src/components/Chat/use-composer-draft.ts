import { useCallback, useEffect, useMemo, useRef, useState, useSyncExternalStore, type SetStateAction } from 'react'

const STORAGE_PREFIX = 'denova.composerDraft.v1:'
const MAX_SAVED_LENGTH = 128 * 1024

type DraftSnapshot = { text: string; savedText: string; saveFailed?: boolean }

function createDraft(key: string, persistent: boolean) {
  const storageKey = STORAGE_PREFIX + key
  const subscribers = new Set<() => void>()
  function read(): DraftSnapshot {
    try {
      const text = window.localStorage.getItem(storageKey) ?? ''
      return { text, savedText: text }
    } catch {
      return { text: '', savedText: '' }
    }
  }

  let snapshot: DraftSnapshot = persistent ? read() : { text: '', savedText: '' }
  function publish(next: DraftSnapshot) {
    snapshot = next
    subscribers.forEach((notify) => notify())
  }

  return {
    getSnapshot: () => snapshot,
    subscribe(notify: () => void) {
      subscribers.add(notify)
      return () => { subscribers.delete(notify) }
    },
    refresh() {
      const saved = read()
      if (!snapshot.saveFailed && saved.text !== snapshot.savedText) publish(saved)
    },
    write(text: string, persist: boolean, savedText = text) {
      if (!savedText.trim()) savedText = ''
      if (persist) {
        try {
          if (savedText.length > MAX_SAVED_LENGTH) throw new Error('Composer draft exceeds the storage limit')
          if (savedText) window.localStorage.setItem(storageKey, savedText)
          else window.localStorage.removeItem(storageKey)
        } catch (error) {
          if (!snapshot.saveFailed) console.warn('[use-composer-draft] Failed to save composer text', error)
          publish({ text, savedText, saveFailed: true })
          return
        }
      }
      publish({ text, savedText })
    },
  }
}

const drafts = new Map<string, ReturnType<typeof createDraft>>()

/** One text buffer per composer destination. A receipt can only consume the snapshot it submitted. */
export function useComposerDraft(key: string | undefined, persistent: boolean) {
  const draft = useMemo(() => {
    if (!key) return createDraft('', false)
    const existing = drafts.get(key)
    if (existing) return existing
    const next = createDraft(key, persistent)
    drafts.set(key, next)
    return next
  }, [key, persistent])
  const snapshot = useSyncExternalStore(draft.subscribe, draft.getSnapshot)
  const previous = useRef({ draft, persistent })
  const [restored, setRestored] = useState<typeof draft | null>(null)

  useEffect(() => {
    if (persistent && previous.current.draft === draft && !previous.current.persistent) {
      // A first send can promote the tab while the user is already typing the next message.
      const { text, savedText } = draft.getSnapshot()
      if (text) draft.write(text, true, savedText)
    } else if (persistent) {
      draft.refresh()
    }
    previous.current = { draft, persistent }
  }, [draft, persistent])

  useEffect(() => {
    if (!key || !persistent) return
    const onStorage = (event: StorageEvent) => {
      if (event.key === STORAGE_PREFIX + key || event.key === null) draft.refresh()
    }
    window.addEventListener('storage', onStorage)
    return () => window.removeEventListener('storage', onStorage)
  }, [draft, key, persistent])

  useEffect(() => {
    if (!persistent || !draft.getSnapshot().text) return
    setRestored(draft)
    const timeout = window.setTimeout(() => setRestored(null), 3000)
    return () => { window.clearTimeout(timeout); setRestored(null) }
  }, [draft, persistent])

  const setText = useCallback((update: SetStateAction<string>, savedText?: string) => {
    const current = draft.getSnapshot()
    const text = typeof update === 'function' ? update(current.text) : update
    const nextSavedText = savedText ?? (typeof update === 'function' ? update(current.savedText) : text)
    if (text === current.text && nextSavedText === current.savedText) return
    setRestored(null)
    draft.write(text, persistent, nextSavedText)
  }, [draft, persistent])

  function accept() {
    if (persistent) draft.refresh()
    if (draft.getSnapshot() !== snapshot) return false
    if (snapshot.text) draft.write('', persistent)
    return true
  }

  return {
    text: snapshot.text,
    setText,
    accept,
    status: snapshot.saveFailed ? 'saveFailed' as const : restored === draft && snapshot.text ? 'restored' as const : undefined,
  }
}
