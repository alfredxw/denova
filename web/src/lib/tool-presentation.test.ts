import { describe, expect, it } from 'vitest'
import { readToolPresentation, toolCallRenderer, toolPresentationKind, toolResultRenderer } from './tool-presentation'

describe('tool presentation contract', () => {
  it('accepts only the exhaustive backend vocabulary', () => {
    expect(readToolPresentation({ call: 'todo', result: 'todo' })).toEqual({ call: 'todo', result: 'todo' })
    expect(readToolPresentation({ call: 'future', result: 'future' })).toBeUndefined()
    expect(readToolPresentation({ call: 'todo' })).toBeUndefined()
  })

  it('does not infer a specialized renderer from the tool name', () => {
    expect(toolPresentationKind({ role: 'tool_call', name: 'todo' }, 'call')).toBe('generic')
    expect(toolPresentationKind({ role: 'tool_call', name: 'custom', tool_presentation: { call: 'todo', result: 'todo' } }, 'call')).toBe('todo')
    expect(toolCallRenderer({ role: 'tool_call', name: 'todo' })).toBe('generic')
    expect(toolCallRenderer({ role: 'tool_call', name: 'custom', tool_presentation: { call: 'todo', result: 'todo' } })).toBe('todo')
    expect(toolResultRenderer({ role: 'tool_result', name: 'custom', tool_presentation: { call: 'image', result: 'image' } })).toBe('image')
  })
})
