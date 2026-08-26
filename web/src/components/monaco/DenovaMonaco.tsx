import {
  DiffEditor as MonacoDiffEditor,
  Editor as MonacoEditor,
  type DiffEditorProps,
  type EditorProps,
  type Monaco,
} from '@monaco-editor/react'
import { useTheme } from 'next-themes'
import { useCallback, useMemo, useSyncExternalStore } from 'react'
import type { editor } from 'monaco-editor'
import { getSourceEditorFontFamily, subscribeSourceEditorFont } from '@/features/settings/source-editor-font'
import { getContentFontScale, subscribeContentFontScale } from '@/features/settings/content-font-scale'

export const DENOVA_MONACO_THEME_DARK = 'denova-dark'
export const DENOVA_MONACO_THEME_LIGHT = 'denova-light'

const REQUIRED_ALLOWED_LOCALES = {
  'zh-hans': true,
  'zh-hant': true,
  ja: true,
} as const

/**
 * Keeps Monaco's spoofing protection enabled while treating Chinese and
 * Japanese typography as expected content in every Denova authoring surface.
 */
export const DENOVA_MONACO_UNICODE_HIGHLIGHT = {
  nonBasicASCII: false,
  invisibleCharacters: true,
  ambiguousCharacters: true,
  allowedLocales: {
    _os: true,
    _vscode: true,
    ...REQUIRED_ALLOWED_LOCALES,
  },
} satisfies editor.IUnicodeHighlightOptions

type DenovaMonacoEditorProps = Omit<EditorProps, 'theme'>
type DenovaMonacoDiffEditorProps = Omit<DiffEditorProps, 'theme'>

/** Monaco editor with Denova's shared visual language and Unicode policy. */
export function DenovaMonacoEditor({ beforeMount, options, ...props }: DenovaMonacoEditorProps) {
  const theme = useDenovaMonacoTheme()
  const sourceEditorFontFamily = useSourceEditorFontFamily()
  const contentFontScale = useContentFontScale()
  const handleBeforeMount = useDenovaBeforeMount(beforeMount)
  const denovaOptions = useMemo<editor.IStandaloneEditorConstructionOptions>(() => ({
    ...options,
    fontFamily: options?.fontFamily || sourceEditorFontFamily,
    fontSize: options?.fontSize ?? contentFontScale.sourceEditor,
    unicodeHighlight: denovaUnicodeHighlight(options?.unicodeHighlight),
  }), [contentFontScale.sourceEditor, options, sourceEditorFontFamily])

  return (
    <MonacoEditor
      {...props}
      theme={theme}
      beforeMount={handleBeforeMount}
      options={denovaOptions}
    />
  )
}

/** Monaco diff editor sharing the exact theme and safeguards of regular editors. */
export function DenovaMonacoDiffEditor({ beforeMount, options, ...props }: DenovaMonacoDiffEditorProps) {
  const theme = useDenovaMonacoTheme()
  const sourceEditorFontFamily = useSourceEditorFontFamily()
  const contentFontScale = useContentFontScale()
  const handleBeforeMount = useDenovaBeforeMount(beforeMount)
  const denovaOptions = useMemo<editor.IStandaloneDiffEditorConstructionOptions>(() => ({
    ...options,
    fontFamily: options?.fontFamily || sourceEditorFontFamily,
    fontSize: options?.fontSize ?? contentFontScale.sourceEditor,
    unicodeHighlight: denovaUnicodeHighlight(options?.unicodeHighlight),
  }), [contentFontScale.sourceEditor, options, sourceEditorFontFamily])

  return (
    <MonacoDiffEditor
      {...props}
      theme={theme}
      beforeMount={handleBeforeMount}
      options={denovaOptions}
    />
  )
}

const installedMonacoInstances = new WeakSet<Monaco>()

/** Installs both variants once because Monaco themes are global per runtime. */
export function installDenovaMonacoThemes(monaco: Monaco) {
  if (installedMonacoInstances.has(monaco)) return
  installedMonacoInstances.add(monaco)
  monaco.editor.defineTheme(DENOVA_MONACO_THEME_DARK, createTheme(DARK_PALETTE))
  monaco.editor.defineTheme(DENOVA_MONACO_THEME_LIGHT, createTheme(LIGHT_PALETTE))
}

function useDenovaMonacoTheme() {
  const { resolvedTheme } = useTheme()
  return resolvedTheme === 'light' ? DENOVA_MONACO_THEME_LIGHT : DENOVA_MONACO_THEME_DARK
}

function useSourceEditorFontFamily() {
  return useSyncExternalStore(
    subscribeSourceEditorFont,
    getSourceEditorFontFamily,
    getSourceEditorFontFamily,
  )
}

function useContentFontScale() {
  return useSyncExternalStore(
    subscribeContentFontScale,
    getContentFontScale,
    getContentFontScale,
  )
}

function useDenovaBeforeMount(beforeMount: ((monaco: Monaco) => void) | undefined) {
  return useCallback((monaco: Monaco) => {
    installDenovaMonacoThemes(monaco)
    beforeMount?.(monaco)
  }, [beforeMount])
}

