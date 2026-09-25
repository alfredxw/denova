import { useCallback, useEffect, useLayoutEffect, useRef } from 'react'
import { toast } from '@/lib/toast'
import { useTranslation } from 'react-i18next'
import { useSpeechSettings } from '@/features/speech/hooks'
import { speechConfigError, speechPlayer } from '@/features/speech/player'
import { speechText } from '@/features/speech/text'
import type { InteractiveTurnPersistedEvent, Snapshot, StorySummary, TurnEvent } from './types'
import { sanitizeStoredNarrative } from './stream-parser'

/** Live persistence is the only automatic trigger. Replayed stream checkpoints,
 * snapshot hydration, version selection and edited saves cannot enqueue audio. */
export function useStorySpeech({ owner, story, snapshot, active }: { owner: string; story?: StorySummary; snapshot: Snapshot | null; active: boolean }) {
  const { t } = useTranslation()
  const { settings } = useSpeechSettings()
  const mode = story?.speech_settings?.mode || 'all'
  const ignoreAsterisks = story?.speech_settings?.ignore_asterisks || false
  const seen = useRef(new Set<string>())
  const originals = useRef(new Map<string, string>())
  const latest = useRef({ owner, story, active, settings, mode, ignoreAsterisks })
  useLayoutEffect(() => { latest.current = { owner, story, active, settings, mode, ignoreAsterisks } })

  useEffect(() => {
    seen.current.clear()
    originals.current.clear()
    return () => speechPlayer.stopOwner(owner)
  }, [owner])
  useEffect(() => { if (!active) speechPlayer.stopOwner(owner) }, [active, owner])
  useEffect(() => { speechPlayer.stopOwner(owner) }, [owner, mode, ignoreAsterisks])
  useEffect(() => {
    for (const turn of snapshot?.turns || []) {
      seen.current.add(turn.id)
      if (originals.current.has(turn.id) && originals.current.get(turn.id) !== turn.narrative) speechPlayer.stopOwner(owner)
    }
  }, [snapshot, owner])

  const read = useCallback((turn: TurnEvent, enqueue = false) => {
    const current = latest.current
    if (!current.active) return
    const text = speechText(sanitizeStoredNarrative(turn.narrative), { mode: current.mode, ignore_asterisks: current.ignoreAsterisks })
    if (!text && !enqueue) toast.info(t('speech.status.empty'))
    originals.current.set(turn.id, turn.narrative)
    speechPlayer.read({ owner: current.owner, turnId: turn.id, text, settings: current.settings, enqueue })
  }, [t])
  const onPersisted = useCallback((event: InteractiveTurnPersistedEvent, options?: { replayed: boolean }) => {
    const current = latest.current
    if (event.story_id !== current.story?.id || event.branch_id !== snapshot?.branch_id || seen.current.has(event.turn.id)) return
    seen.current.add(event.turn.id)
    if (!options?.replayed && current.active && current.story?.speech_settings?.auto_read) read(event.turn, true)
  }, [read, snapshot?.branch_id])
  return { configured: !speechConfigError(settings), read, onPersisted, stop: () => speechPlayer.stopOwner(owner) }
}
