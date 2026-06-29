import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { VirtuosoMockContext } from 'react-virtuoso'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { StoryStage } from './StoryStage'
import type { Snapshot, StorySummary } from '../types'

const { generateInteractiveHotChoicesMock, generateInteractiveImageMock, sendInteractiveMessageMock } = vi.hoisted(() => ({
  generateInteractiveHotChoicesMock: vi.fn(),
  generateInteractiveImageMock: vi.fn(),
  sendInteractiveMessageMock: vi.fn(),
}))

vi.mock('@/features/settings/api', () => ({
  fetchSettings: vi.fn().mockResolvedValue({ effective: {} }),
}))

vi.mock('@/hooks/useSkillCommands', () => ({
  useSkillCommands: () => [],
}))

vi.mock('../api', () => ({
  abortInteractiveChat: vi.fn(),
  analyzeInteractiveContext: vi.fn(),
  compactInteractiveContext: vi.fn(),
  generateInteractiveHotChoices: generateInteractiveHotChoicesMock,
  generateInteractiveImage: generateInteractiveImageMock,
  removeInteractiveContextCompaction: vi.fn(),
  sendInteractiveMessage: sendInteractiveMessageMock,
  switchInteractiveTurnVersion: vi.fn(),
}))

beforeEach(() => {
  window.localStorage.clear()
  generateInteractiveHotChoicesMock.mockReset()
  generateInteractiveImageMock.mockReset()
  generateInteractiveImageMock.mockResolvedValue({ enabled: false, skipped: true })
  sendInteractiveMessageMock.mockReset()
})

describe('StoryStage hot choices mode', () => {
  it('defaults to auto and keeps generated choices hidden until the user opens them', async () => {
    const user = userEvent.setup()
    sendInteractiveMessageMock.mockResolvedValue(interactiveStream([
      { event: 'chunk', data: JSON.stringify({ content: '故事继续。' }) },
      { event: 'done', data: '{}' },
    ]))
    generateInteractiveHotChoicesMock.mockResolvedValue({
      enabled: true,
      choices: ['查看门后', '询问守夜人'],
    })

    render(<StoryStageHarness />)

    await user.type(screen.getByPlaceholderText('你要做什么？'), '继续前进')
    await user.click(screen.getByRole('button', { name: '发送' }))

    await waitFor(() => {
      expect(generateInteractiveHotChoicesMock).toHaveBeenCalledWith('story-1', expect.objectContaining({
        branch: 'main',
        exclude_choices: [],
      }))
    })
    expect(screen.queryByText('查看门后')).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '获取行动选择' }))

    expect(await screen.findByText('查看门后')).toBeInTheDocument()
    expect(screen.getByTestId('story-stage-hot-choices-list')).toHaveClass('flex-wrap')
  })

  it('opens the choices panel with a loading state while background generation is pending', async () => {
    const user = userEvent.setup()
    const pendingChoices = deferred<{ enabled: boolean; choices: string[] }>()
    sendInteractiveMessageMock.mockResolvedValue(interactiveStream([
      { event: 'chunk', data: JSON.stringify({ content: '故事继续。' }) },
      { event: 'done', data: '{}' },
    ]))
    generateInteractiveHotChoicesMock.mockReturnValue(pendingChoices.promise)

    render(<StoryStageHarness />)

    await user.type(screen.getByPlaceholderText('你要做什么？'), '继续前进')
    await user.click(screen.getByRole('button', { name: '发送' }))

    await waitFor(() => expect(generateInteractiveHotChoicesMock).toHaveBeenCalled())
    await user.click(screen.getByRole('button', { name: '获取行动选择' }))

    expect(screen.getByText('正在生成可选择行动…')).toBeInTheDocument()

    pendingChoices.resolve({ enabled: true, choices: ['贴近门缝听动静'] })

    expect(await screen.findByText('贴近门缝听动静')).toBeInTheDocument()
  })

  it('does not auto-generate choices after switching to manual mode', async () => {
    const user = userEvent.setup()
    sendInteractiveMessageMock.mockResolvedValue(interactiveStream([
      { event: 'chunk', data: JSON.stringify({ content: '故事继续。' }) },
      { event: 'done', data: '{}' },
    ]))
    generateInteractiveHotChoicesMock.mockResolvedValue({
      enabled: true,
      choices: ['查看门后'],
    })

    render(<StoryStageHarness />)

    fireEvent.pointerDown(screen.getByRole('button', { name: '输入动作' }))
    await waitFor(() => expect(screen.getByRole('menuitem', { name: /行动选项/ })).toBeInTheDocument())
    await user.hover(screen.getByRole('menuitem', { name: /行动选项/ }))
    await waitFor(() => expect(screen.getByRole('menuitem', { name: '手动生成' })).toBeInTheDocument())
    fireEvent.click(screen.getByRole('menuitem', { name: '手动生成' }))
    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('menuitem', { name: '手动生成' })).not.toBeInTheDocument())

    await user.type(screen.getByPlaceholderText('你要做什么？'), '继续前进')
    await user.click(screen.getByRole('button', { name: '发送' }))

    await waitFor(() => expect(screen.getByText('故事继续。')).toBeInTheDocument())
    expect(generateInteractiveHotChoicesMock).not.toHaveBeenCalled()
  })
})

