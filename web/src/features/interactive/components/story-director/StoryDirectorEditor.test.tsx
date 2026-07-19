import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { StoryDirector } from '../../types'
import { StoryDirectorEditor } from './StoryDirectorEditor'

describe('StoryDirectorEditor', () => {
  it('does not expose story schema policy as a director setting', () => {
    const draft: StoryDirector = {
      version: 1,
		id: 'custom-director',
		name: '自定义导演',
      description: '',
      strategy: {
        enabled: true,
      },
      trpg_system: {},
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

	expect(screen.queryByText('首轮后动态适配')).not.toBeInTheDocument()
  })
})
