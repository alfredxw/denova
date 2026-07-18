/** 按资源类型路由到对应预设编辑器的主体区域。 */
import type { PresetResourceKind } from '../../preset-ownership'
import type { ActorStateModule, EventPackageModule, ImagePreset, RuleSystemModule, StoryDirector, Teller } from '../../types'
import { TellerEditor } from '../SettingPanelTellerEditor'
import { StoryDirectorEditor } from '../story-director/StoryDirectorEditor'
import { ActorStateEditor } from './ActorStateEditor'
import { EventPackageEditor } from './EventPackageEditor'
import { ImagePresetEditor } from './ImagePresetEditor'
import { RuleSystemEditor } from './RuleSystemEditor'

interface PresetResourcePaneProps {
  kind: PresetResourceKind
  workspace: string
  tellers: Teller[]
  storyDirectors: StoryDirector[]
  imagePresets: ImagePreset[]
  eventPackages: EventPackageModule[]
  ruleSystems: RuleSystemModule[]
  actorStates: ActorStateModule[]
  tellerDraft: Teller | null
  setTellerDraft: (draft: Teller | null) => void
  activeSlotId: string
  setActiveSlotId: (value: string) => void
  storyDirectorDraft: StoryDirector | null
  setStoryDirectorDraft: (draft: StoryDirector | null) => void
  imagePresetDraft: ImagePreset | null
  setImagePresetDraft: (draft: ImagePreset | null) => void
  eventPackageDraft: EventPackageModule | null
  setEventPackageDraft: (draft: EventPackageModule | null) => void
  ruleSystemDraft: RuleSystemModule | null
  setRuleSystemDraft: (draft: RuleSystemModule | null) => void
  actorStateDraft: ActorStateModule | null
  setActorStateDraft: (draft: ActorStateModule | null) => void
  onOpenActorState: (id: string) => void
  onOpenRuleSystem: (id: string) => void
  onSave: () => void
  onValidityChange: (valid: boolean) => void
}

export function PresetResourcePane(props: PresetResourcePaneProps) {
  if (props.kind === 'image') return <ImagePresetPane draft={props.imagePresetDraft} setDraft={props.setImagePresetDraft} onSave={props.onSave} />
  if (props.kind === 'event') return <EventPackagePane draft={props.eventPackageDraft} setDraft={props.setEventPackageDraft} onSave={props.onSave} onValidityChange={props.onValidityChange} />
  if (props.kind === 'rule') return <RuleSystemPane draft={props.ruleSystemDraft} actorStates={props.actorStates} setDraft={props.setRuleSystemDraft} onOpenActorState={props.onOpenActorState} onSave={props.onSave} onValidityChange={props.onValidityChange} />
  if (props.kind === 'actor-state') return <ActorStatePane draft={props.actorStateDraft} ruleSystems={props.ruleSystems} setDraft={props.setActorStateDraft} onOpenRuleSystem={props.onOpenRuleSystem} onSave={props.onSave} onValidityChange={props.onValidityChange} />
  if (props.kind === 'director') {
    return (
      <StoryDirectorPane
        draft={props.storyDirectorDraft}
        tellers={props.tellers}
        eventPackages={props.eventPackages}
        ruleSystems={props.ruleSystems}
        actorStates={props.actorStates}
        imagePresets={props.imagePresets}
        setDraft={props.setStoryDirectorDraft}
        onSave={props.onSave}
        onValidityChange={props.onValidityChange}
      />
    )
  }
  return <TellerPane workspace={props.workspace} draft={props.tellerDraft} setDraft={props.setTellerDraft} activeSlotId={props.activeSlotId} setActiveSlotId={props.setActiveSlotId} onSave={props.onSave} />
}

function TellerPane(props: { workspace: string; draft: Teller | null; setDraft: (draft: Teller | null) => void; activeSlotId: string; setActiveSlotId: (value: string) => void; onSave: () => void }) {
  return <TellerEditor {...props} />
}

function ImagePresetPane(props: { draft: ImagePreset | null; setDraft: (draft: ImagePreset | null) => void; onSave: () => void }) {
  return <ImagePresetEditor {...props} />
}

function EventPackagePane(props: { draft: EventPackageModule | null; setDraft: (draft: EventPackageModule | null) => void; onSave: () => void; onValidityChange: (valid: boolean) => void }) {
  return <EventPackageEditor {...props} />
}

function RuleSystemPane(props: { draft: RuleSystemModule | null; actorStates: ActorStateModule[]; setDraft: (draft: RuleSystemModule | null) => void; onOpenActorState: (id: string) => void; onSave: () => void; onValidityChange: (valid: boolean) => void }) {
  return <RuleSystemEditor {...props} />
}

function ActorStatePane(props: { draft: ActorStateModule | null; ruleSystems: RuleSystemModule[]; setDraft: (draft: ActorStateModule | null) => void; onOpenRuleSystem: (id: string) => void; onSave: () => void; onValidityChange: (valid: boolean) => void }) {
  return <ActorStateEditor {...props} />
}

function StoryDirectorPane(props: { draft: StoryDirector | null; tellers: Teller[]; eventPackages: EventPackageModule[]; ruleSystems: RuleSystemModule[]; actorStates: ActorStateModule[]; imagePresets: ImagePreset[]; setDraft: (draft: StoryDirector | null) => void; onSave: () => void; onValidityChange: (valid: boolean) => void }) {
  return <StoryDirectorEditor {...props} />
}
