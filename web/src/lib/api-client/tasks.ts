import { requestJSON } from './client'

export type TaskCenterTaskType = 'agent' | 'automation' | 'interactive_story' | 'image_generation' | 'import_export'
export type TaskCenterStatus = 'running' | 'waiting_user' | 'failed' | 'completed' | 'stopped'
export type TaskCenterRecoveryKind = 'automation_run' | 'automation_inbox' | 'agent_session' | 'config_manager' | 'interactive_story' | 'image_generation' | 'import_export'

export interface TaskCenterTask {
  id: string
  type: TaskCenterTaskType
  status: TaskCenterStatus
  title: string
  project: {
    name: string
    path: string
  }
  started_at: string
  updated_at: string
  recovery: {
    kind: TaskCenterRecoveryKind
    workspace: string
    task_id?: string
    session_id?: string
    origin?: string
    resource_id?: string
    story_id?: string
    branch_id?: string
    run_id?: string
    inbox_id?: string
  }
  error?: string
}

export interface TaskCenterResult {
  tasks: TaskCenterTask[]
  action_required_count: number
}

export async function getTasks(): Promise<TaskCenterResult> {
  const data = await requestJSON<Partial<TaskCenterResult>>('/api/tasks')
  return {
    tasks: data.tasks || [],
    action_required_count: data.action_required_count || 0,
  }
}