describe('StoryStage streaming rendering', () => {
  it('batches fast interactive chunks into one animation frame without slicing text', async () => {
    const user = userEvent.setup()
    const stream = controllableInteractiveStream()
    const originalRequestAnimationFrame = window.requestAnimationFrame
    const originalCancelAnimationFrame = window.cancelAnimationFrame
    const frames = new Map<number, FrameRequestCallback>()
    let nextFrameId = 1
    window.requestAnimationFrame = vi.fn((callback: FrameRequestCallback) => {
      const id = nextFrameId
      nextFrameId += 1
      frames.set(id, callback)
      return id
    })
    window.cancelAnimationFrame = vi.fn((id: number) => {
      frames.delete(id)
    })

    try {
      sendInteractiveMessageMock.mockResolvedValue(stream.readable)
      render(<StoryStageHarness />)

      await user.type(screen.getByPlaceholderText('你要做什么？'), '继续前进')
      await user.click(screen.getByRole('button', { name: '发送' }))

      await waitFor(() => expect(sendInteractiveMessageMock).toHaveBeenCalled())
      act(() => runAnimationFrames(frames))
      stream.enqueue({ event: 'chunk', data: JSON.stringify({ content: '青石镇外' }) })
      stream.enqueue({ event: 'chunk', data: JSON.stringify({ content: '风声忽然停了。' }) })
      await waitFor(() => expect(frames.size).toBeGreaterThan(0))
      expect(screen.queryByText('青石镇外风声忽然停了。')).not.toBeInTheDocument()

      act(() => runAnimationFrames(frames))

      expect(await screen.findByText('青石镇外风声忽然停了。')).toBeInTheDocument()
      stream.enqueue({ event: 'done', data: '{}' })
      stream.close()
    } finally {
      stream.close()
      window.requestAnimationFrame = originalRequestAnimationFrame
      window.cancelAnimationFrame = originalCancelAnimationFrame
    }
  })
})

describe('StoryStage interactive image settings', () => {
  it('sets interactive image mode from the input actions submenu', async () => {
    const user = userEvent.setup()
    const handleImageSettingsChange = vi.fn().mockResolvedValue(undefined)
    render(
      <StoryStage
        workspace="/tmp/book"
        stories={[story()]}
        story={story()}
        tellers={[]}
        storyId="story-1"
        branchId="main"
        snapshot={{ story_id: 'story-1', branch_id: 'main', turns: [], state: {} }}
        onDone={() => {}}
        onImageSettingsChange={handleImageSettingsChange}
      />,
    )

    fireEvent.pointerDown(screen.getByRole('button', { name: '输入动作' }))
    await waitFor(() => expect(screen.getByText('互动图像')).toBeInTheDocument())
    await user.hover(screen.getByRole('menuitem', { name: /互动图像/ }))
    await waitFor(() => expect(screen.getByRole('menuitem', { name: /每 3 轮生成/ })).toBeInTheDocument())
    fireEvent.click(screen.getByRole('menuitem', { name: /每 3 轮生成/ }))

    await waitFor(() => {
      expect(handleImageSettingsChange).toHaveBeenCalledWith({ mode: 'interval', interval_turns: 3, preset_id: 'game-cg' })
    })
  })
})

