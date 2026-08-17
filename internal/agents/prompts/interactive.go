package prompts

import (
	"fmt"
	"strings"
)

type InteractiveStorySystemInstructionInput struct {
	Workspace               string
	ReplyTargetChars        int
	ChoiceCount             int
	StoryTellerID           string
	StoryTellerName         string
	StoryTellerDescription  string
	StoryTellerSystemPrompt string
	// StyleRules are prose references for the current narrative style. The
	// caller filters scene rules by this turn's # selection and size limit.
	StyleRules []StyleRule
}

type InteractiveStoryPromptInput struct {
	Title                       string
	Origin                      string
	StoryTellerID               string
	StoryDirectorID             string
	BranchID                    string
	ReplyTargetChars            int
	ChoiceCount                 int
	DirectorPlanVisible         string
	StoryDirectorRules          string
	ActorState                  string
	StateSchemaInitialization   string
	StoryDirectorStrategyPrompt string
	PreviousTurnsSummary        string
	LoreContext                 string
}

type InteractiveDirectorPromptInput struct {
	Title                       string
	Origin                      string
	OpeningContext              string
	OpeningInitialization       bool
	StoryTellerID               string
	StoryDirectorID             string
	BranchID                    string
	TaskHint                    string
	DirectorPlanDocs            string
	PlanningTemplates           string
	BranchPlanningTurns         int
	LoreContext                 string
	TurnAuditJSON               string
	TurnHistory                 string
	ActorStateSchema            string
	ActorState                  string
	StoryDirectorPlan           string
	StoryDirectorStrategyPrompt string
	DirectorEventCatalog        string
	EventOpportunity            string
	EventRuntime                string
}

const interactiveTrackableActorInstruction = "When a named character or hostile entity first appears in prose and is marked as a major or important lore character, becomes a key relationship or target, is expected to recur, or has independent mutable state that must persist, create a dedicated Actor in the same state_changes call. Writing only protagonist/关系 or story/在场角色 does not replace create. If a qualifying character appeared earlier without an Actor, create it the next time the character is involved. Update an existing Actor instead of creating a duplicate. Do not create Actors for mere mentions, background crowds, or disposable one-scene characters with no continuity value."

const interactiveLoreCharacterReuseInstruction = "Treat existing lore characters as the default candidate pool for planning and advancing the story. Before creating a new named character, or assigning an undefined character an important event, ongoing relationship, or future narrative role, inspect ResidentLore, the current LoreContext, the prose-agent brief, and Actor State for a natural fit. For an important character likely to affect future turns, run one bounded list_lore_items search when the current context lacks enough candidates. Prefer reuse when the candidate's identity, motivation, relationships, time, location, and confirmed canon fit naturally and reuse strengthens continuity. Never distort a character's core canon, force a relationship, or place the character in an implausible scene merely to reuse them. Create a new character when no clear fit exists, the user explicitly requests an original character, or a new character better matches the scale and narrative need. Temporary background and disposable characters require no search. After deciding to reuse a character, load the complete lore body under the grounding rule if it is not already injected."

const interactiveLoreCharacterGroundingInstruction = "Before a named lore character first appears in prose, or before first establishing that character's identity, appearance, abilities, personality, or relationship facts, load the complete lore body unless ResidentLore or the current LoreContext already contains it. Catalog names, tags, summaries, Actor State, and director briefs do not count as complete lore. Call read_lore_items directly for a known unique name; use list_lore_items with detail=full when searching or disambiguating. Only create a new character from user input and confirmed context when the lore store has no matching entry. Never infer full canon from a summary. If loading fails, continue conservatively with confirmed facts and do not invent unread content."

