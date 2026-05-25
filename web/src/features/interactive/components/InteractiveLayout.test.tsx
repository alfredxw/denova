import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { http, HttpResponse } from 'msw'
import { InteractiveLayout } from './InteractiveLayout'
import { useInteractiveStore } from '../stores/interactive-store'
import { server } from '@/test/msw/server'

describe('InteractiveLayout', () => {
  it('renders story stage and snapshot panels', async () => {
    const { container } = render(<InteractiveLayout />)

    expect(await screen.findByText('故事舞台 · 当前分支 main')).toBeInTheDocument()
    expect(screen.getByText('当前快照')).toBeInTheDocument()
    expect(container.querySelector('[data-slot="select-trigger"]')).toBeInTheDocument()
    expect(container.querySelector('[data-slot="button"]')).toBeInTheDocument()
    expect(container.querySelector('[data-slot="tabs-list"]')).toBeInTheDocument()
    expect(screen.getByTestId('interactive-shell')).toHaveClass('rounded-xl')
    expect(screen.getByTestId('story-stage-card')).toHaveClass('rounded-xl')
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
})
