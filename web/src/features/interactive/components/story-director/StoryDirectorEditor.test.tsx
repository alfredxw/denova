import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { StoryDirector } from '../../types'
import { StoryDirectorEditor } from './StoryDirectorEditor'

describe('StoryDirectorEditor', () => {
  it('displays the legacy auto schema mode as adapt after opening', () => {
    const draft: StoryDirector = {
      version: 1,
      id: 'legacy-director',
      name: '旧导演',
      description: '',
      strategy: {
        enabled: true,
        state_schema_adaptation_mode: 'auto',
      },
      trpg_system: {},
      opening_selector: { enabled: true },
      custom: true,
    }

    render(
      <StoryDirectorEditor
        draft={draft}
        tellers={[]}
        eventPackages={[]}
        ruleSystems={[]}
        actorStates={[]}
        imagePresets={[]}
        setDraft={vi.fn()}
      />,
    )

    expect(screen.getByText('首轮后动态适配')).toBeInTheDocument()
    expect(screen.queryByText('自定义（auto）')).not.toBeInTheDocument()
  })
})
