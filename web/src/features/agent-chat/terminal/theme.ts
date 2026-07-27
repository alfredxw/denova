import type { ITheme } from '@xterm/xterm'

/**
 * xterm needs concrete colors, so the workbench palette is read from the live CSS
 * variables instead of being duplicated here. This keeps the terminal in sync with
 * both the light and dark themes, including user-selected presets.
 */

interface TerminalPaletteFallback {
  background: string
  foreground: string
  muted: string
  border: string
}

const DARK_FALLBACK: TerminalPaletteFallback = {
  background: '#141412',
  foreground: '#e8e8e2',
  muted: '#bdbdb7',
  border: '#2a2a27',
}

const LIGHT_FALLBACK: TerminalPaletteFallback = {
  background: '#fbfbf8',
  foreground: '#1b1b19',
  muted: '#595954',
  border: '#e0e0da',
}

/** ANSI colors tuned for the workbench's muted palette in each theme. */
const DARK_ANSI = {
  black: '#3a3a36', red: '#d37a73', green: '#8eae98', yellow: '#c49a58',
  blue: '#7f9dc0', magenta: '#b18ec2', cyan: '#79aeae', white: '#d8d8d2',
  brightBlack: '#6d6d66', brightRed: '#e2938c', brightGreen: '#a6c4af',
  brightYellow: '#d9b478', brightBlue: '#9db8d6', brightMagenta: '#c7a8d5',
  brightCyan: '#96c5c5', brightWhite: '#f3f3ee',
} as const

const LIGHT_ANSI = {
  black: '#3c3c38', red: '#963f39', green: '#2f6944', yellow: '#8a611f',
  blue: '#385378', magenta: '#6a4380', cyan: '#26605f', white: '#8c8c85',
  brightBlack: '#6a6a63', brightRed: '#b4544d', brightGreen: '#417f57',
  brightYellow: '#a3752b', brightBlue: '#4a6b96', brightMagenta: '#82589a',
  brightCyan: '#357a78', brightWhite: '#1b1b19',
} as const

/** Build the xterm theme for the currently applied workbench theme. */
export function terminalTheme(dark: boolean): ITheme {
  const fallback = dark ? DARK_FALLBACK : LIGHT_FALLBACK
  const background = cssVariable('--nova-bg', fallback.background)
  const foreground = cssVariable('--nova-text', fallback.foreground)
  return {
    background,
    foreground,
    cursor: foreground,
    cursorAccent: background,
    selectionBackground: cssVariable('--nova-active', fallback.border),
    selectionInactiveBackground: cssVariable('--nova-hover', fallback.border),
    ...(dark ? DARK_ANSI : LIGHT_ANSI),
  }
}

/**
 * Resolve a CSS custom property to a usable color string. Variables that indirect
 * through other variables resolve at computed-style time, so the returned value is
 * always a literal color unless the variable is missing entirely.
 */
function cssVariable(name: string, fallback: string): string {
  if (typeof window === 'undefined') return fallback
  const value = window.getComputedStyle(document.documentElement).getPropertyValue(name).trim()
  return value || fallback
}
