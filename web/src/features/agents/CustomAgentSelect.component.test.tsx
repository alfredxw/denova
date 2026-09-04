import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { settingsQueryKeys } from '@/features/settings/query'
import type { AgentRuntimeKind } from '@/features/settings/types'
import { CustomAgentSelect } from './CustomAgentSelect'

describe('CustomAgentSelect', () => {
  it.each([
    ['ide', '写作 Agent'],
    ['interactive_story', '游戏 Agent'],
  ] satisfies Array<[AgentRuntimeKind, string]>)('shows the actual %s Agent name', (runtimeKind, expectedName) => {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: Infinity } },
    })
    queryClient.setQueryData(settingsQueryKeys.global(), {
      effective: { custom_agents: [] },
    })

    render(
      <QueryClientProvider client={queryClient}>
        <CustomAgentSelect runtimeKind={runtimeKind} value="" onValueChange={vi.fn()} />
      </QueryClientProvider>,
    )

    const select = screen.getByRole('combobox', { name: 'Agent' })
    expect(select).toHaveTextContent(expectedName)
    expect(select).not.toHaveTextContent('内置')
  })
})
