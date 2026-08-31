package interactive

import (
	"fmt"
	"strings"
)

type branchPlanSection struct {
	heading   string
	bodyStart int
	bodyEnd   int
}

type branchPlanSectionUpdateError struct {
	Index    int
	Heading  string
	Code     string
	Expected string
	Actual   string
	Message  string
}

// applyBranchPlanSectionUpdates applies every valid edit and reports invalid
// siblings independently. This lets a weak model retry only failed sections
// while retaining accepted edits in the run-local turn draft.
func applyBranchPlanSectionUpdates(current string, updates []TurnPlanSectionUpdate) (string, []string, []branchPlanSectionUpdateError, error) {
	draft := normalizeBranchPlanMarkdown(current)
	if err := validateBranchPlanMarkdown(draft); err != nil {
		return draft, nil, nil, fmt.Errorf("current branch plan is unavailable for section replacement: %w", err)
	}
	if _, err := parseBranchPlanSections(draft); err != nil {
		return draft, nil, nil, err
	}

	duplicateKeys := map[string]bool{}
	counts := map[string]int{}
	for _, update := range updates {
		key := normalizePlanSectionHeadingKey(update.Heading)
		if key != "" {
			counts[key]++
		}
	}
	for key, count := range counts {
		if count > 1 {
			duplicateKeys[key] = true
		}
	}

	accepted := make([]string, 0, len(updates))
	errorsOut := make([]branchPlanSectionUpdateError, 0)
	for listIndex, update := range updates {
		index := listIndex
		if update.sourceIndex != nil {
			index = *update.sourceIndex
		}
		heading := normalizePlanSectionHeading(update.Heading)
		key := normalizePlanSectionHeadingKey(heading)
		body := strings.TrimSpace(update.Markdown)
		switch {
		case heading == "":
			errorsOut = append(errorsOut, newBranchPlanSectionUpdateError(index, heading, "empty_plan_section_heading", "non-empty existing H2 heading", "empty heading", "A plan section update must name an existing H2 heading."))
			continue
		case duplicateKeys[key]:
			errorsOut = append(errorsOut, newBranchPlanSectionUpdateError(index, heading, "duplicate_plan_section_update", "each H2 heading at most once per call", heading, "The same plan section cannot be replaced more than once in one call."))
			continue
		case body == "":
			errorsOut = append(errorsOut, newBranchPlanSectionUpdateError(index, heading, "empty_plan_section_body", "non-empty Markdown section body", "empty body", "A replacement section body must be non-empty."))
			continue
		case branchPlanBodyChangesStructure(body):
			errorsOut = append(errorsOut, newBranchPlanSectionUpdateError(index, heading, "plan_section_structure_change", "section body without H1 or H2 headings", "structural heading", "replace_sections cannot add, rename, remove, or reorder H2 modules. Use replace_document for structural changes."))
			continue
		}

		sections, err := parseBranchPlanSections(draft)
		if err != nil {
			return draft, accepted, errorsOut, err
		}
		section, exists := sections[key]
		if !exists {
			errorsOut = append(errorsOut, newBranchPlanSectionUpdateError(index, heading, "plan_section_not_found", "exact text of an existing unique H2 heading", heading, "The requested H2 section does not exist in the current branch plan. Use replace_document to change the plan structure."))
			continue
		}

		replacement := "\n" + body + "\n\n"
		candidate := normalizeBranchPlanMarkdown(draft[:section.bodyStart] + replacement + draft[section.bodyEnd:])
		if err := validateBranchPlanMarkdown(candidate); err != nil {
			errorsOut = append(errorsOut, newBranchPlanSectionUpdateError(index, heading, "plan_document_too_large", fmt.Sprintf("complete plan up to %d bytes", maxBranchPlanBytes), fmt.Sprintf("%d bytes", len([]byte(candidate))), err.Error()))
			continue
		}
		draft = candidate
		accepted = append(accepted, section.heading)
	}
	return draft, accepted, errorsOut, nil
}

func newBranchPlanSectionUpdateError(index int, heading, code, expected, actual, message string) branchPlanSectionUpdateError {
	return branchPlanSectionUpdateError{
		Index: index, Heading: heading, Code: code,
		Expected: expected, Actual: actual, Message: message,
	}
}

