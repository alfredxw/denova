import { useQuery } from '@tanstack/react-query'
import { useLocalAccess } from '@/features/remote-access/api'
import { management, type PluginPage } from './api'

/** Navigation reads the page catalog without loading plugin players or editors. */
export function usePluginPages() {
  const local = useLocalAccess()
  return useQuery({
    queryKey: ['platform', 'plugin-pages'],
    queryFn: () => management<PluginPage[]>('/plugin-pages'),
    enabled: local,
    refetchInterval: 5000,
  })
}