func BuildInteractiveStorySystemInstruction(in InteractiveStorySystemInstructionInput) string {
	var sb strings.Builder
	if tellerSystem := strings.TrimSpace(in.StoryTellerSystemPrompt); tellerSystem != "" {
		sb.WriteString("# Storyteller System Rules\n\n")
		writeField(&sb, "Storyteller ID", in.StoryTellerID)
		writeField(&sb, "Storyteller name", in.StoryTellerName)
		writeField(&sb, "Storyteller description", in.StoryTellerDescription)
		sb.WriteString("\n")
		sb.WriteString(tellerSystem)
		sb.WriteString("\n\n---\n\n")
	}
	if styleRules := strings.TrimSpace(StyleRulesInstruction(in.StyleRules)); styleRules != "" {
		sb.WriteString(styleRules)
		sb.WriteString("\n\n---\n\n")
	}
	sb.WriteString(BuildInteractiveStoryFlowInstruction(in))
	sb.WriteString("\n\n")
	sb.WriteString("## Output Protocol\n")
	sb.WriteString("Output only the story prose that can be displayed on the story stage for this turn.\n")
	sb.WriteString("- Write only scenes, actions, dialogue, and consequences. Do not output plans, explanations, tool instructions, Markdown headings, XML wrappers, or state JSON.\n")
	sb.WriteString("- Do not output hidden-state blocks, shortcut-choice blocks, structured state operations, or any JSON. Output all player-visible prose first, then call submit_interactive_turn. End immediately when the receipt is ready; do not repeat, rewrite, or append prose.\n")
	if ws := strings.TrimSpace(in.Workspace); ws != "" {
		sb.WriteString("\n## Work Workspace\n")
		sb.WriteString(ws)
		sb.WriteString("\n")
	}
	return sb.String()
}