function denovaUnicodeHighlight(overrides: editor.IUnicodeHighlightOptions | undefined): editor.IUnicodeHighlightOptions {
  return {
    ...overrides,
    // Locale allowances only apply to ambiguous-character detection. Keeping
    // this false prevents ordinary CJK text from entering the broad fallback.
    nonBasicASCII: false,
    invisibleCharacters: true,
    ambiguousCharacters: true,
    allowedLocales: {
      ...overrides?.allowedLocales,
      ...DENOVA_MONACO_UNICODE_HIGHLIGHT.allowedLocales,
    },
  }
}

interface DenovaMonacoPalette {
  base: 'vs' | 'vs-dark'
  background: string
  gutter: string
  surface: string
  raised: string
  border: string
  borderSoft: string
  foreground: string
  muted: string
  faint: string
  accent: string
  accentSoft: string
  selection: string
  selectionInactive: string
  selectionMatch: string
  lineHighlight: string
  findMatch: string
  findHighlight: string
  keyword: string
  string: string
  number: string
  type: string
  function: string
  regex: string
  success: string
  warning: string
  warningSoft: string
  danger: string
  scrollbar: string
  scrollbarHover: string
  addedLine: string
  removedLine: string
  addedWord: string
  removedWord: string
}

// Monaco commonly renders dense 12–14 px text, so token and supporting-text
// colors intentionally target at least 7:1 contrast against the canvas.
const DARK_PALETTE: DenovaMonacoPalette = {
  base: 'vs-dark',
  background: '#0a0a0a',
  gutter: '#0a0a0a',
  surface: '#121212',
  raised: '#1a1a19',
  border: '#292929',
  borderSoft: '#1e1e1e',
  foreground: '#f7f7f4',
  muted: '#d2d2cc',
  faint: '#9d9d97',
  accent: '#f0f0eb',
  accentSoft: '#d5d5cf22',
  selection: '#4a4a4666',
  selectionInactive: '#3a3a373d',
  selectionMatch: '#d5d5cf1c',
  lineHighlight: '#141413',
  findMatch: '#7a5b2f99',
  findHighlight: '#c49a5840',
  keyword: '#e2c977',
  string: '#a8cbb2',
  number: '#e3aa80',
  type: '#a9c8da',
  function: '#e5cfaa',
  regex: '#dfa097',
  success: '#a8cbb2',
  warning: '#dfad61',
  warningSoft: '#c49a5824',
  danger: '#ed8a83',
  scrollbar: '#50504c66',
  scrollbarHover: '#68686299',
  addedLine: '#1f3124',
  removedLine: '#3c1f1b',
  addedWord: '#32533c',
  removedWord: '#62302a',
}

const LIGHT_PALETTE: DenovaMonacoPalette = {
  base: 'vs',
  background: '#ffffff',
  gutter: '#fafafa',
  surface: '#f5f5f5',
  raised: '#ffffff',
  border: '#d4d4d4',
  borderSoft: '#e5e5e5',
  foreground: '#171717',
  muted: '#3f3f3f',
  faint: '#595959',
  accent: '#202020',
  accentSoft: '#2f2f2f14',
  selection: '#b8b8b866',
  selectionInactive: '#d4d4d45c',
  selectionMatch: '#2f2f2f12',
  lineHighlight: '#f5f5f5',
  findMatch: '#d1ad628f',
  findHighlight: '#d1ad6240',
  keyword: '#604a0e',
  string: '#245d3b',
  number: '#87421f',
  type: '#285b75',
  function: '#5f481b',
  regex: '#833b36',
  success: '#125f36',
  warning: '#6b4400',
  warningSoft: '#b47a102e',
  danger: '#a40e03',
  scrollbar: '#8c8c8c55',
  scrollbarHover: '#73737380',
  addedLine: '#e2f2e7',
  removedLine: '#f8e2dd',
  addedWord: '#b9e2c3',
  removedWord: '#efb7ad',
}

