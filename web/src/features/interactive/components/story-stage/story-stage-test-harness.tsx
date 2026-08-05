import { screen, waitFor } from '@testing-library/react'
import { useState, type ComponentProps } from 'react'
import { VirtuosoMockContext } from 'react-virtuoso'
import { expect, type Mock } from 'vitest'
import { StoryStage as ProjectStoryStage } from '../StoryStage'
import { mergeInteractiveTurnPersistedSnapshot, useInteractiveStore } from '../../stores/interactive-store'
import type { InteractiveTurnPersistedEvent, Snapshot, StorySummary, TurnEvent } from '../../types'

export interface StoryStageTestMocks {
  generateInteractiveImageMock: Mock
  getActiveInteractiveChatMock: Mock
  runInteractiveDirectorMock: Mock
  sendInteractiveMessageMock: Mock
  streamActiveInteractiveChatMock: Mock
  submitInteractiveAgentCommandMock: Mock
  updateInteractiveTurnNarrativeMock: Mock
  useSkillCommandsMock: Mock
}

function StoryStage(props: Omit<ComponentProps<typeof ProjectStoryStage>, 'projectId'>) {
  return <ProjectStoryStage {...props} projectId="project-story" />
}

/** Resets only shared DOM/store state and mock defaults; each suite owns its module mocks. */
export function resetStoryStageTestHarness({
  generateInteractiveImageMock,
  getActiveInteractiveChatMock,
  runInteractiveDirectorMock,
  sendInteractiveMessageMock,
  streamActiveInteractiveChatMock,
  submitInteractiveAgentCommandMock,
  updateInteractiveTurnNarrativeMock,
  useSkillCommandsMock,
}: StoryStageTestMocks) {
  window.localStorage.clear()
  useInteractiveStore.setState({ storyStageRuns: {} })
  generateInteractiveImageMock.mockReset()
  generateInteractiveImageMock.mockResolvedValue({ enabled: false, skipped: true })
  getActiveInteractiveChatMock.mockReset()
  getActiveInteractiveChatMock.mockResolvedValue({ active: false })
  runInteractiveDirectorMock.mockReset()
  runInteractiveDirectorMock.mockResolvedValue(directorStatus('running', { completed_docs: 1 }))
  sendInteractiveMessageMock.mockReset()
  streamActiveInteractiveChatMock.mockReset()
  submitInteractiveAgentCommandMock.mockReset()
  submitInteractiveAgentCommandMock.mockResolvedValue({ command_id: 'command-1', operation_id: 'operation-1', cursor: 7 })
  updateInteractiveTurnNarrativeMock.mockReset()
  useSkillCommandsMock.mockReset()
  useSkillCommandsMock.mockReturnValue([])
}

export function ReplyEditHarness({ readSavedNarrative }: { readSavedNarrative: () => string | undefined }) {
  const initialTurn: TurnEvent = {
    id: 'turn-edit',
    parent_id: null,
    branch_id: 'main',
    ts: '2026-06-28T00:00:00Z',
    user: '去找朋友',
    narrative: '朋友住在 3 楼 403 室。',
  }
  const [snapshot, setSnapshot] = useState<Snapshot>({
    story_id: 'story-1',
    branch_id: 'main',
    turns: [initialTurn],
    current_turn: initialTurn,
    state: {},
  })

  return (
    <VirtuosoMockContext.Provider value={{ viewportHeight: 1200, itemHeight: 120 }}>
      <StoryStage
        workspace="/tmp/book"
        stories={[story()]}
        story={story()}
        tellers={[]}
        storyId="story-1"
        branchId="main"
        snapshot={snapshot}
        onDone={() => {
          const narrative = readSavedNarrative() || initialTurn.narrative
          const turn = { ...snapshot.turns[0], narrative }
          const nextSnapshot = { ...snapshot, turns: [turn], current_turn: turn }
          setSnapshot(nextSnapshot)
          return Promise.resolve(nextSnapshot)
        }}
      />
    </VirtuosoMockContext.Provider>
  )
}

