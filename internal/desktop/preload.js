if (window === window.top) {
  const call = (method, ...args) => window.mygo.call(`Desktop.${method}`, ...args);
  Object.defineProperty(window, 'denovaDesktop', { value: Object.freeze({
    ...denovaShell,
    chooseDirectory: initial => call('ChooseDirectory', initial || ''),
    changeDataDirectory: () => call('ChangeDataDirectory'),
    setAppearance: appearance => call('SetAppearance', appearance),
    updates: Object.freeze({
      status: () => call('UpdateStatus'), open: () => call('OpenUpdates'),
      preferences: () => call('UpdatePreferences'), setPreferences: value => call('SetUpdatePreferences', value),
    }),
  }) });
}
