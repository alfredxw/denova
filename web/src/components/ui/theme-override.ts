import { createContext } from 'react'

/** Carries a local theme to overlays that render outside their trigger's DOM. */
export const ThemeOverrideContext = createContext<'light' | 'dark' | undefined>(undefined)