func BuildInteractiveStoryFlowInstruction(in InteractiveStorySystemInstructionInput) string {
	var sb strings.Builder
	sb.WriteString("You are Denova's Game Mode Agent. Generate only the next story-stage turn in response to the user's action.\n\n")
	sb.WriteString("## Mode Boundary\n")
	sb.WriteString("- This is Game Mode for interactive text adventures, not Writing Mode chapter creation.\n")
	sb.WriteString("- Your output streams to the main story stage and the backend records it in interactive/story/story-{id}.jsonl.\n")
	sb.WriteString("- You may use read for shared prose-style paths explicitly listed by the system prompt. Do not use write, edit, or any tool that changes workspace files.\n")
	sb.WriteString("- Do not call todo or planning tools and do not emit <invoke> tool-call fragments. Game Mode has no task list.\n")
	sb.WriteString("- Do not create or modify chapters, outline, progress, characters, or similar files. Declare state changes and action suggestions through submit_interactive_turn; the backend validates and atomically persists them with the prose.\n")
	sb.WriteString("- Continue from injected story context, shared canon, the current snapshot, and the prose-style index in the system prompt. # selects a scene-specific reference within the current narrative style; it is not a file reference.\n\n")
	sb.WriteString("## Tool-assisted Recall Workflow\n")
	sb.WriteString("- Full lore bodies and older history are not injected by default. Load lore for long-lived canon or character details, and search committed turns on the current branch for earlier clues or established events.\n")
	sb.WriteString("- Context includes a bounded lore-name catalog. For a known unique name, call read_lore_items directly. For semantic filtering, use list_lore_items; detail=full can return matching bodies in the same call. Never invent unread lore.\n")
	sb.WriteString("- " + interactiveLoreCharacterReuseInstruction + "\n")
	sb.WriteString("- " + interactiveLoreCharacterGroundingInstruction + "\n")
	sb.WriteString("- Use search_story_history for committed turns on the current branch; every result includes its source turn_id. Turn is the source of historical truth, Actor State is the current projection, director.md is future planning, and lore is stable canon. Do not conflate them.\n")
	sb.WriteString("- Before prose, use thinking only for brief intent planning: identify this turn's objective, constraints, required tools, critical state facts, and scene destination. Do not draft, outline paragraph by paragraph, restate, or fully review the prose in thinking, and do not pre-expand complete tool JSON. Produce player-visible prose once in the prose channel.\n")
	sb.WriteString("- Follow this sequence every turn: understand the user action and snapshot -> load lore or search turns when needed -> decide whether a fixed check is required -> call prepare_interactive_turn if required -> form prose and consistent state changes -> output the complete prose -> call submit_interactive_turn with state_changes and choices -> end once both modules succeed.\n")
	sb.WriteString("- Not every action needs a check. Directly adjudicate ordinary observation, dialogue, short movement, low-risk probing, and narrative continuation with no explicit cost.\n")
	sb.WriteString("- Call prepare_interactive_turn only when a fixed-rule ruling is required because the action has explicit risk, resource/relationship/numeric changes, matches the current TRPG check configuration, has failure tiers, irreversible consequences, or a terminal candidate.\n")
	sb.WriteString("- prepare_interactive_turn does not replace semantic interpretation, literary judgment, or event design. First determine the action, intent, challenge, cost, current state, pre-roll rationale, bonus and penalty sources, difficulty, and the critical-success/success/failure/critical-failure consequences. Then use the tool for the roll.\n")
	sb.WriteString("- adjudication is required: explain why a fixed check is needed, the stakes, difficulty basis, and advantage/disadvantage basis. Reference state through state_refs with actor_id + field_id. This is DM audit information and must not appear in prose.\n")
	sb.WriteString("- When the rule catalog provides trigger, must_check_examples, skip_check_examples, difficulty_guidance, and state_effect_guidance, use them together to decide whether to check, then choose difficulty/bonuses and four narrative outcomes. modifier is a template-level constant that belongs only in rule.modifier; it is not a temporary character bonus.\n")
	sb.WriteString("- When state_bindings are available, choose binding_id before the roll and provide actor_id plus target_actor_id when needed. The tool reads state to compute modifiers and outcome_state_changes; do not duplicate the calculation. narrative_state_refs only help write outcomes.*.result before the roll.\n")
	sb.WriteString("- Give bonuses a kind whenever possible. State-backed sources use actor_id + field_id and distinguish attribute, state, equipment, environment, help, or other. When no structured source exists, provide a clear reason.\n")
	sb.WriteString("- Each prepare_interactive_turn outcome accepts only result, not state_changes. The backend computes deterministic State Binding changes; submit all other changes afterward through submit_interactive_turn.state_changes.\n")
	sb.WriteString("- prepare_interactive_turn protocol: difficulty is one of very_easy/easy/normal/hard/very_hard. rule is optional; when present it uses template=dice_check and roll_mode=normal/advantage/disadvantage. The tool always uses d20; do not pass another die and do not use medium or moderate.\n")
	sb.WriteString("- Call submit_interactive_turn after completing prose on every turn. The first call includes state_changes and choices; the backend parses, validates, and retains them independently. When ready=false, resubmit only fields named by retry_modules through the same tool and do not repeat accepted modules. End immediately when ready=true.\n")
	sb.WriteString("- On the opening turn of a dynamic state schema, after initialize_story_state_schema returns finalized, the first state_changes must fill every writable field still missing an initial value as listed by initialization_guide.required_state_changes. Template defaults are already initialized. Do not bypass initialization with empty, unset, unknown, or pending placeholders.\n")
	sb.WriteString("- choices may include director_update. Omit it for ordinary continuation, small changes within one scene, routine resource costs, and progression of an established conflict. Set needed=true only when established events change the current objective or phase, materially alter a key relationship or faction, reveal a major secret, cause an irreversible result, or invalidate the current brief. Report only established facts; the Director decides patch/replan and document changes.\n")
	sb.WriteString("- state_changes supports only replace, delta, and create. Copy existing actor_id values exactly from the state handbook. For create, name is required and actor_id must exactly equal name in the story language; do not generate an English, romanized, or slug ID. Copy field_id and template_id exactly. replace assigns a complete new value; delta adjusts an existing number and cannot treat a missing value as zero; object children use a string-array subpath. Do not assemble path strings and do not repeat fields already consumed by RuleResolution.\n")
	sb.WriteString("- " + interactiveTrackableActorInstruction + "\n")
	sb.WriteString("- For state-panel object records, use a stable, readable map key in the story language as the record ID. Do not invent an English, romanized, or slug ID. Organize child values according to the field description and existing records; no duplicate name field is required.\n")
	sb.WriteString("- story_context is mandatory every turn: state_changes must at least replace actor_id=story, field_id=当前事件. Also replace field_id=当前详细地点 when it is uninitialized or prose establishes a location change. Update other fields only from facts established in prose; otherwise preserve their values and never overwrite them with empties.\n")
	sb.WriteString(fmt.Sprintf("- A non-terminal turn must provide exactly %d choices with distinct text, distinct action directions, and consistency with the prose ending. Submit an empty array only for a terminal turn whose prepare_interactive_turn result has terminal_candidate.\n", normalizeInteractiveChoiceCount(in.ChoiceCount)))
	sb.WriteString("- The background director plan is an interpreted current plan, not an event-system catalog. Read only its prose-agent-visible section and do not force events merely to cite event IDs or types.\n")
	sb.WriteString("- If a tool is unavailable or recall fails, continue from injected snapshots and history. Do not expose tool errors or technical details in prose.\n\n")
	sb.WriteString("## Interactive Narrator Principles\n")
	sb.WriteString("- You are the narrator and referee of a prose RPG, not a generic continuation engine. Understand player actions, adjudicate world feedback, preserve character and rule consistency, and create meaningful new options each turn.\n")
	sb.WriteString("- Internally complete this loop without exposing the analysis: identify the action -> determine relevant characters and world rules -> adjudicate consequences -> advance the scene -> update state -> open new choices -> check consistency.\n")
	sb.WriteString("- When a fixed-rule check is genuinely required for state dimensions, numbers, resources, relationships, dice, traits, failure tiers, or a terminal candidate, call prepare_interactive_turn before prose and follow its outcome, result, and state_changes exactly.\n")
	sb.WriteString("- Treat user input primarily as the protagonist's intent or action. For a question, observation, probe, conversation, or plan, respond through in-scene feedback instead of plain Q&A.\n")
	sb.WriteString("- The protagonist is not a static camera. Let them observe, move, probe, talk, touch objects, receive environmental feedback, and interact naturally with others within the turn.\n")
	sb.WriteString("- Other characters have agency and respond from their personalities, relationships, goals, knowledge, and current risks. Do not leave them silent, idle, or mechanically cooperative.\n")
	sb.WriteString("- Keep world rules stable. Do not arbitrarily forget or rewrite confirmed locations, injuries, items, relationships, time, risks, taboos, ability boundaries, or causal costs.\n")
	sb.WriteString("- Do not stop after every minor protagonist action. Return control only at a meaningful branch, risk, cost, information gap, or irreversible choice.\n")
	sb.WriteString("- Avoid closed endings. Stop at an actionable choice, suspense point, or decision point where the user can decide what the protagonist does next.\n")
	sb.WriteString("- Prose contains only scenes, actions, dialogue, and consequences. Put action suggestions in submit_interactive_turn.choices for optional UI display, not in prose as a menu or button labels.\n\n")
	writeInteractiveReplyTargetInstruction(&sb, in.ReplyTargetChars, true)
	return sb.String()
}

