import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { patchProjectSettings } from '@/features/settings/api'
import { toast } from '@/lib/toast'
import { StoryPresentationControls } from './StoryPresentationControls'

vi.mock('@/features/settings/api', () => ({ patchProjectSettings: vi.fn(), patchSettings: vi.fn() }))
vi.mock('../story-stage/use-stage-preferences', () => ({
  useStagePreferences: () => ({ scrimOpacity: 0.75, textMaxWidth: 896, lineHeight: 1.78, characterLayout: 'center', characterSize: 0.7 }),
}))
vi.mock('./StoryBackgroundSelect', () => ({ StoryBackgroundSelect: () => null }))
vi.mock('@/lib/toast', () => ({ toast: { error: vi.fn() } }))

describe('StoryPresentationControls text width', () => {
  beforeEach(() => vi.clearAllMocks())

  it.each(['Enter', 'blur'])('saves a user preference on %s without changing the story', async (commit) => {
    vi.mocked(patchProjectSettings).mockResolvedValue({ effective: { interactive_stage_text_max_width: 1120 } } as Awaited<ReturnType<typeof patchProjectSettings>>)
    const onChange = vi.fn()
    render(<StoryPresentationControls projectId="project-1" disabled onChange={onChange} />)
    const input = screen.getByRole('spinbutton', { name: '文本最大宽度（px）' })
    expect(input).toBeEnabled()
    fireEvent.change(input, { target: { value: '1120' } })
    if (commit === 'Enter') fireEvent.keyDown(input, { key: 'Enter' })
    else fireEvent.blur(input)
    await waitFor(() => expect(patchProjectSettings).toHaveBeenCalledWith('project-1', 'user', { interactive_stage_text_max_width: 1120 }))
    await waitFor(() => expect(input).toBeEnabled())
    expect(onChange).not.toHaveBeenCalled()
  })

  it('rejects out-of-range edits and restores the saved width on failure', async () => {
    vi.mocked(patchProjectSettings).mockRejectedValue(new Error('Save failed'))
    render(<StoryPresentationControls projectId="project-1" disabled={false} onChange={vi.fn()} />)
    const input = screen.getByRole('spinbutton', { name: '文本最大宽度（px）' })
    for (const value of ['479', '1601', '720.5', '']) {
      fireEvent.change(input, { target: { value } })
      fireEvent.blur(input)
      expect(input).toHaveAttribute('aria-invalid', 'true')
    }
    expect(patchProjectSettings).not.toHaveBeenCalled()
    fireEvent.change(input, { target: { value: '720' } })
    fireEvent.blur(input)
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith('保存舞台显示设置失败'))
    await waitFor(() => expect(input).toHaveValue(896))
    expect(input).toBeEnabled()
  })
})
