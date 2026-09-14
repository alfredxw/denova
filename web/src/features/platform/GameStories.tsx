import { useEffect, useState, type ReactNode } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { useGameCreationRequest } from './extension-navigation'
import type { StoryPickerProps } from '@/features/interactive/components/StoryPicker'
import { InlineErrorNotice } from '@/components/common/inline-error-notice'
import { GameStoryContext, type GameChoice } from './game-story-context'
import { InstalledGameStory } from './InstalledGameStory'
import { BUILTIN_GAME_ID, management, localized, platformError, type CatalogEntry, type GamePreferences, type Instance } from './api'

const instanceKey = (id: string) => 'game:' + id
function readSelection(key: string) {
  try { return localStorage.getItem(key) ?? '' } catch { return '' }
}

/** Blends package saves into the existing story picker without introducing a game landing page. */
export function GameStories({ projectId, active, builtinPicker, children }: {
  projectId: string
  active: boolean
  builtinPicker: StoryPickerProps
  children: ReactNode
}) {
  const { t, i18n } = useTranslation()
  const client = useQueryClient()
  const storageKey = 'denova.game-story-selection.' + projectId
  const [selectedId, setSelectedId] = useState(() => readSelection(storageKey))
  const [creating, setCreating] = useState(false)
  const [choice, setChoice] = useState<string | null>(null)
  const requestedGame = useGameCreationRequest(state => state.gameId)
  const catalog = useQuery({ queryKey: ['platform', 'catalog'], queryFn: () => management<CatalogEntry[]>('/catalog'), enabled: active })
  const instances = useQuery({ queryKey: ['platform', 'instances'], queryFn: () => management<Instance[]>('/instances'), enabled: active })
  const preferences = useQuery({ queryKey: ['platform', 'game-preferences'], queryFn: () => management<GamePreferences>('/game-preferences'), enabled: active })
  const installed = (catalog.data ?? []).filter(item => item.kind === 'game')
  const saves = (instances.data ?? []).filter(item => !item.preview && item.projectId === projectId)
  const selected = saves.find(item => item.instanceId === selectedId)
  const choices: GameChoice[] = [{ id: BUILTIN_GAME_ID, name: t('platform.builtin.title') }]
  for (const item of installed) {
    if (!item.enabled || item.removed || item.unavailableReason) continue
    const release = item.releases.find(release => release.ref.releaseId === item.currentRelease)
    if (release) choices.push({ id: item.id, name: localized(release.manifest.name, i18n.language), manifest: release.manifest, releaseId: release.ref.releaseId })
  }
  const preferred = preferences.data?.defaultGameId ?? BUILTIN_GAME_ID
  const defaultGameId = choices.some(item => item.id === preferred) ? preferred : BUILTIN_GAME_ID
  const selectedGameId = choices.some(item => item.id === choice) ? choice! : defaultGameId
  useEffect(() => {
    if (!active || !projectId || !requestedGame || !catalog.isSuccess) return
    const available = catalog.data.some(item => item.kind === 'game' && item.id === requestedGame && item.enabled && !item.removed && !item.unavailableReason && item.releases.some(release => release.ref.releaseId === item.currentRelease))
    useGameCreationRequest.setState({ gameId: null })
    if (!available) { toast.error(t('platform.errors.DEPENDENCY_UNAVAILABLE')); return }
    setSelectedId('')
    setChoice(requestedGame)
    setCreating(true)
    console.info('[game-stories] New story setup opened', { gameId: requestedGame, projectId })
  }, [active, projectId, requestedGame, catalog.isSuccess, catalog.data, t])
  useEffect(() => {
    try { if (selectedId) localStorage.setItem(storageKey, selectedId); else localStorage.removeItem(storageKey) }
    catch (error) { console.warn('[game-stories] Failed to store navigation preference', { error }) }
  }, [selectedId, storageKey])
  useEffect(() => {
    if (instances.isSuccess && selectedId && !selected) setSelectedId('')
  }, [instances.isSuccess, selectedId, selected])
  const refresh = () => client.invalidateQueries({ queryKey: ['platform', 'instances'] })
  const picker: StoryPickerProps = {
    ...builtinPicker,
    stories: [...builtinPicker.stories, ...saves.map(instance => ({
      id: instanceKey(instance.instanceId), title: instance.title, updated_at: instance.createdAt,
      gameName: localized(installed.find(item => item.id === instance.gameId)?.releases.find(release => release.ref.releaseId === instance.releaseId)?.manifest.name, i18n.language) || t('platform.type.game'),
    }))].sort((a, b) => b.updated_at.localeCompare(a.updated_at)),
    currentStoryId: selected ? instanceKey(selected.instanceId) : builtinPicker.currentStoryId,
    onSelect: id => {
      setCreating(false)
      if (id.startsWith('game:')) setSelectedId(id.slice(5))
      else { setSelectedId(''); builtinPicker.onSelect(id) }
    },
    onCreate: () => { setSelectedId(''); setChoice(null); setCreating(true) },
    onDeleteStories: async ids => {
      const builtin = ids.filter(id => !id.startsWith('game:'))
      const operations = ids.filter(id => id.startsWith('game:')).map(id => management('/instances/' + id.slice(5), 'DELETE'))
      if (builtin.length) operations.push(Promise.resolve(builtinPicker.onDeleteStories(builtin)))
      const results = await Promise.allSettled(operations)
      await refresh()
      const failure = results.find(result => result.status === 'rejected')
      if (failure?.status === 'rejected') throw failure.reason
    },
    onRenameStory: async (id, title) => {
      if (id.startsWith('game:')) { await management('/instances/' + id.slice(5), 'PATCH', { title }); await refresh() }
      else await builtinPicker.onRenameStory?.(id, title)
    },
  }
  const error = catalog.error || instances.error || preferences.error
  return <GameStoryContext.Provider value={{
    projectId, creating, setCreating, selectedGameId, chooseGame: setChoice, defaultGameId, choices, picker,
    setDefault: async () => { await management('/game-preferences', 'PATCH', { defaultGameId: selectedGameId }); await client.invalidateQueries({ queryKey: ['platform', 'game-preferences'] }) },
    createInstalled: async (title, setup, storyId) => {
      const game = choices.find(item => item.id === selectedGameId)
      if (!game?.releaseId) throw new Error('Selected game is unavailable')
      const instance = await management<Instance>('/instances', 'POST', { gameId: game.id, releaseId: game.releaseId, title: title.trim() || game.name, projectId, models: setup.models, setup: setup.configuration, ...(storyId ? { storyId } : {}) })
      client.setQueryData<Instance[]>(['platform', 'instances'], previous => [...(previous ?? []), instance])
      setSelectedId(instance.instanceId)
      setCreating(false)
      await refresh()
    },
  }}>
    {error && <InlineErrorNotice className="m-3" message={platformError(error)} />}
    {selected && <InstalledGameStory key={selected.instanceId} instance={selected} item={installed.find(item => item.id === selected.gameId)} active={active} picker={picker} onRefresh={() => { void refresh() }} />}
    <div hidden={!!selected} className={selected ? 'hidden' : 'flex min-h-0 flex-1 flex-col'}>{children}</div>
  </GameStoryContext.Provider>
}
