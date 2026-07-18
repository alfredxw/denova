interface PresetStatusItem {
  custom?: boolean
  builtin_overridden?: boolean
}

/** Consistent built-in/custom status copy for preset directories and editors. */
export function presetStatusLabel(item: PresetStatusItem, t: (key: string) => string) {
  if (item.custom) return t('settingPanel.custom')
  if (item.builtin_overridden) return t('settingPanel.builtInOverridden')
  return t('settingPanel.builtIn')
}
