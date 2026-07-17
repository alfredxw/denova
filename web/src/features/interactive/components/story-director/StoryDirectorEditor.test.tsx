import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { StoryDirector } from '../../types'
import { StoryDirectorEditor } from './StoryDirectorEditor'

describe('StoryDirectorEditor', () => {
  it('displays the configured after-opening schema mode', () => {
    const draft: StoryDirector = {
      version: 1,
		id: 'custom-director',
		name: '自定义导演',
      description: '',
      strategy: {
        enabled: true,
		state_schema_adaptation_mode: 'after_opening',
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

	expect(screen.getByText('首轮后动态适配')).toBeInTheDocument()
  })
})
