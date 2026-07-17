import type { DirectorPlanRunStatus, DirectorPlanStatus } from '../../types'

export type DirectorStatusLike = Partial<DirectorPlanRunStatus & DirectorPlanStatus>

// 右栏状态面板的分区 tab：本回合变化 / 角色 / 世界与场景。
export type StatePanelTab = 'changes' | 'actors' | 'world'
