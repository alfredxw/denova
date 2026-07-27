import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { Tabs, TabsContent, TabsList, TabsTrigger } from './tabs'

describe('tabs primitive', () => {
  it('styles the Radix active-state attribute used by the selected trigger', () => {
    render(
      <Tabs value="current">
        <TabsList>
          <TabsTrigger value="current">Current</TabsTrigger>
          <TabsTrigger value="other">Other</TabsTrigger>
        </TabsList>
        <TabsContent value="current">Current content</TabsContent>
        <TabsContent value="other">Other content</TabsContent>
      </Tabs>,
    )

    const current = screen.getByRole('tab', { name: 'Current' })
    const other = screen.getByRole('tab', { name: 'Other' })
    expect(current).toHaveAttribute('data-state', 'active')
    expect(current).toHaveAttribute('aria-selected', 'true')
    expect(current.className).toContain('data-[state=active]:bg-background')
    expect(other).toHaveAttribute('data-state', 'inactive')
  })
})
