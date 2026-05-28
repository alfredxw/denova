import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { StoryStage } from './StoryStage'
import { abortInteractiveChat, sendInteractiveMessage } from '../api'
import { useInteractiveStore } from '../stores/interactive-store'
import type { InteractiveSSEEvent } from '../types'

vi.mock('../api', () => ({
  abortInteractiveChat: vi.fn(),
  sendInteractiveMessage: vi.fn(),
}))

function streamEvents(events: InteractiveSSEEvent[]): ReadableStream<InteractiveSSEEvent> {
  return new ReadableStream<InteractiveSSEEvent>({
    start(controller) {
      for (const event of events) controller.enqueue(event)
      controller.close()
    },
  })
}

describe('StoryStage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    useInteractiveStore.setState({ storyStageRuns: {} })
  })

  it('uses chat messages for interactive history and streamed agent events', async () => {
    vi.mocked(sendInteractiveMessage).mockResolvedValue(streamEvents([
      { event: 'thinking', data: JSON.stringify({ content: '先判断现场风险。' }) },
      { event: 'chunk', data: JSON.stringify({ content: '<NARRATIVE>\n火光照亮了' }) },
      { event: 'chunk', data: JSON.stringify({ content: '墙上的新线索。</NARRATIVE><STATE' }) },
      { event: 'chunk', data: JSON.stringify({ content: '_DELTA>{"ops":[{"op":"set","path":"on_stage","value":["林川"]}]}</STATE_DELTA>' }) },
      { event: 'done', data: '{}' },
    ]))
    const onDone = vi.fn()

    const initialSnapshot = {
      story_id: 'st_1',
      branch_id: 'main',
      state: {},
      turns: [
        {
          id: 'ev_1',
          parent_id: null,
          branch_id: 'main',
          ts: '',
          user: '我推开酒馆的门',
          narrative: '门后传来低沉的风声。',
        },
      ],
    }
    const { container, rerender } = render(
      <StoryStage
        storyId="st_1"
        branchId="main"
        snapshot={initialSnapshot}
        onDone={onDone}
      />,
    )

    expect(screen.getAllByText('Nova').length).toBeGreaterThan(0)
    expect(screen.getByTestId('story-stage-card').parentElement).toHaveClass('h-full', 'overflow-hidden')
    expect(screen.getByText('我推开酒馆的门')).toBeInTheDocument()
    expect(screen.getByText('门后传来低沉的风声。')).toBeInTheDocument()

    fireEvent.change(screen.getByPlaceholderText('你要做什么？'), { target: { value: '我点燃火把' } })
    fireEvent.click(screen.getByRole('button', { name: /发送/ }))

    await screen.findByText('我点燃火把')
    expect(screen.queryByText('先判断现场风险。')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: /思考过程/ })).toBeInTheDocument()
    await screen.findByText(/火光照亮了墙上的新线索。/)
    expect(screen.queryByText(/STATE_DELTA/)).not.toBeInTheDocument()
    expect(screen.queryByText(/on_stage/)).not.toBeInTheDocument()
    await waitFor(() => expect(onDone).toHaveBeenCalledTimes(1))

    rerender(
      <StoryStage
        storyId="st_1"
        branchId="main"
        snapshot={{
          ...initialSnapshot,
          turns: [
            ...initialSnapshot.turns,
            {
              id: 'ev_2',
              parent_id: 'ev_1',
              branch_id: 'main',
              ts: '',
              user: '我点燃火把',
              narrative: '火光照亮了墙上的新线索。',
              thinking: '先判断现场风险。',
            },
          ],
        }}
        onDone={onDone}
      />,
    )
    expect(screen.getAllByText('我点燃火把')).toHaveLength(1)
    expect(screen.getAllByText(/火光照亮了墙上的新线索。/)).toHaveLength(1)
    expect(container.querySelector('.streaming-markdown')).toBeInTheDocument()
  })

  it('restores persisted thinking as a collapsed block after refresh', async () => {
    render(
      <StoryStage
        storyId="st_1"
        branchId="main"
        snapshot={{
          story_id: 'st_1',
          branch_id: 'main',
          state: {},
          turns: [{
            id: 'ev_1',
            parent_id: null,
            branch_id: 'main',
            ts: '',
            user: '观察柜台',
            narrative: '柜台后方传来一声轻响。',
            thinking: '先整理当前场景和风险。',
          }],
        }}
        onDone={vi.fn()}
      />,
    )

    expect(screen.getByRole('button', { name: /思考过程/ })).toBeInTheDocument()
    expect(screen.queryByText('先整理当前场景和风险。')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /思考过程/ }))
    expect(await screen.findByText('先整理当前场景和风险。')).toBeInTheDocument()
  })

  it('fills the input from hot state choices without sending immediately', () => {
    render(
      <StoryStage
        storyId="st_1"
        branchId="main"
        snapshot={{
          story_id: 'st_1',
          branch_id: 'main',
          state: {},
          current_turn: {
            id: 'ev_1',
            parent_id: null,
            branch_id: 'main',
            ts: '',
            user: '观察酒馆',
            narrative: '柜台后的影子露出一道缝。',
            state_status: 'pending',
            hot_state: {
              choices: ['我靠近地窖门，观察门缝和周围痕迹。'],
            },
          },
          turns: [],
        }}
        onDone={vi.fn()}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: '我靠近地窖门，观察门缝和周围痕迹。' }))

    expect(screen.getByPlaceholderText('你要做什么？')).toHaveValue('我靠近地窖门，观察门缝和周围痕迹。')
    expect(sendInteractiveMessage).not.toHaveBeenCalled()
  })

  it('clears transient stage messages when switching to another branch snapshot', async () => {
    vi.mocked(sendInteractiveMessage).mockResolvedValue(streamEvents([
      { event: 'chunk', data: JSON.stringify({ content: '<NARRATIVE>临时路线的火光亮起。</NARRATIVE>' }) },
      { event: 'done', data: '{}' },
    ]))
    const onDone = vi.fn()
    const { rerender } = render(
      <StoryStage
        storyId="st_1"
        branchId="main"
        snapshot={{ story_id: 'st_1', branch_id: 'main', state: {}, turns: [] }}
        onDone={onDone}
      />,
    )

    fireEvent.change(screen.getByPlaceholderText('你要做什么？'), { target: { value: '点燃火把' } })
    fireEvent.click(screen.getByRole('button', { name: /发送/ }))

    await screen.findByText('点燃火把')
    await screen.findByText('临时路线的火光亮起。')
    await waitFor(() => expect(onDone).toHaveBeenCalledTimes(1))

    rerender(
      <StoryStage
        storyId="st_1"
        branchId="br_alt"
        snapshot={{
          story_id: 'st_1',
          branch_id: 'br_alt',
          state: {},
          turns: [{
            id: 'ev_alt',
            parent_id: null,
            branch_id: 'br_alt',
            ts: '',
            user: '走向另一条巷子',
            narrative: '巷尾传来铃声。',
          }],
        }}
        onDone={onDone}
      />,
    )

    await waitFor(() => expect(screen.queryByText('点燃火把')).not.toBeInTheDocument())
    expect(screen.queryByText('临时路线的火光亮起。')).not.toBeInTheDocument()
    expect(screen.getByText('走向另一条巷子')).toBeInTheDocument()
    expect(screen.getByText('巷尾传来铃声。')).toBeInTheDocument()
  })

  it('does not force story stage back to bottom when a finished turn is persisted', () => {
    const onDone = vi.fn()
    const initialSnapshot = {
      story_id: 'st_1',
      branch_id: 'main',
      state: {},
      turns: [
        {
          id: 'ev_1',
          parent_id: null,
          branch_id: 'main',
          ts: '',
          user: '查看墙上的旧地图',
          narrative: '地图边缘写着潮湿的红字。',
        },
      ],
    }
    const { container, rerender } = render(
      <StoryStage
        storyId="st_1"
        branchId="main"
        snapshot={initialSnapshot}
        onDone={onDone}
      />,
    )
    const scroller = container.querySelector('.overflow-y-auto') as HTMLDivElement
    let scrollTop = 0
    Object.defineProperty(scroller, 'scrollHeight', { configurable: true, get: () => 1200 })
    Object.defineProperty(scroller, 'clientHeight', { configurable: true, get: () => 320 })
    Object.defineProperty(scroller, 'scrollTop', {
      configurable: true,
      get: () => scrollTop,
      set: (value) => {
        scrollTop = value
      },
    })

    scroller.scrollTop = 200
    fireEvent.scroll(scroller)

    rerender(
      <StoryStage
        storyId="st_1"
        branchId="main"
        snapshot={{
          ...initialSnapshot,
          turns: [
            ...initialSnapshot.turns,
            {
              id: 'ev_2',
              parent_id: 'ev_1',
              branch_id: 'main',
              ts: '',
              user: '沿着红字指向的楼梯下去',
              narrative: '楼梯尽头传来缓慢的敲击声。',
            },
          ],
        }}
        onDone={onDone}
      />,
    )

    expect(scroller.scrollTop).toBe(200)
  })

  it('sends with Enter and keeps Shift+Enter for newline', async () => {
    vi.mocked(sendInteractiveMessage).mockResolvedValue(streamEvents([{ event: 'done', data: '{}' }]))

    render(
      <StoryStage
        storyId="st_1"
        branchId="main"
        snapshot={{ story_id: 'st_1', branch_id: 'main', state: {}, turns: [] }}
        onDone={vi.fn()}
      />,
    )

    const input = screen.getByPlaceholderText('你要做什么？')
    fireEvent.change(input, { target: { value: '第一行' } })
    fireEvent.keyDown(input, { key: 'Enter', shiftKey: true })
    expect(sendInteractiveMessage).not.toHaveBeenCalled()

    fireEvent.keyDown(input, { key: 'Enter' })
    await waitFor(() => expect(sendInteractiveMessage).toHaveBeenCalledTimes(1))
  })

  it('can abort a running interactive agent output', async () => {
    vi.mocked(sendInteractiveMessage).mockResolvedValue(new ReadableStream<InteractiveSSEEvent>())

    render(
      <StoryStage
        storyId="st_1"
        branchId="main"
        snapshot={{ story_id: 'st_1', branch_id: 'main', state: {}, turns: [] }}
        onDone={vi.fn()}
      />,
    )

    fireEvent.change(screen.getByPlaceholderText('你要做什么？'), { target: { value: '继续探索' } })
    fireEvent.click(screen.getByRole('button', { name: '发送' }))
    await screen.findByRole('button', { name: '中断 AI 执行' })
    fireEvent.click(screen.getByRole('button', { name: '中断 AI 执行' }))

    await waitFor(() => expect(abortInteractiveChat).toHaveBeenCalledTimes(1))
  })

  it('keeps streamed output visible after the story stage remounts', async () => {
    let controller: ReadableStreamDefaultController<InteractiveSSEEvent>
    vi.mocked(sendInteractiveMessage).mockResolvedValue(new ReadableStream<InteractiveSSEEvent>({
      start(streamController) {
        controller = streamController
      },
    }))
    const onDone = vi.fn()
    const props = {
      workspace: '/books/demo',
      storyId: 'st_1',
      branchId: 'main',
      snapshot: { story_id: 'st_1', branch_id: 'main', state: {}, turns: [] },
      onDone,
    }

    const view = render(<StoryStage {...props} />)
    fireEvent.change(screen.getByPlaceholderText('你要做什么？'), { target: { value: '继续' } })
    fireEvent.click(screen.getByRole('button', { name: '发送' }))
    await screen.findByText('继续')

    controller!.enqueue({ event: 'chunk', data: JSON.stringify({ content: '<NARRATIVE>雨声' }) })
    await screen.findByText('雨声')
    view.unmount()

    controller!.enqueue({ event: 'chunk', data: JSON.stringify({ content: '逼近。</NARRATIVE>' }) })
    await waitFor(() => {
      const run = useInteractiveStore.getState().storyStageRuns['/books/demo:st_1:main']
      expect(run?.liveMessages.some((message) => message.content?.includes('雨声逼近。'))).toBe(true)
    })

    render(<StoryStage {...props} />)
    expect(await screen.findByText('雨声逼近。')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '中断 AI 执行' })).toBeInTheDocument()

    controller!.enqueue({ event: 'done', data: '{}' })
    controller!.close()
    await waitFor(() => expect(onDone).toHaveBeenCalledTimes(1))
  })
})
