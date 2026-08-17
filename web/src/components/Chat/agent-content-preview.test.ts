import { describe, expect, it } from 'vitest'
import { agentContentPreview } from './agent-content-preview'

describe('agentContentPreview', () => {
  it('uses the first meaningful line and normalizes its whitespace', () => {
    expect(agentContentPreview('\n  Now   I need to inspect the timeline.  \nThe next line stays hidden.')).toBe(
      'Now I need to inspect the timeline.',
    )
  })

  it('supports Windows line endings and empty content', () => {
    expect(agentContentPreview('\r\n\t第一行思考\r\n第二行思考')).toBe('第一行思考')
    expect(agentContentPreview(' \n\t')).toBe('')
  })
})
