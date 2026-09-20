import { useEffect, useRef, useSyncExternalStore } from 'react'
import { useQuery } from '@tanstack/react-query'
import { queryClient } from '@/lib/query-client'
import { GLOBAL_SETTINGS_TARGET, settingsQueryOptions } from '@/features/settings/query'
import { speechPlayer } from './player'

export function useSpeechPlayer() { return useSyncExternalStore(speechPlayer.subscribe, speechPlayer.getSnapshot) }

export function useSpeechSettings() {
  const query = useQuery(settingsQueryOptions(GLOBAL_SETTINGS_TARGET), queryClient)
  const settings = query.data?.effective.speech
  const signature = JSON.stringify(settings)
  const previous = useRef(signature)
  useEffect(() => {
    if (previous.current !== signature && speechPlayer.getSnapshot().owner !== 'preview') speechPlayer.clear()
    previous.current = signature
  }, [signature])
  return { settings, loading: query.isPending, error: query.isError }
}
