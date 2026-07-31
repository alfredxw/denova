import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { ContextUsageIndicator, latestContextUsage } from './ContextUsageIndicator'

describe('ContextUsageIndicator', () => {
  it('uses the paired root context prompt instead of the aggregated prompt total', () => {
    expect(latestContextUsage([{
      role: 'token_usage',
      context_window_tokens: 1000,
      context_prompt_tokens: 700,
      prompt_tokens: 1300,
      model_calls: 2,
      usage_calls: [
        { index: 1, prompt_tokens: 600 },
        { index: 2, prompt_tokens: 700 },
      ],
    }])).toEqual({
      promptTokens: 700,
      contextWindowTokens: 1000,
      ratio: 0.7,
    })
  })

  it('does not treat an ambiguous multi-call aggregate as context occupancy', () => {
    expect(latestContextUsage([{
      role: 'token_usage',
      context_window_tokens: 1000,
      prompt_tokens: 1300,
      model_calls: 2,
    }])).toBeNull()
  })

  it('does not fall back to stale usage when the latest matching run is incomplete', () => {
    expect(latestContextUsage([
      {
        role: 'token_usage',
        agent_kind: 'ide',
        context_window_tokens: 1000,
        context_prompt_tokens: 600,
      },
      {
        role: 'token_usage',
        agent_kind: 'ide',
        context_window_tokens: 1000,
      },
    ], 'ide')).toBeNull()
  })

  it('keeps a later Game Director run from replacing the story Agent usage', () => {
    expect(latestContextUsage([
      {
        role: 'token_usage',
        agent_kind: 'interactive_story',
        context_window_tokens: 2000,
        context_prompt_tokens: 500,
        usage_calls: [{ prompt_tokens: 500 }],
      },
      {
        role: 'token_usage',
        agent_kind: 'interactive_director',
        context_window_tokens: 1000,
        context_prompt_tokens: 900,
        usage_calls: [{ prompt_tokens: 900 }],
      },
    ], 'interactive_story')).toEqual({
      promptTokens: 500,
      contextWindowTokens: 2000,
      ratio: 0.25,
    })
  })

  it('shows the percentage and opens current context details', async () => {
    const user = userEvent.setup()
    const onOpenDetails = vi.fn()
    render(
      <ContextUsageIndicator
        messages={[{
          role: 'token_usage',
          context_window_tokens: 2000,
          context_prompt_tokens: 1800,
          model_calls: 1,
          usage_calls: [{ prompt_tokens: 1800 }],
        }]}
        onOpenDetails={onOpenDetails}
      />,
    )

    const indicator = screen.getByRole('button', { name: /90%/ })
    expect(indicator).toHaveTextContent('90%')
    expect(indicator).toHaveClass('text-[var(--nova-warning)]')

    await user.click(indicator)
    expect(onOpenDetails).toHaveBeenCalledTimes(1)
  })

  it.each([
    { promptTokens: 896, percent: '90%', expectedClass: 'text-[var(--nova-warning)]' },
    { promptTokens: 996, percent: '100%', expectedClass: 'text-[var(--nova-danger)]' },
    { promptTokens: 1200, percent: '120%', expectedClass: 'text-[var(--nova-danger)]' },
  ])('keeps rounded $percent labels aligned with their severity', ({ promptTokens, percent, expectedClass }) => {
    render(
      <ContextUsageIndicator
        messages={[{
          role: 'token_usage',
          context_window_tokens: 1000,
          context_prompt_tokens: promptTokens,
        }]}
      />,
    )

    expect(screen.getByRole('status', { name: new RegExp(percent) })).toHaveClass(expectedClass)
  })
})