func parseBranchPlanSections(markdown string) (map[string]branchPlanSection, error) {
	headings := scanBranchPlanATXHeadings(markdown)
	sections := map[string]branchPlanSection{}
	for index, heading := range headings {
		if heading.level != 2 {
			continue
		}
		visible := normalizePlanSectionHeading(heading.text)
		key := normalizePlanSectionHeadingKey(visible)
		if key == "" {
			return nil, fmt.Errorf("current branch plan contains an empty H2 heading; use replace_document to repair its structure")
		}
		if _, duplicate := sections[key]; duplicate {
			return nil, fmt.Errorf("current branch plan contains duplicate H2 heading %q; use replace_document to repair its structure", visible)
		}
		bodyEnd := len(markdown)
		for next := index + 1; next < len(headings); next++ {
			if headings[next].level == 2 {
				bodyEnd = headings[next].lineStart
				break
			}
		}
		sections[key] = branchPlanSection{
			heading:   visible,
			bodyStart: heading.lineEnd, bodyEnd: bodyEnd,
		}
	}
	if len(sections) == 0 {
		return nil, fmt.Errorf("current branch plan has no ATX H2 sections; use replace_document to create a modular plan")
	}
	return sections, nil
}

func branchPlanBodyChangesStructure(markdown string) bool {
	for _, heading := range scanBranchPlanATXHeadings(markdown) {
		if heading.level <= 2 {
			return true
		}
	}
	return false
}

type branchPlanATXHeading struct {
	level     int
	text      string
	lineStart int
	lineEnd   int
}

func scanBranchPlanATXHeadings(markdown string) []branchPlanATXHeading {
	headings := make([]branchPlanATXHeading, 0)
	fenceMarker := byte(0)
	fenceLength := 0
	for lineStart := 0; lineStart <= len(markdown); {
		lineBreak := strings.IndexByte(markdown[lineStart:], '\n')
		lineEnd := len(markdown)
		nextLine := len(markdown) + 1
		if lineBreak >= 0 {
			lineEnd = lineStart + lineBreak
			nextLine = lineEnd + 1
		}
		line := strings.TrimSuffix(markdown[lineStart:lineEnd], "\r")
		indent := 0
		for indent < len(line) && indent < 4 && line[indent] == ' ' {
			indent++
		}
		content := line[indent:]
		marker, count := markdownFencePrefix(content)
		if fenceMarker != 0 {
			if marker == fenceMarker && count >= fenceLength && strings.TrimSpace(content[count:]) == "" {
				fenceMarker, fenceLength = 0, 0
			}
		} else if indent <= 3 && marker != 0 {
			fenceMarker, fenceLength = marker, count
		} else if indent <= 3 {
			if level, text, ok := parseBranchPlanATXHeading(content); ok {
				headingLineEnd := nextLine
				if headingLineEnd > len(markdown) {
					headingLineEnd = len(markdown)
				}
				headings = append(headings, branchPlanATXHeading{level: level, text: text, lineStart: lineStart, lineEnd: headingLineEnd})
			}
		}
		if nextLine > len(markdown) {
			break
		}
		lineStart = nextLine
	}
	return headings
}

func markdownFencePrefix(line string) (byte, int) {
	if len(line) < 3 || (line[0] != '`' && line[0] != '~') {
		return 0, 0
	}
	marker := line[0]
	count := 0
	for count < len(line) && line[count] == marker {
		count++
	}
	if count < 3 {
		return 0, 0
	}
	return marker, count
}

func parseBranchPlanATXHeading(line string) (int, string, bool) {
	level := 0
	for level < len(line) && level < 6 && line[level] == '#' {
		level++
	}
	if level == 0 || (level < len(line) && line[level] != ' ' && line[level] != '\t') {
		return 0, "", false
	}
	text := strings.TrimSpace(line[level:])
	lastNonHash := len(text)
	for lastNonHash > 0 && text[lastNonHash-1] == '#' {
		lastNonHash--
	}
	if lastNonHash < len(text) && lastNonHash > 0 && (text[lastNonHash-1] == ' ' || text[lastNonHash-1] == '\t') {
		text = strings.TrimSpace(text[:lastNonHash])
	}
	return level, text, true
}

func normalizePlanSectionHeading(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

func normalizePlanSectionHeadingKey(value string) string {
	return strings.ToLower(normalizePlanSectionHeading(value))
}