export function StoryStageHarness({
  initialSnapshot,
  onDone,
}: {
  initialSnapshot?: Snapshot
  onDone?: (options?: { silent?: boolean }) => Promise<Snapshot | void>
} = {}) {
  const [snapshot, setSnapshot] = useState<Snapshot>(
    initialSnapshot || { story_id: 'story-1', branch_id: 'main', turns: [], state: {} },
  )
  const nextSnapshot: Snapshot = {
    story_id: 'story-1',
    branch_id: 'main',
    state: {},
    turns: [
      {
        id: 'turn-1',
        parent_id: null,
        branch_id: 'main',
        ts: '2026-06-28T00:00:00Z',
        user: '继续前进',
        narrative: '故事继续。',
      },
    ],
  }

  return (
    <VirtuosoMockContext.Provider value={{ viewportHeight: 1200, itemHeight: 120 }}>
      <StoryStage
        workspace="/tmp/book"
        stories={[story()]}
        story={story()}
        tellers={[]}
        storyId="story-1"
        branchId="main"
        snapshot={snapshot}
        onDone={
          onDone || (() => {
            setSnapshot(nextSnapshot)
            return Promise.resolve(nextSnapshot)
          })
        }
      />
    </VirtuosoMockContext.Provider>
  )
}

export function getStageInput() {
  const input = screen.getAllByRole('textbox').find((element) => element.getAttribute('enterkeyhint') === 'send')
  if (!input) throw new Error('stage input missing')
  return input
}

export function PersistedTurnHarness({ onDone }: { onDone: (options?: { silent?: boolean }) => Promise<void> }) {
  const [snapshot, setSnapshot] = useState<Snapshot>({ story_id: 'story-1', branch_id: 'main', turns: [], state: {} })

  return (
    <VirtuosoMockContext.Provider value={{ viewportHeight: 1200, itemHeight: 120 }}>
      <StoryStage
        workspace="/tmp/book"
        stories={[story()]}
        story={story()}
        tellers={[]}
        storyId="story-1"
        branchId="main"
        snapshot={snapshot}
        onTurnPersisted={(event) => {
          const nextSnapshot = mergeInteractiveTurnPersistedSnapshot(snapshot, event)
          setSnapshot(nextSnapshot)
          return nextSnapshot
        }}
        onDone={onDone}
      />
    </VirtuosoMockContext.Provider>
  )
}

export function persistedTurnEvent(): InteractiveTurnPersistedEvent {
  return {
    story_id: 'story-1',
    branch_id: 'main',
    turn_count: 1,
    turn: {
      id: 'turn-1',
      parent_id: null,
      branch_id: 'main',
      ts: '2026-06-28T00:00:00Z',
      user: '推门',
      narrative: '门外有灯。',
    },
    state: { scene: { location: '门外' } },
    graph: {
      nodes: [{
        id: 'turn-1',
        branch_id: 'main',
        title: '推门',
        summary: '门外有灯。',
        ts: '2026-06-28T00:00:00Z',
        current: true,
        head: true,
      }],
      branches: [{ id: 'main', head: 'turn-1', created_at: '2026-06-28T00:00:00Z', current: true }],
    },
    branches: [{ id: 'main', head: 'turn-1', created_at: '2026-06-28T00:00:00Z', current: true }],
  }
}

export function directorStatus(status: string, overrides: Partial<NonNullable<Snapshot['director_plan_status']>> = {}) {
  return {
    story_id: 'story-1',
    branch_id: 'main',
    status,
    summary: status === 'running' ? '后台导演正在规划开局。' : '后台导演更新失败，已保留现有规划。',
    error: '',
    source_turn_id: 'turn-1',
    updated_at: '2026-06-28T00:00:00Z',
    planned_docs: 1,
    completed_docs: status === 'ready' ? 1 : 0,
    doc_bytes: 1200,
    visible_bytes: 320,
    start_ready: status === 'ready',
    blocking: false,
    ...overrides,
  }
}

