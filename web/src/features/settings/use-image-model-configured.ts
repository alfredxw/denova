import { useQuery } from '@tanstack/react-query'
import { queryClient } from '@/lib/query-client'
import { hasConfiguredImageModel } from './image-profiles'
import { GLOBAL_SETTINGS_TARGET, projectSettingsTarget, settingsQueryOptions } from './query'

/** Share live model availability between image controls and generation actions. */
export function useImageModelConfigured(projectId?: string): boolean {
  const query = useQuery(settingsQueryOptions(projectId ? projectSettingsTarget(projectId) : GLOBAL_SETTINGS_TARGET), queryClient)
  return hasConfiguredImageModel(query.data?.effective)
}
