import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
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
    expect(screen.getByTestId('interactive-shell')).not.toHaveClass('rounded-xl')
    expect(screen.getByTestId('story-stage-card')).not.toHaveClass('rounded-xl')
    expect(screen.queryByTestId('branch-graph-canvas')).not.toBeInTheDocument()
  })

  it('can hide interactive side panels independently', async () => {
    render(<InteractiveLayout rightPanelVisible={false} />)

    expect(await screen.findByText('故事舞台 · 当前分支 main')).toBeInTheDocument()
    expect(screen.queryByText('设定条目与 Markdown 正文')).not.toBeInTheDocument()
    expect(screen.queryByText('场景记忆')).not.toBeInTheDocument()
  })

  it('reloads stories and snapshot when workspace changes', async () => {
    useInteractiveStore.setState({
      stories: [],
      tellers: [],
      branches: [],
      snapshot: null,
      currentStoryId: '',
      currentBranchId: 'main',
      submode: 'story',
    })
    let activeWorkspace = 'old'
    server.use(
      http.get('/api/interactive/stories', () => {
        const isNew = activeWorkspace === 'new'
        return HttpResponse.json({
          current_story_id: isNew ? 'st_new' : 'st_old',
          stories: [
            {
              id: isNew ? 'st_new' : 'st_old',
              title: isNew ? '新书故事' : '旧书故事',
              origin: '',
              story_teller_id: 'classic',
              reply_target_chars: 1200,
              created_at: '',
              updated_at: '',
              branches: 1,
              events: 1,
            },
          ],
        })
      }),
      http.get('/api/interactive/stories/:id/branches', () =>
        HttpResponse.json({
          branches: [
            {
              id: 'main',
              head: '',
              title: '主线',
              created_at: '',
              current: true,
            },
          ],
        }),
      ),
      http.get('/api/interactive/stories/:id/snapshot', ({ params }) => {
        const isNew = params.id === 'st_new'
        return HttpResponse.json({
          story_id: params.id,
          branch_id: 'main',
          turns: [
            {
              id: isNew ? 'ev_new' : 'ev_old',
              parent_id: null,
              branch_id: 'main',
              ts: '',
              user: isNew ? '新书回合' : '旧书回合',
              narrative: isNew ? '新的场景展开。' : '旧的场景残留。',
            },
          ],
          state: {
            on_stage: [isNew ? '新角色' : '旧角色'],
            characters: {},
            events: [],
          },
        })
      }),
    )

    const view = render(<InteractiveLayout workspace="/books/old" />)

    expect((await screen.findAllByText('旧书回合')).length).toBeGreaterThan(0)

    activeWorkspace = 'new'
    view.rerender(<InteractiveLayout workspace="/books/new" />)

    expect((await screen.findAllByText('新书回合')).length).toBeGreaterThan(0)
    expect(screen.getAllByText('新的场景展开。').length).toBeGreaterThan(0)
    await waitFor(() => expect(screen.queryByText('旧角色')).not.toBeInTheDocument())
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
      http.get('/api/interactive/stories', () =>
        HttpResponse.json({
          current_story_id: 'st_1',
          stories: [
            {
              id: 'st_1',
              title: '末日开端',
              origin: '',
              story_teller_id: 'classic',
              reply_target_chars: 1200,
              created_at: '',
              updated_at: '',
              branches: 1,
              events: 1,
            },
          ],
        }),
      ),
      http.get('/api/interactive/stories/:id/snapshot', ({ request }) => {
        const branch = new URL(request.url).searchParams.get('branch')
        return HttpResponse.json({
          story_id: 'st_1',
          branch_id: branch || 'main',
          turns: branch
            ? []
            : [
                {
                  id: 'ev_1',
                  parent_id: null,
                  branch_id: 'main',
                  ts: '',
                  user: '我推开酒馆的门',
                  narrative: '门后传来低沉的风声。',
                },
              ],
          state: { on_stage: [], characters: {}, events: [] },
        })
      }),
    )

    render(<InteractiveLayout />)

    expect((await screen.findAllByText('我推开酒馆的门')).length).toBeGreaterThan(0)
    expect(screen.getAllByText('门后传来低沉的风声。').length).toBeGreaterThan(0)
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
    let switchCalls = 0
    let releaseAltSnapshot!: () => void
    const altSnapshotReady = new Promise<void>((resolve) => {
      releaseAltSnapshot = resolve
    })
    const snapshotBranches: string[] = []
    server.use(
      http.get('/api/interactive/stories', () =>
        HttpResponse.json({
          current_story_id: 'st_1',
          stories: [
            {
              id: 'st_1',
              title: '末日开端',
              origin: '',
              story_teller_id: 'classic',
              reply_target_chars: 1200,
              created_at: '',
              updated_at: '',
              branches: 2,
              events: 2,
            },
          ],
        }),
      ),
      http.post('/api/interactive/stories/:id/switch-branch', async ({ request }) => {
        switchCalls += 1
        const body = (await request.json()) as { branch_id: string }
        currentBranch = body.branch_id || currentBranch
        return HttpResponse.json({ status: 'ok' })
      }),
      http.get('/api/interactive/stories/:id/branches', () =>
        HttpResponse.json({
          branches: [
            {
              id: 'main',
              head: 'ev_main',
              title: '主线',
              created_at: '',
              current: currentBranch === 'main',
            },
            {
              id: 'br_alt',
              head: 'ev_alt',
              from: 'main',
              from_event: 'ev_main',
              title: '支线',
              created_at: '',
              current: currentBranch === 'br_alt',
            },
          ],
        }),
      ),
      http.get('/api/interactive/stories/:id/snapshot', ({ request }) => {
        const branch = new URL(request.url).searchParams.get('branch') || 'main'
        snapshotBranches.push(branch)
        if (branch === 'br_alt') {
          return altSnapshotReady.then(() =>
            HttpResponse.json({
              story_id: 'st_1',
              branch_id: 'br_alt',
              turns: [
                {
                  id: 'ev_alt',
                  parent_id: 'ev_main',
                  branch_id: 'br_alt',
                  ts: '',
                  user: '走向另一条巷子',
                  narrative: '巷尾传来铃声。',
                },
              ],
              state: {
                on_stage: ['阿岚'],
                characters: {},
                events: [{ summary: '发现侧巷' }],
              },
              graph: {
                nodes: [
                  {
                    id: 'ev_main',
                    branch_id: 'main',
                    title: '进门',
                    summary: '旧酒馆',
                    ts: '',
                    current: false,
                    head: true,
                  },
                  {
                    id: 'ev_alt',
                    parent_id: 'ev_main',
                    branch_id: 'br_alt',
                    title: '侧巷',
                    summary: '铃声',
                    ts: '',
                    current: true,
                    head: true,
                  },
                ],
                branches: [
                  {
                    id: 'main',
                    head: 'ev_main',
                    title: '主线',
                    created_at: '',
                    current: false,
                  },
                  {
                    id: 'br_alt',
                    head: 'ev_alt',
                    from: 'main',
                    from_event: 'ev_main',
                    title: '支线',
                    created_at: '',
                    current: true,
                  },
                ],
              },
            }),
          )
        }
        return HttpResponse.json({
          story_id: 'st_1',
          branch_id: 'main',
          turns: [
            {
              id: 'ev_main',
              parent_id: null,
              branch_id: 'main',
              ts: '',
              user: '进入旧酒馆',
              narrative: '酒馆里只剩炉火。',
            },
          ],
          state: {
            on_stage: ['林川'],
            characters: {},
            events: [{ summary: '进入酒馆' }],
          },
          graph: {
            nodes: [
              {
                id: 'ev_main',
                branch_id: 'main',
                title: '进门',
                summary: '旧酒馆',
                ts: '',
                current: true,
                head: true,
              },
              {
                id: 'ev_alt',
                parent_id: 'ev_main',
                branch_id: 'br_alt',
                title: '侧巷',
                summary: '铃声',
                ts: '',
                current: false,
                head: true,
              },
            ],
            branches: [
              {
                id: 'main',
                head: 'ev_main',
                title: '主线',
                created_at: '',
                current: true,
              },
              {
                id: 'br_alt',
                head: 'ev_alt',
                from: 'main',
                from_event: 'ev_main',
                title: '支线',
                created_at: '',
                current: false,
              },
            ],
          },
        })
      }),
    )

    render(<InteractiveLayout />)

    expect(await screen.findByText('进入旧酒馆')).toBeInTheDocument()
    expect(screen.getByText('林川')).toBeInTheDocument()

    act(() => useInteractiveStore.getState().setSubmode('timeline'))
    fireEvent.click(await screen.findByText('侧巷'))
    await waitFor(() => expect(switchCalls).toBeGreaterThan(0))

    expect(screen.getAllByText('侧巷').length).toBeGreaterThan(0)
    expect(screen.getByTestId('branch-graph-canvas')).toHaveAttribute('data-edge-count', '1')

    releaseAltSnapshot()
    await waitFor(() => expect(snapshotBranches).toContain('br_alt'))

    fireEvent.click(screen.getByRole('button', { name: /返回剧情/ }))
    await screen.findByText('走向另一条巷子')
    expect(screen.getByText('巷尾传来铃声。')).toBeInTheDocument()
    await waitFor(() => expect(screen.queryByText('林川')).not.toBeInTheDocument())
    expect(screen.getByText('阿岚')).toBeInTheDocument()
    expect(screen.getByText('发现侧巷')).toBeInTheDocument()
  })

  it('keeps polling pending turn state until scene memory is ready', async () => {
    useInteractiveStore.setState({
      stories: [],
      tellers: [],
      branches: [],
      snapshot: null,
      currentStoryId: '',
      currentBranchId: 'main',
      submode: 'story',
    })
    let snapshotRequests = 0
    server.use(
      http.get('/api/interactive/stories', () =>
        HttpResponse.json({
          current_story_id: 'st_1',
          stories: [
            {
              id: 'st_1',
              title: '末日开端',
              origin: '',
              story_teller_id: 'classic',
              reply_target_chars: 1200,
              created_at: '',
              updated_at: '',
              branches: 1,
              events: 1,
            },
          ],
        }),
      ),
      http.get('/api/interactive/stories/:id/branches', () =>
        HttpResponse.json({
          branches: [
            {
              id: 'main',
              head: 'ev_1',
              title: '主线',
              created_at: '',
              current: true,
            },
          ],
        }),
      ),
      http.get('/api/interactive/stories/:id/snapshot', () => {
        snapshotRequests += 1
        const ready = snapshotRequests >= 2
        return HttpResponse.json({
          story_id: 'st_1',
          branch_id: 'main',
          turns: [
            {
              id: 'ev_1',
              parent_id: null,
              branch_id: 'main',
              ts: '',
              user: '点燃火把',
              narrative: '火光照亮了墙面。',
              state_status: ready ? 'ready' : 'pending',
              state_delta: ready ? { ops: [{ op: 'set', path: 'on_stage', value: ['林川'] }] } : undefined,
            },
          ],
          current_turn: {
            id: 'ev_1',
            parent_id: null,
            branch_id: 'main',
            ts: '',
            user: '点燃火把',
            narrative: '火光照亮了墙面。',
            state_status: ready ? 'ready' : 'pending',
            state_delta: ready ? { ops: [{ op: 'set', path: 'on_stage', value: ['林川'] }] } : undefined,
          },
          state: ready ? { on_stage: ['林川'], characters: {}, events: [] } : { on_stage: [], characters: {}, events: [] },
        })
      }),
    )

    render(<InteractiveLayout />)

    expect(await screen.findByText('同步中')).toBeInTheDocument()
    expect(await screen.findByText('林川', {}, { timeout: 3000 })).toBeInTheDocument()
    await waitFor(() => expect(screen.queryByText('同步中')).not.toBeInTheDocument())
    expect(snapshotRequests).toBeGreaterThanOrEqual(2)
  })

  it('opens the branch graph as a full workspace view from the timeline submode', async () => {
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
      http.get('/api/interactive/stories', () =>
        HttpResponse.json({
          current_story_id: 'st_1',
          stories: [
            {
              id: 'st_1',
              title: '末日开端',
              origin: '',
              story_teller_id: 'classic',
              reply_target_chars: 1200,
              created_at: '',
              updated_at: '',
              branches: 2,
              events: 2,
            },
          ],
        }),
      ),
      http.get('/api/interactive/stories/:id/branches', () =>
        HttpResponse.json({
          branches: [
            {
              id: 'main',
              head: 'ev_main',
              title: '主线',
              created_at: '',
              current: true,
            },
            {
              id: 'br_alt',
              head: 'ev_alt',
              from: 'main',
              from_event: 'ev_main',
              title: '支线',
              created_at: '',
              current: false,
            },
          ],
        }),
      ),
      http.get('/api/interactive/stories/:id/snapshot', () =>
        HttpResponse.json({
          story_id: 'st_1',
          branch_id: 'main',
          turns: [
            {
              id: 'ev_main',
              parent_id: null,
              branch_id: 'main',
              ts: '',
              user: '进入旧酒馆',
              narrative: '酒馆里只剩炉火。',
            },
          ],
          state: { on_stage: [], characters: {}, events: [] },
          graph: {
            nodes: [
              {
                id: 'ev_main',
                branch_id: 'main',
                title: '进门',
                summary: '旧酒馆',
                ts: '',
                current: true,
                head: true,
              },
              {
                id: 'ev_alt',
                parent_id: 'ev_main',
                branch_id: 'br_alt',
                title: '侧巷',
                summary: '铃声',
                ts: '',
                current: false,
                head: true,
              },
            ],
            branches: [
              {
                id: 'main',
                head: 'ev_main',
                title: '主线',
                created_at: '',
                current: true,
              },
              {
                id: 'br_alt',
                head: 'ev_alt',
                from: 'main',
                from_event: 'ev_main',
                title: '支线',
                created_at: '',
                current: false,
              },
            ],
          },
        }),
      ),
    )

    render(<InteractiveLayout />)

    expect(await screen.findByText('进入旧酒馆')).toBeInTheDocument()
    expect(screen.queryByTestId('branch-graph-canvas')).not.toBeInTheDocument()

    act(() => useInteractiveStore.getState().setSubmode('timeline'))

    expect(await screen.findByTestId('branch-graph-canvas')).toBeInTheDocument()
    expect(await screen.findByText('侧巷')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: '返回剧情' }))
    expect(await screen.findByText('故事舞台 · 当前分支 main')).toBeInTheDocument()
    expect(screen.queryByTestId('branch-graph-canvas')).not.toBeInTheDocument()
  })

  it('switches the route map when choosing another story in timeline view', async () => {
    const user = userEvent.setup()
    useInteractiveStore.setState({
      stories: [],
      tellers: [],
      branches: [],
      snapshot: null,
      currentStoryId: '',
      currentBranchId: 'main',
      submode: 'timeline',
    })
    const requestedSnapshots: string[] = []
    server.use(
      http.get('/api/interactive/stories', () =>
        HttpResponse.json({
          current_story_id: 'st_1',
          stories: [
            {
              id: 'st_1',
              title: '旧城线',
              origin: '',
              story_teller_id: 'classic',
              reply_target_chars: 1200,
              created_at: '',
              updated_at: '',
              branches: 1,
              events: 1,
            },
            {
              id: 'st_2',
              title: '雪山线',
              origin: '',
              story_teller_id: 'classic',
              reply_target_chars: 1200,
              created_at: '',
              updated_at: '',
              branches: 1,
              events: 1,
            },
          ],
        }),
      ),
      http.get('/api/interactive/stories/:id/branches', ({ params }) =>
        HttpResponse.json({
          branches: [
            {
              id: 'main',
              head: params.id === 'st_2' ? 'snow_1' : 'city_1',
              title: '主线',
              created_at: '',
              current: true,
            },
          ],
        }),
      ),
      http.get('/api/interactive/stories/:id/snapshot', ({ params }) => {
        requestedSnapshots.push(String(params.id))
        if (params.id === 'st_2') {
          return HttpResponse.json({
            story_id: 'st_2',
            branch_id: 'main',
            turns: [],
            state: {},
            graph: {
              branches: [
                {
                  id: 'main',
                  head: 'snow_1',
                  title: '主线',
                  created_at: '',
                  current: true,
                },
              ],
              nodes: [
                {
                  id: 'snow_1',
                  branch_id: 'main',
                  title: '雪山入口',
                  summary: '风雪封路',
                  ts: '',
                  current: true,
                  head: true,
                },
              ],
            },
          })
        }
        return HttpResponse.json({
          story_id: 'st_1',
          branch_id: 'main',
          turns: [],
          state: {},
          graph: {
            branches: [
              {
                id: 'main',
                head: 'city_1',
                title: '主线',
                created_at: '',
                current: true,
              },
            ],
            nodes: [
              {
                id: 'city_1',
                branch_id: 'main',
                title: '旧城开端',
                summary: '钟楼响起',
                ts: '',
                current: true,
                head: true,
              },
            ],
          },
        })
      }),
    )

    render(<InteractiveLayout />)

    expect(await screen.findByText('旧城开端')).toBeInTheDocument()
    expect(screen.getByLabelText('选择故事线')).toBeInTheDocument()

    await user.click(screen.getByLabelText('选择故事线'))
    await user.click(await screen.findByRole('option', { name: '雪山线' }))

    expect(await screen.findByText('雪山入口')).toBeInTheDocument()
    await waitFor(() => expect(screen.queryByText('旧城开端')).not.toBeInTheDocument())
    expect(requestedSnapshots).toContain('st_2')
  })
})
