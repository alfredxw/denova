import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { useGameStories, type GameChoice } from './game-story-context'
import { BUILTIN_GAME_ID, platformError } from './api'
import { RuntimeSetup, emptySetup, type Setup } from './RuntimeSetup'

/** The selector is part of new-story setup, never a separate navigation step. */
export function GameStorySetup({ enabled, children }: { enabled: boolean; children: ReactNode }) {
  const context = useGameStories()
  const { t } = useTranslation()
  const [saving, setSaving] = useState(false)
  if (!enabled || !context) return children
  const game = context.choices.find(game => game.id === context.selectedGameId)!
  return <div className="flex min-h-0 flex-1 flex-col">
    {context.choices.length > 1 && <div className="flex flex-wrap items-end gap-2 border-b px-4 py-3 sm:px-6">
      <Field className="min-w-0 flex-1 sm:max-w-sm">
        <FieldLabel htmlFor="story-game-type">{t('platform.gameType')}</FieldLabel>
        <Select value={context.selectedGameId} onValueChange={context.chooseGame}>
          <SelectTrigger id="story-game-type" className="w-full"><SelectValue /></SelectTrigger>
          <SelectContent>{context.choices.map(choice => <SelectItem key={choice.id} value={choice.id}>{choice.name}</SelectItem>)}</SelectContent>
        </Select>
      </Field>
      <Button variant="ghost" size="sm" disabled={saving || context.defaultGameId === context.selectedGameId} onClick={() => {
        setSaving(true)
        void context.setDefault().catch(error => toast.error(platformError(error))).finally(() => setSaving(false))
      }}>{t(context.defaultGameId === context.selectedGameId ? 'platform.defaultGame' : 'platform.setDefaultGame')}</Button>
    </div>}
    {game.id === BUILTIN_GAME_ID ? children : <InstalledGameSetup key={game.id} game={game} projectId={context.projectId} onCreate={context.createInstalled} />}
  </div>
}

function InstalledGameSetup({ game, projectId, onCreate }: { game: GameChoice; projectId: string; onCreate: (title: string, setup: Setup, storyId?: string) => Promise<void> }) {
  const { t } = useTranslation()
  const context = useGameStories()
  const [title, setTitle] = useState('')
  const [storyId, setStoryId] = useState('new')
  const [setup, setSetup] = useState({ ...emptySetup, projectId })
  const [busy, setBusy] = useState(false)
  return <div className="flex min-h-0 flex-1 flex-col overflow-y-auto p-4 sm:p-6">
    <form className="mx-auto flex w-full max-w-xl flex-col gap-6" onSubmit={event => {
      event.preventDefault()
      setBusy(true)
      void onCreate(title, setup, storyId === 'new' ? undefined : storyId).catch(error => toast.error(platformError(error))).finally(() => setBusy(false))
    }}>
      <div className="space-y-2"><h2 className="break-words text-lg font-semibold">{game.name}</h2><p className="text-sm text-muted-foreground">{t('platform.newGameStoryDescription')}</p></div>
      <Field><FieldLabel htmlFor="game-story-title">{t('platform.storyName')}</FieldLabel><Input id="game-story-title" value={title} placeholder={game.name} maxLength={120} onChange={event => setTitle(event.target.value)} /></Field>
      {game.manifest?.game?.storage.kind === 'story' && <Field>
        <FieldLabel htmlFor="game-story-source">{t('platform.storySource')}</FieldLabel>
        <Select value={storyId} onValueChange={setStoryId} disabled={busy}>
          <SelectTrigger id="game-story-source" className="w-full"><SelectValue /></SelectTrigger>
          <SelectContent><SelectGroup>
            <SelectItem value="new">{t('platform.newStorySource')}</SelectItem>
            {context?.picker.stories.filter(story => !story.id.startsWith('game:')).map(story => <SelectItem key={story.id} value={story.id}>{story.title}</SelectItem>)}
          </SelectGroup></SelectContent>
        </Select>
        <FieldDescription>{t('platform.storySourceHelp')}</FieldDescription>
      </Field>}
      <RuntimeSetup configurationEndpoint={`/packages/game/${game.id}/setup?releaseId=${game.releaseId}`} manifest={game.manifest} value={setup} onChange={setSetup} projectLocked />
      <Button type="submit" disabled={busy || !projectId}>{t('platform.createStory')}</Button>
    </form>
  </div>
}
