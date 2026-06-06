export interface FontOption {
  value: string
  labelKey: string
  stack: string
}

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
    stack: '"SFMono-Regular", "Cascadia Code", "JetBrains Mono", Consolas, "Liberation Mono", monospace',
  },
]

export function fontStackFor(value?: string | null, fallback = 'system-sans') {
  const option = FONT_OPTIONS.find((item) => item.value === value) || FONT_OPTIONS.find((item) => item.value === fallback) || FONT_OPTIONS[0]
  return option.stack
}

export function fontLabelKeyFor(value?: string | null) {
  return FONT_OPTIONS.find((item) => item.value === value)?.labelKey
}