describe('StoryStage interactive image rendering', () => {
  it('手动生成互动图像成功后不等刷新快照也会立即渲染到对应回合', async () => {
    const user = userEvent.setup()
    const handleDone = vi.fn().mockResolvedValue(undefined)
    generateInteractiveImageMock.mockResolvedValue({
      enabled: true,
      image: {
        schema: 'interactive_image.v1',
        story_id: 'story-1',
        branch_id: 'main',
        turn_id: 'turn-1',
        image_path: 'assets/interactive/images/story-1/main/turn-1/run-a/image.png',
        meta_path: 'assets/interactive/images/story-1/main/turn-1/run-a/meta.json',
        alt_text: '即时互动图像',
      },
    })

    render(
      <VirtuosoMockContext.Provider value={{ viewportHeight: 1200, itemHeight: 120 }}>
        <StoryStage
          workspace="/tmp/book"
          stories={[story()]}
          story={story()}
          tellers={[]}
          storyId="story-1"
          branchId="main"
          snapshot={{
            story_id: 'story-1',
            branch_id: 'main',
            state: {},
            turns: [{
              id: 'turn-1',
              parent_id: null,
              branch_id: 'main',
              ts: '2026-06-28T00:00:00Z',
              user: '继续前进',
              narrative: '玄璃抬头，看见雾气里有一道微光。',
            }],
          }}
          onDone={handleDone}
        />
      </VirtuosoMockContext.Provider>,
    )

    await user.click(screen.getByRole('button', { name: '生成互动图像' }))

    await waitFor(() => {
      expect(screen.getByRole('img', { name: '即时互动图像' })).toHaveAttribute('src', '/api/workspace/asset?path=assets%2Finteractive%2Fimages%2Fstory-1%2Fmain%2Fturn-1%2Frun-a%2Fimage.png')
    })
    expect(handleDone).toHaveBeenCalled()
  })
})

describe('StoryStage opening panel', () => {
  it('fills custom opening text from the book preset button', () => {
    render(
      <StoryStage
        workspace="/tmp/book"
        stories={[story()]}
        story={story()}
        tellers={[]}
        storyId="story-1"
        branchId="main"
        snapshot={{ story_id: 'story-1', branch_id: 'main', turns: [], state: {} }}
        bookOpeningPresets={[{ id: 'preset-1', title: '默认开场', content: '青石镇的雨刚刚停。' }]}
        onDone={() => {}}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: '使用书籍预设' }))

    expect(screen.getByPlaceholderText('写下你想使用的开局。生成时会作为有界来源传给游戏 Agent。')).toHaveValue('青石镇的雨刚刚停。')
  })
})

function StoryStageHarness() {
  const [snapshot, setSnapshot] = useState<Snapshot>({ story_id: 'story-1', branch_id: 'main', turns: [], state: {} })
  const nextSnapshot: Snapshot = {
    story_id: 'story-1',
    branch_id: 'main',
    state: {},
    turns: [{
      id: 'turn-1',
      parent_id: null,
      branch_id: 'main',
      ts: '2026-06-28T00:00:00Z',
      user: '继续前进',
      narrative: '故事继续。',
    }],
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
        onDone={() => {
          setSnapshot(nextSnapshot)
          return Promise.resolve(nextSnapshot)
        }}
      />
    </VirtuosoMockContext.Provider>
  )
}

function interactiveStream(events: Array<{ event: string; data: string }>) {
  return new ReadableStream({
    start(controller) {
      for (const event of events) {
        controller.enqueue(event)
      }
      controller.close()
    },
  })
}

function controllableInteractiveStream() {
  let controller: ReadableStreamDefaultController<{ event: string; data: string }> | null = null
  let closed = false
  const readable = new ReadableStream<{ event: string; data: string }>({
    start(nextController) {
      controller = nextController
    },
  })
  return {
    readable,
    enqueue(event: { event: string; data: string }) {
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

function runAnimationFrames(frames: Map<number, FrameRequestCallback>) {
  const callbacks = [...frames.entries()]
  frames.clear()
  for (const [, callback] of callbacks) {
    callback(performance.now())
  }
}

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
}

function story(): StorySummary {
  return {
    id: 'story-1',
    title: '故事',
    origin: '',
    story_teller_id: 'classic',
    reply_target_chars: 2000,
    image_settings: { mode: 'manual', interval_turns: 3 },
    opening: { mode: 'ai' },
    created_at: '2026-06-27T00:00:00Z',
    updated_at: '2026-06-27T00:00:00Z',
    branches: 1,
    events: 0,
  }
}
