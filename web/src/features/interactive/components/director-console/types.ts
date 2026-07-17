import type { DirectorPlanRunStatus, DirectorPlanStatus } from '../../types'

export type ConsoleTab = 'run' | 'state' | 'plan'
export type DirectorStatusLike = Partial<DirectorPlanRunStatus & DirectorPlanStatus>
