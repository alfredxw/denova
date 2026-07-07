import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useAgentEventStream } from '@/hooks/useAgentEventStream'
import { generateStoryMemoryStream, getStoryMemory } from '../api'
import type { Snapshot, StoryMemoryState } from '../types'
import { DirectorConsole } from './director-console/DirectorConsole'
import { allStructuresId, type ConsoleTab } from './director-console/types'
import { readNumber, storyMemoryEnabled, storyMemorySearchText } from './director-console/utils'

interface MemoryPanelProps {
  storyId?: string
  branchId?: string
  snapshot: Snapshot | null
  loading?: boolean
  refreshKey?: string | number
  onOpenMemoryManager?: () => void
  onSnapshotRefresh?: () => void | Promise<unknown>
}

export function MemoryPanel({ storyId, branchId, snapshot, loading = false, refreshKey, onOpenMemoryManager, onSnapshotRefresh }: MemoryPanelProps) {
  const { t } = useTranslation()
  const [memory, setMemory] = useState<StoryMemoryState | null>(null)
  const [memoryLoading, setMemoryLoading] = useState(false)
  const [error, setError] = useState('')
  const [query, setQuery] = useState('')
  const [selectedStructureId, setSelectedStructureId] = useState(allStructuresId)
  const [activeTab, setActiveTab] = useState<ConsoleTab>('run')
  const [directorRevealed, setDirectorRevealed] = useState(false)
  const autoGenerateTurnKeyRef = useRef('')

  const effectiveBranchId = branchId || snapshot?.branch_id || ''
  const turnSyncStatus = snapshot?.current_turn?.memory_status || snapshot?.current_turn?.state_status || ''
  const syncStatus = turnSyncStatus === 'pending' || turnSyncStatus === 'failed' ? turnSyncStatus : memory?.sync_status || turnSyncStatus
  const syncError = snapshot?.current_turn?.memory_error || snapshot?.current_turn?.state_error || memory?.sync_error || ''

  useEffect(() => {
    setActiveTab('run')
    setDirectorRevealed(false)
  }, [effectiveBranchId, storyId])

  const loadMemory = useCallback(async () => {
    if (!storyId) {
      setMemory(null)
      return
    }
    setMemoryLoading(true)
    setError('')
    try {
      const next = await getStoryMemory(storyId, effectiveBranchId, false)
      setMemory(next)
      setSelectedStructureId((current) => {
        if (current === allStructuresId || next.structures.some((structure) => structure.id === current)) return current
        return allStructuresId
      })
    } catch (err) {
      console.error('[interactive-memory-panel] load failed', err)
      setError(err instanceof Error ? err.message : t('memoryPanel.loadFailed'))
    } finally {
      setMemoryLoading(false)
    }
  }, [effectiveBranchId, storyId, t])

  const { messages: generateMessages, setMessages: setGenerateMessages, isStreaming: generating, activityContent: generateActivity, consumeAgentStream, resetStreamingState, setAbortController, abortLocalStream } = useAgentEventStream({
    onEvent: (event, data) => {
      if (event.event !== 'story_memory_result') return
      setGenerateMessages(prev => [...prev, {
        role: 'system',
        content: t('memoryPanel.generateDone', {
          patches: readNumber(data.patches),
          records: readNumber(data.records),
        }),
      }])
      void loadMemory()
    },
  })

  useEffect(() => {
    void loadMemory()
  }, [loadMemory, refreshKey])

  const structures = useMemo(() => (memory?.structures || []).filter((structure) => storyMemoryEnabled(structure.enabled)), [memory?.structures])
  const filteredRecords = useMemo(() => {
    const needle = query.trim().toLowerCase()
    const enabledStructureIds = new Set(structures.map((structure) => structure.id))
    const source = (memory?.records || []).filter((record) => enabledStructureIds.has(record.structure_id))
    if (!needle) return source
    return source.filter((record) => {
      const structure = structures.find((item) => item.id === record.structure_id)
      return storyMemorySearchText(record, structure).toLowerCase().includes(needle)
    })
  }, [memory?.records, query, structures])
  const structureRecordCounts = useMemo(() => {
    const counts = new Map<string, number>()
    filteredRecords.forEach((record) => counts.set(record.structure_id, (counts.get(record.structure_id) || 0) + 1))
    return counts
  }, [filteredRecords])
  const visibleStructures = useMemo(() => {
    if (selectedStructureId === allStructuresId) return structures
    return structures.filter((structure) => structure.id === selectedStructureId)
  }, [selectedStructureId, structures])

  const runStoryMemoryGenerate = useCallback(async (source: 'manual' | 'auto' = 'manual') => {
    if (!storyId || generating) return
    if (source === 'manual') {
      setActiveTab('run')
    }
    resetStreamingState()
    setGenerateMessages([{ role: 'user', content: source === 'auto' ? t('memoryPanel.autoGenerateRequest') : t('memoryPanel.generateRequest') }])
    const controller = new AbortController()
    setAbortController(controller)
    try {
      const stream = await generateStoryMemoryStream(storyId, effectiveBranchId, source, controller.signal)
      await consumeAgentStream(stream)
      await loadMemory()
    } catch (err) {
      console.error('[interactive-memory-panel] generate stream failed', err)
      setGenerateMessages(prev => [...prev, { role: 'error', content: err instanceof Error ? err.message : t('memoryPanel.generateFailed') }])
      resetStreamingState()
    }
  }, [consumeAgentStream, effectiveBranchId, generating, loadMemory, resetStreamingState, setAbortController, setGenerateMessages, storyId, t])

  useEffect(() => {
    const turn = snapshot?.current_turn
    if (!storyId || !effectiveBranchId || !turn?.id || turn.memory_status !== 'pending' || generating) return
    const turnKey = `${storyId}:${effectiveBranchId}:${turn.id}`
    if (autoGenerateTurnKeyRef.current === turnKey) return
    autoGenerateTurnKeyRef.current = turnKey
    void runStoryMemoryGenerate('auto')
  }, [effectiveBranchId, generating, runStoryMemoryGenerate, snapshot?.current_turn?.id, snapshot?.current_turn?.memory_status, storyId])

  return (
    <DirectorConsole
      storyId={storyId}
      branchId={effectiveBranchId}
      snapshot={snapshot}
      loading={loading}
      memoryLoading={memoryLoading}
      memoryError={error}
      syncStatus={syncStatus}
      syncError={syncError}
      activeTab={activeTab}
      onTabChange={setActiveTab}
      directorRevealed={directorRevealed}
      onRevealDirector={() => setDirectorRevealed(true)}
      structures={structures}
      filteredRecords={filteredRecords}
      visibleStructures={visibleStructures}
      structureRecordCounts={structureRecordCounts}
      selectedStructureId={selectedStructureId}
      onSelectStructure={setSelectedStructureId}
      query={query}
      onQueryChange={setQuery}
      generateMessages={generateMessages}
      generating={generating}
      generateActivity={generateActivity}
      onGenerateMemory={() => void runStoryMemoryGenerate()}
      onAbortGenerate={abortLocalStream}
      onOpenMemoryManager={onOpenMemoryManager}
      onSnapshotRefresh={onSnapshotRefresh}
    />
  )
}
