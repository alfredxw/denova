export interface ProductMessage {
  id: string
  type: 'changelog' | string
  title: string
  summary?: string
  body: string
  published_at?: string
  read_at?: string
}

export interface ProductMessageList {
  items: ProductMessage[]
  unread_count: number
}
