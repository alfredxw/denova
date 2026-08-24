export interface FontOption {
  value: string
  labelKey: string
  stack: string
}

export const CUSTOM_FONT_PREFIX = 'custom:'
export const MAX_CUSTOM_FONT_FAMILY_LENGTH = 200

const INVALID_CUSTOM_FONT_CHARACTERS = /[\u0000-\u001f\u007f-\u009f]/

export const FONT_OPTIONS: FontOption[] = [
  {
    value: 'system-sans',
    labelKey: 'settings.font.systemSans',
    stack: '-apple-system, BlinkMacSystemFont, "SF Pro Text", "SF Pro Display", "PingFang SC", "HarmonyOS Sans SC", "MiSans", "Microsoft YaHei UI", "Microsoft YaHei", "Noto Sans CJK SC", sans-serif',
  },
  {
    value: 'humanist-sans',
    labelKey: 'settings.font.humanistSans',
    stack: '"Inter", "Aptos", "Segoe UI Variable", "Segoe UI", "PingFang SC", "Microsoft YaHei UI", "Noto Sans CJK SC", sans-serif',
  },
  {
    value: 'apple-system',
    labelKey: 'settings.font.appleSystem',
    stack: '"SF Pro Text", "SF Pro Display", "PingFang SC", "Hiragino Sans GB", -apple-system, BlinkMacSystemFont, sans-serif',
  },
  {
    value: 'microsoft-yahei',
    labelKey: 'settings.font.microsoftYahei',
    stack: '"Microsoft YaHei UI", "Microsoft YaHei", "Segoe UI", "Noto Sans CJK SC", sans-serif',
  },
  {
    value: 'source-han-serif',
    labelKey: 'settings.font.sourceHanSerif',
    stack: '"Source Han Serif SC", "Noto Serif CJK SC", "Songti SC", "STSong", "SimSun", serif',
  },
  {
    value: 'system-serif',
    labelKey: 'settings.font.systemSerif',
    stack: '"Songti SC", "STSong", "Noto Serif CJK SC", "Source Han Serif SC", Georgia, serif',
  },
  {
    value: 'lxgw-wenkai',
    labelKey: 'settings.font.lxgwWenkai',
    stack: '"LXGW WenKai Screen", "LXGW WenKai", "霞鹜文楷屏幕阅读版", "霞鹜文楷", "Kaiti SC", "KaiTi", serif',
  },
  {
    value: 'mono',
    labelKey: 'settings.font.mono',
    stack: '"SFMono-Regular", "SF Mono", "Cascadia Mono", "Cascadia Code", "JetBrains Mono", "Sarasa Mono SC", "Noto Sans Mono CJK SC", Menlo, Monaco, Consolas, "Liberation Mono", "Noto Sans SC", "Microsoft YaHei UI", "Microsoft YaHei", "PingFang SC", monospace',
  },
]

export function fontStackFor(value?: string | null, fallback = 'system-sans') {
  const fallbackOption = FONT_OPTIONS.find((item) => item.value === fallback) || FONT_OPTIONS[0]
  const customFamily = customFontFamilyFromValue(value)
  if (customFamily) return `${quoteCSSString(customFamily)}, ${fallbackOption.stack}`
  return FONT_OPTIONS.find((item) => item.value === value)?.stack || fallbackOption.stack
}

export function fontLabelKeyFor(value?: string | null) {
  return FONT_OPTIONS.find((item) => item.value === value)?.labelKey
}

/** Creates the persisted identifier for one local family, never an arbitrary CSS stack. */
export function customFontValue(family: string): string | null {
  const normalized = normalizeCustomFontFamily(family)
  return normalized ? `${CUSTOM_FONT_PREFIX}${normalized}` : null
}

export function customFontFamilyFromValue(value?: string | null): string | null {
  if (!value?.startsWith(CUSTOM_FONT_PREFIX)) return null
  return normalizeCustomFontFamily(value.slice(CUSTOM_FONT_PREFIX.length))
}

function normalizeCustomFontFamily(value: string): string | null {
  const normalized = value.trim()
  if (
    normalized.length === 0
    || normalized.length > MAX_CUSTOM_FONT_FAMILY_LENGTH
    || INVALID_CUSTOM_FONT_CHARACTERS.test(normalized)
  ) return null
  return normalized
}

function quoteCSSString(value: string) {
  return `"${value.replace(/\\/g, '\\\\').replace(/"/g, '\\"')}"`
}
