import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from '@/i18n'
import type { ResolvedAgentContextSettings } from '@/features/settings/types'
import { AgentCheckpointSection } from './AgentCheckpointSection'

const resolvedContext: ResolvedAgentContextSettings = {
  compaction_enabled: true,
  compaction_threshold: 0.85,
  checkpoint_guidance: 'Inherited preference.',
  tool_result_context_enabled: true,
  max_fragment_bytes: 262144,
  max_total_injected_bytes: 4194304,
  max_fragments: 256,
  max_metadata_field_bytes: 4096,
  max_provider_input_bytes: 4194304,
}

describe('AgentCheckpointSection', () => {
  beforeEach(async () => {
    await i18next.changeLanguage('en-US')
  })

  it('shows runtime-owned sources and edits only checkpoint guidance', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(
      <AgentCheckpointSection
        value={{}}
        resolved={resolvedContext}
        sources={[
          { id: 'runtime_contract', title: 'Runtime', source: 'Denova runtime', content: 'Do not call tools.' },
          { id: 'checkpoint_schema', title: 'Schema', source: 'Denova runtime', content: '## Goal' },
          { id: 'domain_rules', title: 'Domain', source: 'Denova runtime', content: 'Workspace requirements.' },
        ]}
        onChange={onChange}
      />,
    )

    const runtime = screen.getByRole('button', { name: /Runtime Compaction Protocol/ })
    expect(runtime).toHaveAttribute('aria-expanded', 'false')
    await user.click(runtime)
    expect(runtime).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByText('Do not call tools.')).toBeVisible()

    const guidance = screen.getByRole('textbox', { name: 'Checkpoint Retention Preferences' })
    expect(guidance).toHaveValue('Inherited preference.')
    expect(guidance).toHaveAttribute('maxlength', '2000')
    fireEvent.change(guidance, { target: { value: 'Preserve exact test results.' } })
    expect(onChange).toHaveBeenLastCalledWith({ checkpoint_guidance: 'Preserve exact test results.' })

    fireEvent.change(guidance, { target: { value: 'x'.repeat(1001) } })
    expect(onChange).toHaveBeenLastCalledWith({ checkpoint_guidance: 'x'.repeat(1000) })
  })

  it('treats guidance omitted by an older backend snapshot as empty', () => {
    const legacyResolvedContext = { ...resolvedContext }
    delete legacyResolvedContext.checkpoint_guidance
    render(
      <AgentCheckpointSection
        value={{}}
        resolved={legacyResolvedContext}
        onChange={vi.fn()}
      />,
    )

    expect(screen.getByRole('textbox', { name: 'Checkpoint Retention Preferences' })).toHaveValue('')
  })
})
