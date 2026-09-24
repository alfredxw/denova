import { describe, expect, it } from 'vitest'
import { buildConfigurationAgentMessage } from './configuration-message'
import { buildDevelopmentMessage } from '@/features/platform/development-context'

describe('buildConfigurationAgentMessage', () => {
  it('invokes the shared skill once and appends stable bounded page provenance', () => {
    const context = Object.fromEntries(Array.from({ length: 26 }, (_, index) => [
      `field-${String(index).padStart(2, '0')}`,
      index === 0 ? 'x'.repeat(2200) : `value-${index}`,
    ]))

    const message = buildConfigurationAgentMessage('Update the current resource.', {
      origin: 'skills',
      resourceId: 'demo',
      context,
    })

    expect(message.match(/\/configuration/g)).toHaveLength(1)
    expect(message).toContain('[Configuration Page Context]')
    const metadata = JSON.parse(message.slice(message.indexOf('{'))) as {
      source: string
      origin: string
      resource_id: string
      page_context: Record<string, string>
    }
    expect(metadata).toMatchObject({
      source: 'Denova configuration page',
      origin: 'skills',
      resource_id: 'demo',
    })
    expect(Object.keys(metadata.page_context)).toHaveLength(24)
    expect(Object.keys(metadata.page_context)).toEqual([...Object.keys(metadata.page_context)].sort())
    expect(metadata.page_context['field-00']).toHaveLength(2048)
  })

  it('does not duplicate an explicit invocation', () => {
    const message = buildConfigurationAgentMessage('/configuration\n\nReview this.', { origin: 'agents' })
    expect(message.match(/\/configuration/g)).toHaveLength(1)
  })

  it('leaves built-in conversation commands untouched', () => {
    expect(buildConfigurationAgentMessage('/clear', { origin: 'lore' })).toBe('/clear')
  })

  it('routes extension work to development guidance and retains the latest bounded feedback', () => {
    const source = { developmentId: 'source', projectId: 'project', projectName: 'Project', kind: 'game' as const, relativePath: '.' }
    const message = buildDevelopmentMessage('Repair this game.', source, 'x'.repeat(20000) + 'latest failure')
    expect(message).toContain('/extension-development')
    expect(message).not.toContain('/configuration\n')
    const metadata = JSON.parse(message.slice(message.indexOf('{')))
    expect(metadata.source).toBe('Denova workbench')
    expect(metadata.source_directory).toBe('.')
    expect(metadata.latest_development_feedback).toMatch(/^\[Earlier output omitted\]/)
    expect(metadata.latest_development_feedback).toMatch(/latest failure$/)
    expect(metadata.latest_development_feedback.length).toBeLessThanOrEqual(16384)
    expect(buildDevelopmentMessage('/status', source)).toBe('/status')
  })
})
