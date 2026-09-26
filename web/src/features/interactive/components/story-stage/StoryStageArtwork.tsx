import { useEffect, useState } from 'react'
import { projectFileAssetURL } from '@/lib/api-client/project-files'
import type { PresentationMaterial, StoryPresentationSettings, TurnEvent } from '../../types'

interface StoryStageArtworkProps {
  projectId: string
  turn?: TurnEvent
  // Derived from the branch's ordered turns. parent_id may identify a plan or
  // other journal event, so it cannot establish visual turn continuity.
  previousTurnId?: string
  latest: boolean
  settings?: StoryPresentationSettings
  textHidden: boolean
  scrimOpacity: number
}

// Key this component by project/story/branch. Only an immediate continuation at
// the live head may retain a previously loaded image while its replacement loads.
export function StoryStageArtwork({ projectId, turn, previousTurnId, latest, settings, textHidden, scrimOpacity }: StoryStageArtworkProps) {
  const scene = `${turn?.id || ''}:${turn?.version_idx ?? 0}`
  const [continuity, setContinuity] = useState({ scene, turnId: turn?.id, latest, epoch: 0 })
  if (continuity.scene !== scene || continuity.latest !== latest) {
    const continues = latest && continuity.latest && previousTurnId !== undefined && previousTurnId === continuity.turnId
    setContinuity({ scene, turnId: turn?.id, latest, epoch: continuity.epoch + (continues ? 0 : 1) })
  }
  const stage = turn?.turn_result?.presentation
  const background = settings?.background !== false ? stage?.background : undefined
  const characters = settings?.characters !== false ? stage?.characters ?? [] : []
  if (!background && !characters.length) return null

  return (
    <div data-testid="story-stage-artwork" className="pointer-events-none absolute inset-0 overflow-hidden" aria-hidden="true">
      {background && <StageImage key={`${continuity.epoch}:background`} projectId={projectId} material={background} layer="background" />}
      <div className="absolute inset-x-0 bottom-0 flex h-[88%] items-end justify-center">
        {characters.map(character => (
          <div key={`${continuity.epoch}:${character.item_id}`} className="relative h-full min-w-0 flex-1" style={{ maxWidth: characters.length === 1 ? '70%' : undefined }}>
            <StageImage projectId={projectId} material={character} layer="character" />
          </div>
        ))}
      </div>
      <div data-testid="story-stage-scrim" className="absolute inset-0 bg-[var(--nova-surface-2)] transition-opacity duration-200 motion-reduce:transition-none" style={{ opacity: textHidden ? 0 : scrimOpacity }} />
    </div>
  )
}

function StageImage({ projectId, material, layer }: { projectId: string; material: PresentationMaterial; layer: 'background' | 'character' }) {
  const [loaded, setLoaded] = useState<{ src: string; name: string }>()
  const src = projectFileAssetURL(projectId, material.path)
  useEffect(() => {
    let cancelled = false
    const image = new Image()
    image.onload = () => {
      // decode avoids replacing the previous image before the browser can paint.
      const ready = typeof image.decode === 'function' ? image.decode() : Promise.resolve()
      void ready.then(() => { if (!cancelled) setLoaded({ src, name: material.name }) }).catch(() => {
        if (!cancelled) console.warn('[story-presentation] image decoding failed', { path: material.path })
      })
    }
    image.onerror = () => console.warn('[story-presentation] image loading failed', { path: material.path })
    image.src = src
    return () => { cancelled = true; image.onload = null; image.onerror = null }
  }, [src, material.name, material.path])

  return loaded ? <img data-stage-layer={layer} src={loaded.src} alt={loaded.name} draggable={false} className={`absolute inset-0 h-full w-full ${layer === 'background' ? 'object-cover' : 'object-contain object-bottom'} animate-in fade-in duration-200 motion-reduce:animate-none`} /> : null
}
