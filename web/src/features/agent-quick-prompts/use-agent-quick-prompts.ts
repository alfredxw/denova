import { useCallback, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { patchSettings, refreshSettings, type SettingsPatch } from '@/features/settings/api'
import { GLOBAL_SETTINGS_TARGET, settingsQueryOptions } from '@/features/settings/query'
import type { AgentQuickPromptSettings, LayeredSettings } from '@/features/settings/types'
import { saveWithRevisionRecovery } from '@/lib/revision-conflict'
import { queryClient } from '@/lib/query-client'

export function useAgentQuickPrompts(scope: string, defaults: AgentQuickPromptSettings[]) {
  const query = useQuery(settingsQueryOptions(GLOBAL_SETTINGS_TARGET), queryClient)
  const registry = query.data?.user.agent_quick_prompts
  const customized = Boolean(registry && Object.prototype.hasOwnProperty.call(registry, scope))
  const prompts = useMemo(
    () => clonePrompts(customized ? registry?.[scope] ?? [] : defaults),
    [customized, defaults, registry, scope],
  )

  const save = useCallback(async (next: AgentQuickPromptSettings[] | null): Promise<LayeredSettings> => {
    const initial = await refreshSettings()
    return saveWithRevisionRecovery<AgentQuickPromptSettings[] | null, LayeredSettings>({
      baseline: initial.user.agent_quick_prompts?.[scope] ?? null,
      draft: next,
      revision: initial.revisions?.user,
      save: (draft, revision) => patchSettings('user', quickPromptPatch(scope, draft), revision),
      loadLatest: async () => {
        const latest = await refreshSettings()
        return {
          value: latest.user.agent_quick_prompts?.[scope] ?? null,
          revision: latest.revisions?.user,
        }
      },
      rebase: (_baseline, draft) => draft,
    })
  }, [scope])

  return {
    customized,
    loading: query.isLoading,
    prompts,
    save,
  }
}

function quickPromptPatch(scope: string, prompts: AgentQuickPromptSettings[] | null): SettingsPatch {
  return { agent_quick_prompts: { [scope]: prompts } }
}

function clonePrompts(prompts: AgentQuickPromptSettings[]): AgentQuickPromptSettings[] {
  return prompts.map((prompt) => ({ ...prompt }))
}