export function interactiveStream(events: Array<{ event: string; data: string }>) {
  return new ReadableStream({
    start(controller) {
			let persisted = false
      for (const event of events) {
				if (event.event === 'interactive_turn_persisted') persisted = true
				if (event.event === 'done' && !persisted) {
					controller.enqueue({ event: 'interactive_turn_persisted', data: JSON.stringify(persistedTurnEvent()) })
					persisted = true
				}
        controller.enqueue(event)
      }
      controller.close()
    },
  })
}

export function controllableInteractiveStream() {
  let controller: ReadableStreamDefaultController<{ id?: string; event: string; data: string }> | null = null
  let closed = false
  const readable = new ReadableStream<{ id?: string; event: string; data: string }>({
    start(nextController) {
      controller = nextController
    },
    cancel() {
      closed = true
    },
  })
  return {
    readable,
    enqueue(event: { id?: string; event: string; data: string }) {
      if (closed) return
      controller?.enqueue(event)
    },
    close() {
      if (closed) return
      closed = true
      controller?.close()
    },
  }
}

export function runAnimationFrames(frames: Map<number, FrameRequestCallback>) {
  const callbacks = [...frames.entries()]
  frames.clear()
  for (const [, callback] of callbacks) {
    callback(performance.now())
  }
}

export async function expectVisibleText(text: string) {
  await waitFor(() => {
    const visible = screen.getAllByText(text).find((element) => !element.closest('[aria-hidden="true"]'))
    expect(visible).toBeVisible()
  })
}

export function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
}

export function story(): StorySummary {
  return {
    id: 'story-1',
    title: '故事',
    origin: '',
    story_teller_id: 'classic',
    story_director_id: 'default',
    choice_count: 5,
    reply_target_chars: 2000,
    image_settings: { mode: 'manual', interval_turns: 3 },
    opening: { mode: 'ai' },
    created_at: '2026-06-27T00:00:00Z',
    updated_at: '2026-06-27T00:00:00Z',
    branches: 1,
    events: 0,
    turn_count: 0,
  }
}

export function storyDirector(ruleVisibilityMode: string) {
  return {
    version: 3,
    id: 'default',
    name: '默认故事导演',
    description: '',
    strategy: {
      enabled: true,
      rule_visibility_mode: ruleVisibilityMode,
		},
		trpg_system: { rule_templates: [] },
		custom: false,
  }
}

export function snapshotWithRuleResolution(): Snapshot {
  return {
    story_id: 'story-1',
    branch_id: 'main',
    state: {},
    turns: [{
      id: 'turn-1',
      parent_id: null,
      branch_id: 'main',
      ts: '2026-06-28T00:00:00Z',
      user: '强行闯入藏书阁',
      narrative: '守阁长老拦在门前。',
      rule_resolution: {
        id: 'rr_1',
        request: {
          action: '强行闯入藏书阁',
          intent: '冒险',
          challenge: '潜入检定',
          cost: '失败会损失体力并暴露行踪',
          state: '守阁长老正在靠近',
          adjudication: {
            stakes: '失败会暴露行踪。',
          },
          difficulty: 'hard',
          outcomes: {
            critical_success: { result: '无声潜入。' },
            success: { result: '成功潜入。' },
            failure: { result: '强闯失败导致主线中断', state_changes: [{ actor_id: 'protagonist', field_id: '当前生命', change: -10, reason: '被禁制反震' }] },
            critical_failure: { result: '被当场抓住。' },
          },
        },
        result: {
          id: 'check_1',
          label: '潜入检定',
          dice: '1d20',
          roll_mode: 'normal',
          rolls: [4],
          kept_roll: 4,
          base_target: 15,
          bonus_total: 2,
          target: 18,
          total: 6,
          outcome: 'failure',
          result: '强闯失败导致主线中断',
          state_changes: [{ actor_id: 'protagonist', field_id: '当前生命', change: -10, reason: '被禁制反震' }],
        },
      },
    }],
  }
}
