// StateView still owns these focused projections; the console combines them in overview.
export type StatePanelTab = 'changes' | 'actors' | 'world'
export type StatePanelSection = StatePanelTab | 'overview'
export type DirectorConsoleTab = 'overview' | 'tuning' | 'routes'
