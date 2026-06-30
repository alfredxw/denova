import { requestJSON } from '@/lib/api-client'
import type { ProductMessage, ProductMessageList } from './types'

export async function getMessages(): Promise<ProductMessageList> {
  return requestJSON('/api/messages')
}

export async function markMessageRead(id: string): Promise<ProductMessage> {
  return requestJSON(`/api/messages/${encodeURIComponent(id)}/read`, { method: 'POST' })
}

export async function markAllMessagesRead(): Promise<ProductMessageList> {
  return requestJSON('/api/messages/read-all', { method: 'POST' })
}
