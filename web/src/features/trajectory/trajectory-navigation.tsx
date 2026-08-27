import { createContext, useContext } from 'react'
import type { ReactNode } from 'react'

export interface TrajectoryNavigationTarget {
  projectId: string
  runId: string
}

export interface TrajectoryNavigationIntent extends TrajectoryNavigationTarget {
  nonce: number
}

interface TrajectoryNavigationContextValue {
  enabled: boolean
  intent: TrajectoryNavigationIntent | null
  open: (target: TrajectoryNavigationTarget) => void
}

const disabledTrajectoryNavigation: TrajectoryNavigationContextValue = {
  enabled: false,
  intent: null,
  open: () => undefined,
}

const TrajectoryNavigationContext = createContext<TrajectoryNavigationContextValue>(disabledTrajectoryNavigation)

export function TrajectoryNavigationProvider({ value, children }: {
  value: TrajectoryNavigationContextValue
  children: ReactNode
}) {
  return <TrajectoryNavigationContext.Provider value={value}>{children}</TrajectoryNavigationContext.Provider>
}

export function useTrajectoryNavigation() {
  return useContext(TrajectoryNavigationContext)
}
