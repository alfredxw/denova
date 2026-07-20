const search = {
  'search.placeholder': '搜索当前书籍...',
  'search.failed': '搜索失败',
  'search.resultCount': '找到 {{count}} 条结果',
  'search.noWorkspace': '请先在书籍管理中选择一本书',
  'search.empty': '输入关键词搜索正文、设定和项目文件',
  'search.noResults': '没有找到匹配内容',
  'search.line': '第 {{line}} 行',
  'search.pathMatch': '路径匹配',
  'search.column': '列 {{column}}',
  'search.toggleRegex': '正则匹配',
  'search.toggleReplace': '替换',
  'search.replacePlaceholder': '替换为...',
  'search.replaceAll': '全部替换',
  'search.replaceConfirmTitle': '全局替换确认',
  'search.replaceConfirmDescription': '将把工作区内所有文件内容中的「{{query}}」替换为「{{replacement}}」。替换范围不限于当前显示的搜索结果，替换前会自动创建可恢复版本。',
  'search.replaceDone': '已在 {{files}} 个文件中替换 {{count}} 处',
  'search.replaceNoMatches': '没有可替换的匹配内容',
  'search.replaceSkipped': '{{count}} 个文件因被同时修改而跳过',
} as const

export default search
