const search = {
  'search.placeholder': 'Поиск по текущей книге...',
  'search.failed': 'Ошибка поиска',
  'search.resultCount': 'Найдено результатов: {{count}}',
  'search.resultLimit': 'Показаны первые {{count}} результатов',
  'search.noWorkspace': 'Сначала выберите книгу в «Управлении книгами»',
  'search.empty': 'Поиск по тексту, лору и файлам проекта',
  'search.noResults': 'Ничего не найдено',
  'search.line': 'Строка {{line}}',
  'search.pathMatch': 'Совпадение в пути',
  'search.toggleRegex': 'Регулярное выражение',
  'search.toggleReplace': 'Замена',
  'search.expandFileResults': 'Показать все результаты ({{count}}) в {{name}}',
  'search.collapseFileResults': 'Свернуть результаты поиска в {{name}}',
  'search.replacePlaceholder': 'Заменить на...',
  'search.replaceAll': 'Заменить все',
  'search.replaceConfirmTitle': 'Подтверждение глобальной замены',
  'search.replaceConfirmDescription': 'Заменить все вхождения «{{query}}» на «{{replacement}}» в содержимом всех файлов рабочей области. Это действие затронет не только показанные результаты. Перед заменой создается версия для восстановления.',
  'search.replaceDone': 'Заменено вхождений: {{count}} в {{files}} файлах',
  'search.replaceNoMatches': 'Нет совпадений для замены',
  'search.replaceSkipped': 'Пропущено файлов: {{count}} (были изменены во время замены)',
} as const

export default search
