const inlineError = {
  'inlineError.defaultTitle': '操作失败',
  'inlineError.cause': '原因详情',
  'inlineError.operation': '失败位置',
  'inlineError.code': '错误码',
  'inlineError.httpStatus': 'HTTP 状态',
  'inlineError.run': '运行 ID',
  'inlineError.backend': '后端',
  'inlineError.frontend': '界面版本 {{version}}',
  'inlineError.copy': '复制诊断信息',
  'inlineError.copied': '已复制',
  'inlineError.copyFailed': '无法复制，请直接截取此错误区域。',
  'inlineError.unexpected': '发生了未预期的错误，具体原因尚未确认。',
  'inlineError.network': '请求连接中断，无法确认操作结果。请先检查当前状态。',
} as const

export default inlineError
