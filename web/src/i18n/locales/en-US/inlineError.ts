const inlineError = {
  'inlineError.defaultTitle': 'Operation failed',
  'inlineError.cause': 'Cause',
  'inlineError.operation': 'Failed operation',
  'inlineError.code': 'Error code',
  'inlineError.httpStatus': 'HTTP status',
  'inlineError.run': 'Run ID',
  'inlineError.backend': 'Backend',
  'inlineError.frontend': 'UI version {{version}}',
  'inlineError.copy': 'Copy diagnostics',
  'inlineError.copied': 'Copied',
  'inlineError.copyFailed': 'Could not copy. Take a screenshot of this error instead.',
  'inlineError.unexpected': 'An unexpected error occurred. Its cause has not been confirmed.',
  'inlineError.network': 'The request connection was interrupted. The outcome is unknown; check the current state first.',
} as const

export default inlineError
