import type { DirectorPlanRunStatus, DirectorPlanStatus } from '../../types'

export type ConsoleTab = 'run' | 'state' | 'memory' | 'plan'
export type DirectorStatusLike = Partial<DirectorPlanRunStatus & DirectorPlanStatus>

export const allStructuresId = '__all__'
