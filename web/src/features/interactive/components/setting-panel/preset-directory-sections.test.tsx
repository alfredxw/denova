import type { TFunction } from 'i18next'
import { describe, expect, it } from 'vitest'
import { buildPresetDirectorySections } from './preset-directory-sections'

describe('preset directory sections', () => {
  it('orders shared resources before dedicated resources', () => {
    const sections = buildPresetDirectorySections({
      lists: {
        tellers: [],
        storyDirectors: [],
        imagePresets: [],
        eventPackages: [],
        ruleSystems: [],
        actorStates: [],
      },
      onCreateKind: () => undefined,
      t: ((key: string) => key) as TFunction,
    })

    expect(sections.map((section) => section.id)).toEqual([
      'teller',
      'image',
      'director',
      'actor-state',
      'rule',
      'event',
    ])
  })
})