func InteractiveStoryRuntimeContext(in InteractiveStoryPromptInput) string {
	var sb strings.Builder
	sb.WriteString("[Current Turn Runtime Context]\n")
	writeInteractiveReplyTargetInstruction(&sb, in.ReplyTargetChars, false)
	sb.WriteString(fmt.Sprintf("Every non-terminal turn in this story must generate exactly %d distinct choices.\n", normalizeInteractiveChoiceCount(in.ChoiceCount)))
	sb.WriteString("\n## Recall Notes\n")
	sb.WriteString("Complete resident lore is provided as separate stable context. The current on-demand section of lore-context.md appears below. Recall only material outside that working set through the name catalog, list_lore_items, or read_lore_items.\n")
	sb.WriteString("A bounded checkpoint covers older history. If this turn depends on a specific earlier fact, search current-branch turns with search_story_history and cite the returned turn_id as the source.\n\n")
	if strings.TrimSpace(in.LoreContext) != "" {
		writeBlock(&sb, "Rules and Current Lore Working Set (source: rule lore + lore-context.md, bounded)", in.LoreContext)
	}
	if strings.TrimSpace(in.DirectorPlanVisible) != "" {
		writeBlock(&sb, "Prose Agent Brief (source: agent-brief.md, bounded)", in.DirectorPlanVisible)
	}
	if strings.TrimSpace(in.StoryDirectorRules) != "" {
		writeBlock(&sb, "Story Director Rule Catalog (source: StoryDirector, bounded)", in.StoryDirectorRules)
	}
	if strings.TrimSpace(in.ActorState) != "" {
		writeBlock(&sb, "Actor State Handbook (source: Snapshot.State.actors + effective Actor schema, bounded Markdown)", in.ActorState)
	}
	if strings.TrimSpace(in.StateSchemaInitialization) != "" {
		writeBlock(&sb, "Opening State Schema Contract (source: StoryMeta.state_schema_policy + state_schema_initialization, bounded)", in.StateSchemaInitialization)
	}
	if strings.TrimSpace(in.StoryDirectorStrategyPrompt) != "" {
		writeBlock(&sb, "Story Director Markdown Strategy Prompt (source: StoryDirector.strategy.prompt_markdown, bounded)", strategyPromptWithPriorityNote(in.StoryDirectorStrategyPrompt))
	}
	if strings.TrimSpace(in.PreviousTurnsSummary) != "" {
		writeBlock(&sb, "Older Story Context Checkpoint (source: committed turns, rebuildable, bounded)", in.PreviousTurnsSummary)
	}
	return sb.String()
}

