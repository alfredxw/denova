import { act, cleanup, render } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { StoryStageArtwork } from './StoryStageArtwork'
import type { PresentationMaterial, TurnEvent } from '../../types'

const pending: Array<{ src: string; onload: (() => void) | null; onerror: (() => void) | null }> = []
class ImageLoader {
  src = ''
  onload: (() => void) | null = null
  onerror: (() => void) | null = null
  decode = () => Promise.resolve()
  constructor() { pending.push(this) }
}
const material = (item: string, asset: string): PresentationMaterial => ({ item_id: item, asset_id: asset, path: `assets/${asset}.png`, name: asset })
const turn = (id: string, parent: string | null, asset: string): TurnEvent => ({
  id, parent_id: parent, branch_id: 'main', ts: '', user: 'Continue', narrative: 'A scene.',
  turn_result: { state_updates: [], choices: [], presentation: { background: material('station', asset), characters: [material('hero', asset)] } },
})
const loadAll = () => act(async () => { for (const image of pending.splice(0)) image.onload?.() })

describe('StoryStageArtwork', () => {
  beforeEach(() => { pending.length = 0; vi.stubGlobal('Image', ImageLoader) })
  afterEach(() => { cleanup(); vi.unstubAllGlobals(); vi.restoreAllMocks() })

  it('retains each previous slot during loading or failure and switches successful siblings', async () => {
    const props = { projectId: 'project', latest: true, textHidden: false, scrimOpacity: 0.75 }
    const { container, rerender } = render(<StoryStageArtwork {...props} turn={turn('one', null, 'day')} />)
    expect(container.querySelectorAll('img')).toHaveLength(0)
    await loadAll()
    expect(container.querySelectorAll('img')).toHaveLength(2)
    rerender(<StoryStageArtwork {...props} turn={turn('two', 'plan-event-after-one', 'night')} previousTurnId="one" />)
    expect(container.querySelector('[data-stage-layer="background"]')).toHaveAttribute('alt', 'day')
    vi.spyOn(console, 'warn').mockImplementation(() => {})
    await act(async () => { pending[0].onerror?.(); pending[1].onload?.() })
    expect(container.querySelector('[data-stage-layer="background"]')).toHaveAttribute('alt', 'day')
    expect(container.querySelector('[data-stage-layer="character"]')).toHaveAttribute('alt', 'night')
    rerender(<StoryStageArtwork {...props} turn={turn('two', 'plan-event-after-one', 'night')} previousTurnId="one" textHidden />)
    expect(container.querySelector('[data-testid="story-stage-scrim"]')).toHaveStyle({ opacity: '0' })
    rerender(<StoryStageArtwork {...props} turn={turn('two', 'plan-event-after-one', 'night')} previousTurnId="one" settings={{ background: false, characters: false }} />)
    expect(container.querySelectorAll('img')).toHaveLength(0)
  })

  it('clears fallback on history, regeneration, branch and project changes, ignoring late loads', async () => {
    const props = { projectId: 'project', latest: true, textHidden: false, scrimOpacity: 0.75 }
    const { container, rerender } = render(<StoryStageArtwork key="main" {...props} turn={turn('one', null, 'day')} />)
    await loadAll()
    rerender(<StoryStageArtwork key="main" {...props} turn={turn('two', 'plan-event-after-one', 'night')} previousTurnId="one" />)
    const lateLoad = pending[0].onload
    rerender(<StoryStageArtwork key="main" {...props} turn={turn('replacement', 'one', 'snow')} previousTurnId="one" />)
    expect(container.querySelectorAll('img')).toHaveLength(0)
    await act(async () => { lateLoad?.() })
    expect(container.querySelectorAll('img')).toHaveLength(0)
    await loadAll()
    rerender(<StoryStageArtwork key="main" {...props} turn={turn('one', null, 'day')} latest={false} />)
    expect(container.querySelectorAll('img')).toHaveLength(0)
    await loadAll()
    rerender(<StoryStageArtwork key="branch" {...props} turn={turn('one', null, 'day')} />)
    expect(container.querySelectorAll('img')).toHaveLength(0)
    await loadAll()
    rerender(<StoryStageArtwork key="another-project" {...props} projectId="another" turn={turn('one', null, 'day')} />)
    expect(container.querySelectorAll('img')).toHaveLength(0)
  })
})
