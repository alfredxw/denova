import { useCallback, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { patchSettings, refreshSettings } from '@/features/settings/api'
import { GLOBAL_SETTINGS_TARGET, settingsQueryOptions } from '@/features/settings/query'
import type { AgentQuickPromptSettings, LayeredSettings } from '@/features/settings/types'
import { saveWithRevisionRecovery } from '@/lib/revision-conflict'
import { queryClient } from '@/lib/query-client'

export interface QuickPromptSettingsChanges {
  prompts?: AgentQuickPromptSettings[] | null
  showInCommands?: boolean
}

export function useAgentQuickPrompts(scope: string | undefined, defaults: AgentQuickPromptSettings[]) {
  const query = useQuery({ ...settingsQueryOptions(GLOBAL_SETTINGS_TARGET), enabled: Boolean(scope) }, queryClient)
  const registry = query.data?.user.agent_quick_prompts
  const customized = Boolean(scope && registry && Object.prototype.hasOwnProperty.call(registry, scope))
  const prompts = useMemo(
    () => clonePrompts(customized && scope ? registry?.[scope] ?? [] : defaults),
    [customized, defaults, registry, scope],
  )

  const save = useCallback(async (changes: QuickPromptSettingsChanges): Promise<LayeredSettings> => {
    if (!scope) throw new Error('Quick prompt scope is required')
    const initial = await refreshSettings()
    return saveWithRevisionRecovery<QuickPromptSettingsChanges, LayeredSettings>({
      baseline: {
        prompts: initial.user.agent_quick_prompts?.[scope] ?? null,
        showInCommands: initial.user.agent_quick_prompts_in_commands === true,
      },
      draft: changes,
      revision: initial.revisions?.user,
      save: (draft, revision) => patchSettings('user', {
        ...(draft.prompts !== undefined ? { agent_quick_prompts: { [scope]: draft.prompts } } : {}),
        ...(draft.showInCommands !== undefined ? { agent_quick_prompts_in_commands: draft.showInCommands } : {}),
      }, revision),
      loadLatest: async () => {
        const latest = await refreshSettings()
        return {
          value: {
            prompts: latest.user.agent_quick_prompts?.[scope] ?? null,
            showInCommands: latest.user.agent_quick_prompts_in_commands === true,
          },
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
    showInCommands: query.data?.user.agent_quick_prompts_in_commands === true,
    save,
  }
}

function clonePrompts(prompts: AgentQuickPromptSettings[]): AgentQuickPromptSettings[] {
  return prompts.map((prompt) => ({ ...prompt }))
}
