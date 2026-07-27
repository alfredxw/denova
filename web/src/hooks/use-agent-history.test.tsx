import { StrictMode, useEffect, useRef, type PropsWithChildren } from 'react'
import { act, renderHook, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { writingAgentChatClient, type AgentChatClient } from './agent-chat-client'
import { useWritingAgentHistory } from './use-agent-history'

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((done) => {
    resolve = done
  })
  return { promise, resolve }
}

const strictMode = ({ children }: PropsWithChildren) => <StrictMode>{children}</StrictMode>

describe('useWritingAgentHistory', () => {
  it('keeps the first authoritative load alive through React Strict Mode effect replay', async () => {
    const page = deferred<Awaited<ReturnType<AgentChatClient['getMessagesPage']>>>()
    const setMessages = vi.fn()
    const getMessagesPage = vi.fn(() => page.promise)
    const client = {
      ...writingAgentChatClient,
      fixedSessionId: 'session-a',
      getMessagesPage,
    }

    renderHook(() => {
      const history = useWritingAgentHistory({ client, setMessages })
      const initializedRef = useRef(false)
      useEffect(() => {
        if (initializedRef.current) return
        initializedRef.current = true
        void history.loadHistory('session-a')
      }, [history.loadHistory])
      return history
    }, { wrapper: strictMode })

    await act(async () => {
      page.resolve({
        messages: [{
          id: 'message-a',
          role: 'user',
          parts: [{ type: 'text', text: 'hello' }],
        }],
        nextBefore: '0',
        hasMore: false,
        total: 1,
      })
      await page.promise
    })

    await waitFor(() => expect(setMessages).toHaveBeenCalledWith([
      {
        id: 'message-a',
        role: 'user',
        parts: [{ type: 'text', text: 'hello' }],
      },
    ]))
    expect(getMessagesPage).toHaveBeenCalledTimes(1)
  })
})