function createTheme(palette: DenovaMonacoPalette): editor.IStandaloneThemeData {
  return {
    base: palette.base,
    inherit: true,
    rules: [
      { token: '', foreground: tokenColor(palette.foreground), background: tokenColor(palette.background) },
      { token: 'comment', foreground: tokenColor(palette.faint) },
      { token: 'keyword', foreground: tokenColor(palette.keyword) },
      { token: 'keyword.control', foreground: tokenColor(palette.keyword) },
      { token: 'string', foreground: tokenColor(palette.string) },
      { token: 'string.key.json', foreground: tokenColor(palette.type) },
      { token: 'string.value.json', foreground: tokenColor(palette.string) },
      { token: 'number', foreground: tokenColor(palette.number) },
      { token: 'constant', foreground: tokenColor(palette.number) },
      { token: 'type', foreground: tokenColor(palette.type) },
      { token: 'type.identifier', foreground: tokenColor(palette.type) },
      { token: 'identifier.function', foreground: tokenColor(palette.function) },
      { token: 'regexp', foreground: tokenColor(palette.regex) },
      { token: 'tag', foreground: tokenColor(palette.keyword) },
      { token: 'attribute.name', foreground: tokenColor(palette.type) },
      { token: 'delimiter', foreground: tokenColor(palette.muted) },
      { token: 'invalid', foreground: tokenColor(palette.danger) },
    ],
    colors: {
      'editor.background': palette.background,
      'editor.foreground': palette.foreground,
      'editorGutter.background': palette.gutter,
      'editorLineNumber.foreground': palette.faint,
      'editorLineNumber.activeForeground': palette.accent,
      'editorCursor.foreground': palette.accent,
      'editor.selectionBackground': palette.selection,
      'editor.inactiveSelectionBackground': palette.selectionInactive,
      'editor.selectionHighlightBackground': palette.selectionMatch,
      'editor.wordHighlightBackground': palette.selectionMatch,
      'editor.wordHighlightStrongBackground': palette.accentSoft,
      'editor.lineHighlightBackground': palette.lineHighlight,
      'editor.lineHighlightBorder': '#00000000',
      'editor.rangeHighlightBackground': palette.selectionMatch,
      'editor.symbolHighlightBackground': palette.selectionMatch,
      'editor.findMatchBackground': palette.findMatch,
      'editor.findMatchHighlightBackground': palette.findHighlight,
      'editor.findRangeHighlightBackground': palette.accentSoft,
      'editor.hoverHighlightBackground': palette.accentSoft,
      'editorWhitespace.foreground': palette.border,
      'editorIndentGuide.background1': palette.borderSoft,
      'editorIndentGuide.activeBackground1': palette.faint,
      'editorBracketMatch.background': palette.accentSoft,
      'editorBracketMatch.border': palette.muted,
      'editorBracketHighlight.foreground1': palette.keyword,
      'editorBracketHighlight.foreground2': palette.type,
      'editorBracketHighlight.foreground3': palette.string,
      'editorBracketHighlight.foreground4': palette.number,
      'editorBracketHighlight.foreground5': palette.regex,
      'editorBracketHighlight.foreground6': palette.function,
      'editorBracketHighlight.unexpectedBracket.foreground': palette.danger,
      'editorBracketPairGuide.background1': palette.borderSoft,
      'editorBracketPairGuide.background2': palette.borderSoft,
      'editorBracketPairGuide.background3': palette.borderSoft,
      'editorBracketPairGuide.activeBackground1': palette.keyword,
      'editorBracketPairGuide.activeBackground2': palette.type,
      'editorBracketPairGuide.activeBackground3': palette.string,
      'editorWidget.background': palette.raised,
      'editorWidget.foreground': palette.foreground,
      'editorWidget.border': palette.border,
      'editorWidget.resizeBorder': palette.faint,
      'editorHoverWidget.background': palette.raised,
      'editorHoverWidget.foreground': palette.foreground,
      'editorHoverWidget.border': palette.border,
      'editorHoverWidget.statusBarBackground': palette.surface,
      'editorSuggestWidget.background': palette.raised,
      'editorSuggestWidget.foreground': palette.foreground,
      'editorSuggestWidget.border': palette.border,
      'editorSuggestWidget.selectedBackground': palette.accentSoft,
      'editorSuggestWidget.highlightForeground': palette.warning,
      'editorStickyScroll.background': palette.background,
      'editorStickyScrollGutter.background': palette.gutter,
      'editorStickyScrollHover.background': palette.surface,
      'editorStickyScroll.border': palette.borderSoft,
      'editorStickyScroll.shadow': '#00000000',
      'editorUnicodeHighlight.border': palette.warning,
      'editorUnicodeHighlight.background': palette.warningSoft,
      'editorOverviewRuler.border': palette.borderSoft,
      'minimap.background': palette.background,
      'minimap.selectionHighlight': palette.selection,
      'minimap.findMatchHighlight': palette.findMatch,
      'scrollbar.shadow': '#00000000',
      'scrollbarSlider.background': palette.scrollbar,
      'scrollbarSlider.hoverBackground': palette.scrollbarHover,
      'scrollbarSlider.activeBackground': palette.muted,
      'focusBorder': palette.faint,
      'diffEditor.border': palette.border,
      'diffEditor.diagonalFill': palette.borderSoft,
      'diffEditor.insertedLineBackground': palette.addedLine,
      'diffEditor.removedLineBackground': palette.removedLine,
      'diffEditor.insertedTextBackground': palette.addedWord,
      'diffEditor.removedTextBackground': palette.removedWord,
      'diffEditorGutter.insertedLineBackground': palette.addedLine,
      'diffEditorGutter.removedLineBackground': palette.removedLine,
      'diffEditorOverview.insertedForeground': palette.success,
      'diffEditorOverview.removedForeground': palette.danger,
      'diffEditor.unchangedRegionBackground': palette.surface,
      'diffEditor.unchangedRegionForeground': palette.muted,
      'diffEditor.unchangedCodeBackground': palette.accentSoft,
    },
  }
}

function tokenColor(color: string) {
  return color.slice(1, 7)
}