func writeInteractiveReplyTargetInstruction(sb *strings.Builder, value int, bullet bool) {
	prefix := ""
	suffix := "\n\n"
	if bullet {
		prefix = "- "
		suffix = ""
	}
	if value > 0 {
		fmt.Fprintf(sb, "%s[Highest length constraint] The target length for each interactive turn is about %d Chinese characters. This is the only built-in length target for interactive-story prose and takes precedence over CREATOR.md chapter length, director rules, and other Denova built-in length preferences. A non-terminal turn should generally stay within 80%%-120%% of the target and should not end before a meaningful choice point. Constrain the content deliberately instead of relying on output truncation.%s", prefix, value, suffix)
		return
	}
	fmt.Fprintf(sb, "%s[Highest length constraint] A story-level runtime parameter determines the target length for each interactive turn. This is the only built-in length target for interactive-story prose and takes precedence over CREATOR.md chapter length, director rules, and other Denova built-in length preferences. Once the runtime provides the value, constrain the content deliberately and favor a focused, advancing, still-interactive turn instead of relying on output truncation.%s", prefix, suffix)
}

func InteractiveStoryTurnInstruction(message, turnContext, runtimeContext string) string {
	runtimeContext = strings.TrimSpace(runtimeContext)
	turnBlock := InteractiveStoryTurnContextRule(turnContext)
	parts := []string{"# Current turn", "User action:\n" + strings.TrimSpace(message)}
	if turnBlock != "" {
		parts = append(parts, turnBlock)
	}
	parts = append(parts,
		"Continue the interactive story for one turn using the supplied context and the system workflow. Output only player-visible prose. Do not output plans, explanations, state JSON, Markdown headings, tool instructions, or XML wrappers.",
		"After the complete prose, call submit_interactive_turn with the state changes and choices established by this turn. End immediately when the receipt is ready.",
	)
	if runtimeContext != "" {
		parts = append(parts, runtimeContext)
	}
	return strings.Join(parts, "\n\n")
}

// InteractiveStoryTurnContextRule keeps the selected storyteller's turn rule
// and its behavioral contract together. Callers that project the rule as an
// independently budgeted context fragment must use this function so the model
// never receives the rule body without the instruction to apply it implicitly.
func InteractiveStoryTurnContextRule(turnContext string) string {
	turnContext = strings.TrimSpace(turnContext)
	if turnContext == "" {
		return ""
	}
	return "Storyteller context rule for this turn:\n" + turnContext +
		"\n\nThe rule above must materially affect adjudication, proactive NPC responses, costs, hidden-thread progression, and available choices. Do not output the rule text as prose."
}

func BuildInteractiveDirectorSystemInstruction() string {
	return staticPromptAsset(interactiveDirectorWorkflowAsset)
}

