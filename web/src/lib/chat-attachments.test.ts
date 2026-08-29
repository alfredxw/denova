import { describe, expect, it } from 'vitest'
import { chatAttachmentImageURL } from './chat-attachments'

describe('chat attachment URLs', () => {
  it('encodes project, owner scope, and attachment identity', () => {
    expect(chatAttachmentImageURL(
      'project one',
      { kind: 'session', id: 'session/one' },
      'att_0123456789abcdef0123456789abcdef',
    )).toBe(
      '/api/projects/project%20one/attachments/att_0123456789abcdef0123456789abcdef?scope=session&scope_id=session%2Fone',
    )
  })
})
