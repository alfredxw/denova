export type ConversationGoalStatus = 'active' | 'paused' | 'completed' | 'blocked' | 'cleared'

export interface ConversationGoal {
  id: string
  objective: string
  status: ConversationGoalStatus
  revision: number
  report?: string
  created_at: string
  updated_at: string
  active_since?: string
  active_duration_millis?: number
}

export type ConversationGoalAction = 'set' | 'pause' | 'resume' | 'clear'