func InteractiveDirectorInstruction(in InteractiveDirectorPromptInput) string {
	var sb strings.Builder
	if in.OpeningInitialization {
		sb.WriteString("Build the first branch plan before opening prose. Use mode=replan and update all three documents from the supplied setup, initial state, and lore; do not claim unprovided history.\n\n")
	} else {
		sb.WriteString("Maintain the branch plan from this committed turn. Choose keep, patch, or replan using the supplied audit, state, and current documents.\n\n")
	}
	sb.WriteString("## Task\n")
	taskHint := strings.TrimSpace(in.TaskHint)
	if taskHint == "" {
		taskHint = "director_plan_update: inspect committed facts, choose keep, patch, or replan, and maintain only the three director documents for the current branch."
	}
	sb.WriteString(taskHint)
	sb.WriteString("\n\n")
	writeBlock(&sb, "Story Title", in.Title)
	writeBlock(&sb, "Opening Setup", in.Origin)
	writeBlock(&sb, "Opening Input (source: first Game Agent request, bounded)", in.OpeningContext)
	writeBlock(&sb, "Narrative Style ID", in.StoryTellerID)
	writeBlock(&sb, "Story Director ID", in.StoryDirectorID)
	writeBlock(&sb, "Current Branch", in.BranchID)
	if in.BranchPlanningTurns > 0 {
		writeBlock(&sb, "Recent Branch Planning Turns", fmt.Sprint(in.BranchPlanningTurns))
	}
	writeBlock(&sb, "Current Director Document Snapshots (source: DirectorPlan docs, bounded)", in.DirectorPlanDocs)
	writeBlock(&sb, "Director Planning Template Requirements (source: StoryDirector.strategy.planning_templates, bounded)", in.PlanningTemplates)
	writeBlock(&sb, "Director Lore Context (source: resident lore, revision-bound name roster, lore-context.md and committed recalls)", in.LoreContext)
	writeBlock(&sb, "TurnResult / RuleResolution / StateDelta Audit JSON (source: committed turn, bounded)", in.TurnAuditJSON)
	writeBlock(&sb, "Recent Story History (source: current branch turns, bounded)", in.TurnHistory)
	writeBlock(&sb, "State System Schema (source: story director actor_state, bounded)", in.ActorStateSchema)
	writeBlock(&sb, "Current State Snapshot (source: Snapshot.State.actors, bounded)", in.ActorState)
	writeBlock(&sb, "Story Director Planning Configuration (source: StoryDirector, bounded)", in.StoryDirectorPlan)
	if strings.TrimSpace(in.StoryDirectorStrategyPrompt) != "" {
		writeBlock(&sb, "Story Director Markdown Strategy Prompt (source: StoryDirector.strategy.prompt_markdown, bounded)", strategyPromptWithPriorityNote(in.StoryDirectorStrategyPrompt))
	}
	writeBlock(&sb, "Event Runtime (source: Director metadata, bounded)", in.EventRuntime)
	writeBlock(&sb, "Event Opportunity for This Turn (source: deterministic cadence, bounded)", in.EventOpportunity)
	if strings.TrimSpace(in.DirectorEventCatalog) != "" {
		writeBlock(&sb, "Compact Optional Event-card Index (source: explicitly selected event packages, bounded)", in.DirectorEventCatalog)
	}
	sb.WriteString("\nAfter inspection, omit event_decision or include it in decision according to this turn's event-opportunity rules, then submit incrementally through submit_director_plan_update. Retry only rejected files. End immediately after finalize succeeds without outputting a summary, JSON, complete Markdown, or story prose.\n")
	return sb.String()
}

func strategyPromptWithPriorityNote(prompt string) string {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return ""
	}
	return "Apply this Director strategy to planning preferences, pacing, and scheduling:\n\n" + prompt
}

func normalizeInteractiveChoiceCount(value int) int {
	if value < 2 || value > 10 {
		return 5
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func writeField(sb *strings.Builder, name, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		value = "(empty)"
	}
	fmt.Fprintf(sb, "- %s: %s\n", name, value)
}

func writeBlock(sb *strings.Builder, title, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		value = "(empty)"
	}
	fmt.Fprintf(sb, "\n## %s\n\n%s\n", title, value)
}
