export interface FontOption {
  value: string
  label: string
  stack: string
}

export const FONT_OPTIONS: FontOption[] = [
  {
    value: 'system-sans',
    label: '系统无衬线（推荐）',
    stack: '-apple-system, BlinkMacSystemFont, "SF Pro Text", "SF Pro Display", "PingFang SC", "HarmonyOS Sans SC", "MiSans", "Microsoft YaHei UI", "Microsoft YaHei", "Noto Sans CJK SC", sans-serif',
  },
  {
    value: 'humanist-sans',
    label: '人文无衬线',
    stack: '"Inter", "Aptos", "Segoe UI Variable", "Segoe UI", "PingFang SC", "Microsoft YaHei UI", "Noto Sans CJK SC", sans-serif',
  },
  {
    value: 'apple-system',
    label: 'Apple / 苹方',
    stack: '"SF Pro Text", "SF Pro Display", "PingFang SC", "Hiragino Sans GB", -apple-system, BlinkMacSystemFont, sans-serif',
  },
  {
    value: 'microsoft-yahei',
    label: '微软雅黑 UI',
    stack: '"Microsoft YaHei UI", "Microsoft YaHei", "Segoe UI", "Noto Sans CJK SC", sans-serif',
  },
  {
    value: 'source-han-serif',
    label: '思源宋体阅读',
    stack: '"Source Han Serif SC", "Noto Serif CJK SC", "Songti SC", "STSong", "SimSun", serif',
  },
  {
    value: 'system-serif',
    label: '系统宋体阅读',
    stack: '"Songti SC", "STSong", "Noto Serif CJK SC", "Source Han Serif SC", Georgia, serif',
  },
  {
    value: 'lxgw-wenkai',
    label: '霞鹜文楷',
    stack: '"LXGW WenKai Screen", "LXGW WenKai", "霞鹜文楷屏幕阅读版", "霞鹜文楷", "Kaiti SC", "KaiTi", serif',
  },
  {
    value: 'mono',
    label: '等宽字体',
    stack: '"SFMono-Regular", "Cascadia Code", "JetBrains Mono", Consolas, "Liberation Mono", monospace',
  },
]

export function fontStackFor(value?: string | null, fallback = 'system-sans') {
  const option = FONT_OPTIONS.find((item) => item.value === value) || FONT_OPTIONS.find((item) => item.value === fallback) || FONT_OPTIONS[0]
  return option.stack
}

export function fontLabelFor(value?: string | null) {
  return FONT_OPTIONS.find((item) => item.value === value)?.label || value || '未设置'
}
