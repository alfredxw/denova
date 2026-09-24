import { useEffect, useState } from 'react'
import { fetchModelCatalog, fetchSettings } from '@/features/settings/api'
import { GLOBAL_SETTINGS_TARGET, subscribeSettingsTarget } from '@/features/settings/query'
import { modelEndpointsWithDefault, modelProfileLabel, modelProfilesWithDefault } from '@/features/settings/model-profiles'
import type { LayeredSettings, ModelCatalog } from '@/features/settings/types'
import type { AgentEngineID } from './types'

/** Protocol filtering uses the same provider defaults as the backend catalog. */
export function runtimeProfileOptions(engine: AgentEngineID, settings: Pick<LayeredSettings, 'effective'>, catalog: ModelCatalog) {
  const protocol = engine === 'codex' ? 'openai-responses' : 'anthropic-messages'
  const endpoints = modelEndpointsWithDefault(settings.effective)
  return modelProfilesWithDefault(settings.effective).flatMap(profile => {
    const endpoint = endpoints.find(item => item.id === profile.endpoint_id)
    if (!endpoint || !profile.model || engine === 'native') return []
    const actual = endpoint.protocol || catalog.providers.find(item => item.id === endpoint.provider)?.default_protocol
    if (actual !== protocol || Object.keys(endpoint.protocol_options ?? {}).length || (endpoint.session_key_mapping && endpoint.session_key_mapping.location !== 'none')) return []
    return [{ id: `profile:${profile.id}`, label: modelProfileLabel(profile), modelLabel: modelProfileLabel(profile) }]
  })
}

export function useRuntimeProfiles(engine: AgentEngineID) {
  const [settings, setSettings] = useState<LayeredSettings | null>(null)
  const [catalog, setCatalog] = useState<ModelCatalog | null>(null)
  const [failed, setFailed] = useState(false)
  useEffect(() => {
    if (engine === 'native') return
    let active = true
    setFailed(false)
    const unsubscribe = subscribeSettingsTarget(GLOBAL_SETTINGS_TARGET, setSettings)
    void Promise.all([fetchSettings(), fetchModelCatalog()]).then(([next, models]) => {
      if (active) { setSettings(next); setCatalog(models) }
    }).catch(cause => {
      console.warn('[agent-runtime] load API profiles failed', cause)
      if (active) setFailed(true)
    })
    return () => { active = false; unsubscribe() }
  }, [engine])
  return { profiles: settings && catalog ? runtimeProfileOptions(engine, settings, catalog) : [], loaded: Boolean(settings && catalog), failed }
}
