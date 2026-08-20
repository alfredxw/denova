package interactive

import (
	"sort"
	"strings"
)

// DefaultMemoryRosterLimit 是喂给抽取器的名册条数上限。名册要小到能塞进
// 每回合的抽取提示里,又要大到覆盖长篇故事的常驻角色与关键物品。
const DefaultMemoryRosterLimit = 60

// MemoryEntity 是实体名册里的一项:一个实体的权威写法与它的活跃度。
type MemoryEntity struct {
	// Name 是该实体的权威写法。同一实体出现过多种写法时,取出现次数最多的
	// 那种 —— 抽取器被要求复用它,确定性对齐层也按它回写。
	Name     string   `json:"name"`
	Mentions int      `json:"mentions"`
	Kinds    []string `json:"kinds,omitempty"`
}

// NormalizeEntityName 是实体的比对键。它在检索归一化之上再去掉所有空白 ——
// 检索键把内部空格保留为分隔符,于是"林 舟"和"林舟"在检索侧是两个实体,
// 这类排版漂移正是对齐层要消除的。对齐把它们回写成同一个权威写法之后,
// 检索侧看到的就已经是干净的世界了。
//
// 代价是英文里"Iron Man"与"Ironman"会被视作同一实体。对实体名而言,
// 空格是排版噪声的可能性远高于语义,这个方向的合并是划算的。
func NormalizeEntityName(value string) string {
	return strings.ReplaceAll(normalizeMemorySearchText(value), " ", "")
}

// StoryEntityRoster 从当前分支已有的记忆记录里自举一份实体名册。
//
// 之所以自举而不是读某份权威名册:代码库里没有覆盖人物/物品/地点/概念的
// 结构化清单,actor 状态只涵盖角色。已落库的记忆记录反而是唯一一份完整的
// "这个故事出现过哪些实体"的记录。
//
// beforeTurnID 语义与 SearchStoryMemory 一致,让重放某个较早时点的抽取
// 不会看见未来才出现的实体。
func (s *Store) StoryEntityRoster(storyID, branchID, beforeTurnID string, limit int) ([]MemoryEntity, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return nil, err
	}
	_, branch, err := resolveBranch(meta, strings.TrimSpace(branchID))
	if err != nil {
		return nil, err
	}
	return entityRosterFromLines(lines, branch.Head, strings.TrimSpace(beforeTurnID), limit), nil
}

// entityRosterFromLines 从分支路径上的记忆事件投影出名册。
func entityRosterFromLines(lines []StoryEventRecord, head, beforeTurnID string, limit int) []MemoryEntity {
	path, _ := eventPath(head, eventsByID(lines))
	memoryEvents, turnOrder := collectNarrativeMemory(path)
	projection := ProjectNarrativeMemory(memoryEvents, turnOrder, beforeTurnID)
	return buildMemoryEntityRoster(projection.Records, limit)
}

// alignRecordsToRoster 把记录里的实体写法就地回写为名册中的权威写法,
// 返回改写留痕。
//
// 这一层只做确定性归一:归一化键相同(大小写、全半角、空格与标点差异)才回写,
// 绝不做语义猜测 —— "那把剑"到"蚀骨剑"的判断需要读懂正文,那是抽取器的职责,
// 由喂给它的名册提示完成。两层分工:提示层管语义别名,这一层管写法漂移。
func alignRecordsToRoster(records []NarrativeMemoryRecord, roster []MemoryEntity) []NarrativeMemoryEntityAlignment {
	index := MemoryEntityRosterIndex(roster)
	if len(index) == 0 || len(records) == 0 {
		return nil
	}
	var aligned []NarrativeMemoryEntityAlignment
	align := func(recordID, field, value string) string {
		if strings.TrimSpace(value) == "" {
			return value
		}
		canonical, ok := index[NormalizeEntityName(value)]
		if !ok || canonical == value {
			return value
		}
		aligned = append(aligned, NarrativeMemoryEntityAlignment{
			RecordID: recordID,
			Field:    field,
			From:     value,
			To:       canonical,
		})
		return canonical
	}
	for i := range records {
		records[i].Subject = align(records[i].ID, "subject", records[i].Subject)
		records[i].Object = align(records[i].ID, "object", records[i].Object)
	}
	return aligned
}

// entityTally 聚合同一归一化键下的所有写法。
type entityTally struct {
	spellings map[string]int
	kinds     map[string]bool
	mentions  int
}

func buildMemoryEntityRoster(records []NarrativeMemoryRecord, limit int) []MemoryEntity {
	if limit <= 0 {
		limit = DefaultMemoryRosterLimit
	}
	tallies := map[string]*entityTally{}
	note := func(raw, kind string) {
		name := strings.TrimSpace(raw)
		key := NormalizeEntityName(name)
		if key == "" {
			return
		}
		tally, ok := tallies[key]
		if !ok {
			tally = &entityTally{spellings: map[string]int{}, kinds: map[string]bool{}}
			tallies[key] = tally
		}
		tally.spellings[name]++
		tally.mentions++
		if kind != "" {
			tally.kinds[kind] = true
		}
	}
	for _, record := range records {
		note(record.Subject, record.Kind)
		note(record.Object, record.Kind)
	}

	roster := make([]MemoryEntity, 0, len(tallies))
	for _, tally := range tallies {
		roster = append(roster, MemoryEntity{
			Name:     canonicalSpelling(tally.spellings),
			Mentions: tally.mentions,
			Kinds:    sortedSet(tally.kinds),
		})
	}
	// 高频实体优先:名册被截断时,常驻角色比一次性提及的路人更该留下。
	sort.SliceStable(roster, func(i, j int) bool {
		if roster[i].Mentions != roster[j].Mentions {
			return roster[i].Mentions > roster[j].Mentions
		}
		return roster[i].Name < roster[j].Name
	})
	if len(roster) > limit {
		roster = roster[:limit]
	}
	return roster
}

// canonicalSpelling 在同一实体的多种写法中选出权威写法:出现最多的那种,
// 平票时取字典序最小的,保证同样的输入永远得到同样的名册。
func canonicalSpelling(spellings map[string]int) string {
	best := ""
	bestCount := 0
	for spelling, count := range spellings {
		if count > bestCount || (count == bestCount && (best == "" || spelling < best)) {
			best, bestCount = spelling, count
		}
	}
	return best
}

func sortedSet(set map[string]bool) []string {
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for value := range set {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

// MemoryEntityRosterIndex 把名册转成"归一化键 → 权威写法"的对照表,
// 供抽取产出后的确定性对齐使用。
func MemoryEntityRosterIndex(roster []MemoryEntity) map[string]string {
	if len(roster) == 0 {
		return nil
	}
	index := make(map[string]string, len(roster))
	for _, entity := range roster {
		key := NormalizeEntityName(entity.Name)
		if key == "" {
			continue
		}
		index[key] = entity.Name
	}
	return index
}
