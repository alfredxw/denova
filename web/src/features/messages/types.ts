export interface ProductMessage {
  id: string
  type: 'changelog' | string
  title: string
  summary?: string
  body: string
  published_at?: string
  read_at?: string
  task_id?: string
  run_id?: string
  inbox_id?: string
  workspace?: string
  status?: string
  action_required?: boolean
}

export interface ProductMessageList {
  items: ProductMessage[]
  unread_count: number
}

export interface AutomationMessageNavigation {
  taskId: string
  runId?: string
  inboxId?: string
  workspace?: string
}
