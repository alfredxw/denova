import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { SETTINGS_SECTION_EVENT, takeSettingsSection, type SettingsSectionRequest } from '@/features/onboarding/events'
import { SETTINGS_SECTIONS, type SettingsFieldRequest, type SettingsSectionId } from './settings-sections'

/** Keeps the directory aligned with the continuous page; requests never own editor state. */
export function useSettingsNavigation({ visible, ready }: { visible: boolean; ready: boolean }) {
  const { t } = useTranslation()
  const contentRef = useRef<HTMLDivElement>(null)
  const [activeSection, setActiveSection] = useState<SettingsSectionId>('general')
  const [fieldRequest, setFieldRequest] = useState<SettingsFieldRequest>()
  const selectSection = useCallback((id: SettingsSectionId, fieldKey?: string) => {
    setFieldRequest({ id, fieldKey })
  }, [])

  useEffect(() => {
    if (!visible || !ready) return
    const openSection = (id: unknown) => {
      const section = SETTINGS_SECTIONS.find((item) => item.id === id)
      if (section) selectSection(section.id)
    }
    openSection(takeSettingsSection())
    const handleRequest = (event: Event) => {
      takeSettingsSection()
      openSection((event as CustomEvent<SettingsSectionRequest>).detail?.section)
    }
    window.addEventListener(SETTINGS_SECTION_EVENT, handleRequest)
    return () => window.removeEventListener(SETTINGS_SECTION_EVENT, handleRequest)
  }, [visible, ready, selectSection])

  useEffect(() => {
    const container = contentRef.current
    if (!visible || !ready || !fieldRequest || !container) return
    // Disclosure cards reveal the requested field before positioning the page.
    const frame = requestAnimationFrame(() => {
      const section = container.querySelector<HTMLElement>(`[data-settings-section="${fieldRequest.id}"]`)!
      const label = fieldRequest.fieldKey ? t(fieldRequest.fieldKey) : ''
      const target = label ? Array.from(section.querySelectorAll<HTMLElement>('label, [data-slot="field-title"]'))
        .find((node) => node.textContent?.trim() === label || node.querySelector(':scope > span')?.textContent?.trim() === label) : undefined
      container.scrollTo({ top: container.scrollTop + (target ?? section).getBoundingClientRect().top - container.getBoundingClientRect().top - 20 })
      if (target) {
        const controlID = target.getAttribute('for')
        const control = (controlID ? document.getElementById(controlID) : null)
          ?? target.querySelector<HTMLElement>('input, button, select, textarea')
          ?? target.closest('[data-slot="field"]')?.querySelector<HTMLElement>('input, button, select, textarea')
        control?.focus({ preventScroll: true })
      }
      setFieldRequest(undefined)
    })
    return () => cancelAnimationFrame(frame)
  }, [visible, ready, fieldRequest, t])

  useEffect(() => {
    const container = contentRef.current
    if (!visible || !ready || !container) return
    const sections = Array.from(container.querySelectorAll<HTMLElement>('[data-settings-section]'))
    let frame = 0
    const updateActiveSection = () => {
      frame = 0
      const top = container.getBoundingClientRect().top + 48
      let active = sections[0]
      for (const section of sections) {
        if (section.getBoundingClientRect().top > top) break
        active = section
      }
      setActiveSection(active.dataset.settingsSection as SettingsSectionId)
    }
    const scheduleUpdate = () => {
      if (!frame) frame = requestAnimationFrame(updateActiveSection)
    }
    container.addEventListener('scroll', scheduleUpdate, { passive: true })
    const resize = new ResizeObserver(scheduleUpdate)
    resize.observe(container)
    resize.observe(container.firstElementChild!)
    scheduleUpdate()
    return () => {
      cancelAnimationFrame(frame)
      resize.disconnect()
      container.removeEventListener('scroll', scheduleUpdate)
    }
  }, [visible, ready])

  return { contentRef, activeSection, fieldRequest, selectSection }
}
