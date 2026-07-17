import type { DirectorPlanRunStatus, DirectorPlanStatus } from '../../types'

export type ConsoleTab = 'state' | 'director'
export type DirectorStatusLike = Partial<DirectorPlanRunStatus & DirectorPlanStatus>
