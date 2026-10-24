const inlineError = {
  'inlineError.defaultTitle': 'Сбой операции',
  'inlineError.cause': 'Причина',
  'inlineError.operation': 'Неудавшаяся операция',
  'inlineError.code': 'Код ошибки',
  'inlineError.httpStatus': 'HTTP-статус',
  'inlineError.run': 'ID запуска',
  'inlineError.backend': 'Бэкенд',
  'inlineError.frontend': 'Версия интерфейса {{version}}',
  'inlineError.copy': 'Скопировать диагностику',
  'inlineError.copied': 'Скопировано',
  'inlineError.copyFailed': 'Не удалось скопировать. Сделайте снимок экрана с этой ошибкой.',
  'inlineError.unexpected': 'Произошла непредвиденная ошибка. Её причина пока не подтверждена.',
  'inlineError.network': 'Соединение с сервером было прервано. Результат неизвестен; сначала проверьте текущее состояние.',
} as const

export default inlineError
