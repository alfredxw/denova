import { describe, expect, it } from 'vitest'
import { diagnosticText, errorMessage } from './error-diagnostics'

describe('screenshot diagnostics', () => {
  it('keeps the cause, operation, identities and server environment together', () => {
    const message = errorMessage({ error: 'Approval failed', code: 'agent_runtime.definition_mismatch', request_id: 'request-1', run_id: 'run-1',
      details: { detail: 'execution definition changed', operation: 'agent.interaction.resolve', backend_version: '0.5.0', platform: 'windows/amd64' } })
    for (const value of ['Approval failed', 'definition changed', 'agent.interaction.resolve', 'agent_runtime.definition_mismatch', 'request-1', 'run-1', '0.5.0', 'windows/amd64']) expect(message).toContain(value)
  })

  it('does not dump unrelated payloads or expose credentials and user paths', () => {
    const message = errorMessage({ message: 'Failed', payload: { document: 'private prose' }, details: { detail: 'open C:\\Users\\alice\\Novel\\draft.md: denied; api_key=private-key', snapshot: 'private prose' } })
    expect(message).toContain('denied')
    for (const value of ['alice', 'private-key', 'private prose']) expect(message).not.toContain(value)
    expect(diagnosticText('Authorization: Bearer private-token')).not.toContain('private-token')
    expect(diagnosticText('{"Authorization":"Bearer private-token"}')).not.toContain('private-token')
  })
})
