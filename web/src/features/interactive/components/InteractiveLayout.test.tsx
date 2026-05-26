import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { http, HttpResponse } from 'msw'
import { InteractiveLayout } from './InteractiveLayout'
import { useInteractiveStore } from '../stores/interactive-store'
import { server } from '@/test/msw/server'

describe('InteractiveLayout', () => {
  it('renders story stage and snapshot panels', async () => {
    const { container } = render(<InteractiveLayout />)

    expect(await screen.findByText('故事舞台 · 当前分支 main')).toBeInTheDocument()
    expect(screen.getByText('场景记忆')).toBeInTheDocument()
    expect(container.querySelector('[data-slot="select-trigger"]')).toBeInTheDocument()
    expect(container.querySelector('[data-slot="button"]')).toBeInTheDocument()
    expect(container.querySelector('[data-slot="tabs-list"]')).toBeInTheDocument()
    expect(screen.getByTestId('interactive-shell')).toHaveClass('rounded-xl')
    expect(screen.getByTestId('story-stage-card')).toHaveClass('rounded-xl')
  })

  it('can hide interactive side panels independently', async () => {
    render(<InteractiveLayout leftPanelVisible={false} rightPanelVisible={false} />)

    expect(await screen.findByText('故事舞台 · 当前分支 main')).toBeInTheDocument()
    expect(screen.queryByText('资料库')).not.toBeInTheDocument()
    expect(screen.queryByText('场景记忆')).not.toBeInTheDocument()
  })

  it('loads persisted turns from current story snapshot after refresh', async () => {
    useInteractiveStore.setState({
      stories: [],
      tellers: [],
      branches: [],
      snapshot: null,
      currentStoryId: '',
      currentBranchId: 'main',
      submode: 'story',
    })
    server.use(
      http.get('/api/interactive/stories', () => HttpResponse.json({
        current_story_id: 'st_1',
        stories: [{ id: 'st_1', title: '末日开端', origin: '', story_teller_id: 'classic', created_at: '', updated_at: '', branches: 1, events: 1 }],
      })),
      http.get('/api/interactive/stories/:id/snapshot', ({ request }) => {
        const branch = new URL(request.url).searchParams.get('branch')
        return HttpResponse.json({
          story_id: 'st_1',
          branch_id: branch || 'main',
          turns: branch ? [] : [{
            id: 'ev_1',
            parent_id: null,
            branch_id: 'main',
            ts: '',
            user: '我推开酒馆的门',
            narrative: '门后传来低沉的风声。',
          }],
          state: { on_stage: [], characters: {}, events: [] },
        })
      }),
    )

    render(<InteractiveLayout />)

    expect(await screen.findByText('我推开酒馆的门')).toBeInTheDocument()
    expect(screen.getByText('门后传来低沉的风声。')).toBeInTheDocument()
  })

  it('refreshes stage and scene memory from the selected branch snapshot', async () => {
    useInteractiveStore.setState({
      stories: [],
      tellers: [],
      branches: [],
      snapshot: null,
      currentStoryId: '',
      currentBranchId: 'main',
      submode: 'story',
    })
    let currentBranch = 'main'
    server.use(
      http.get('/api/interactive/stories', () => HttpResponse.json({
        current_story_id: 'st_1',
        stories: [{ id: 'st_1', title: '末日开端', origin: '', story_teller_id: 'classic', created_at: '', updated_at: '', branches: 2, events: 2 }],
      })),
      http.post('/api/interactive/stories/:id/switch-branch', async ({ request }) => {
        const body = await request.json() as { branch_id: string }
        currentBranch = body.branch_id || currentBranch
        return HttpResponse.json({ status: 'ok' })
      }),
      http.get('/api/interactive/stories/:id/branches', () => HttpResponse.json({
        branches: [
          { id: 'main', head: 'ev_main', title: '主线', created_at: '', current: currentBranch === 'main' },
          { id: 'br_alt', head: 'ev_alt', from: 'main', from_event: 'ev_main', title: '支线', created_at: '', current: currentBranch === 'br_alt' },
        ],
      })),
      http.get('/api/interactive/stories/:id/snapshot', ({ request }) => {
        const branch = new URL(request.url).searchParams.get('branch') || 'main'
        if (branch === 'br_alt') {
          return HttpResponse.json({
            story_id: 'st_1',
            branch_id: 'br_alt',
            turns: [{
              id: 'ev_alt',
              parent_id: 'ev_main',
              branch_id: 'br_alt',
              ts: '',
              user: '走向另一条巷子',
              narrative: '巷尾传来铃声。',
            }],
            state: { on_stage: ['阿岚'], characters: {}, events: [{ summary: '发现侧巷' }] },
            graph: {
              nodes: [
                { id: 'ev_main', branch_id: 'main', title: '进门', summary: '旧酒馆', ts: '', current: false, head: true },
                { id: 'ev_alt', parent_id: 'ev_main', branch_id: 'br_alt', title: '侧巷', summary: '铃声', ts: '', current: true, head: true },
              ],
              branches: [
                { id: 'main', head: 'ev_main', title: '主线', created_at: '', current: false },
                { id: 'br_alt', head: 'ev_alt', from: 'main', from_event: 'ev_main', title: '支线', created_at: '', current: true },
              ],
            },
          })
        }
        return HttpResponse.json({
          story_id: 'st_1',
          branch_id: 'main',
          turns: [{
            id: 'ev_main',
            parent_id: null,
            branch_id: 'main',
            ts: '',
            user: '进入旧酒馆',
            narrative: '酒馆里只剩炉火。',
          }],
          state: { on_stage: ['林川'], characters: {}, events: [{ summary: '进入酒馆' }] },
          graph: {
            nodes: [
              { id: 'ev_main', branch_id: 'main', title: '进门', summary: '旧酒馆', ts: '', current: true, head: true },
              { id: 'ev_alt', parent_id: 'ev_main', branch_id: 'br_alt', title: '侧巷', summary: '铃声', ts: '', current: false, head: true },
            ],
            branches: [
              { id: 'main', head: 'ev_main', title: '主线', created_at: '', current: true },
              { id: 'br_alt', head: 'ev_alt', from: 'main', from_event: 'ev_main', title: '支线', created_at: '', current: false },
            ],
          },
        })
      }),
    )

    render(<InteractiveLayout />)

    expect(await screen.findByText('进入旧酒馆')).toBeInTheDocument()
    expect(screen.getByText('林川')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /剧情路线图/ }))
    fireEvent.click(await screen.findByText('支线'))

    await screen.findByText('走向另一条巷子')
    expect(screen.getByText('巷尾传来铃声。')).toBeInTheDocument()
    await waitFor(() => expect(screen.queryByText('林川')).not.toBeInTheDocument())
    expect(screen.getByText('阿岚')).toBeInTheDocument()
    expect(screen.getByText('发现侧巷')).toBeInTheDocument()
  })
})
