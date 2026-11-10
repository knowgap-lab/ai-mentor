package homework

import (
	"cmp"
	"slices"
	"strconv"
)

const homeworkRatio = 2

type draftItem struct {
	VariantID    string `json:"variant_id"`
	TypeID       string `json:"type_id"`
	AnswerFormat string `json:"answer_format"`
	Minutes      int    `json:"minutes"`
	Source       string `json:"source"`
	Statement    string `json:"statement"`
}

type warningView struct {
	Type   string         `json:"type"`
	Text   string         `json:"text"`
	Params map[string]any `json:"params,omitempty"`
}

type draftView struct {
	LessonID      string        `json:"lesson_id"`
	Deadline      string        `json:"deadline"`
	Items         []draftItem   `json:"items"`
	Hints         []string      `json:"hints"`
	Warnings      []warningView `json:"warnings"`
	DiscussTopics []string      `json:"discuss_topics"`
	Issued        bool          `json:"issued"`
}

type storedDraft struct {
	Items         []draftItem
	Deadline      string
	Hints         []string
	Warnings      []warningView
	DiscussTopics []string
}

type sibling struct {
	PlaceNo  int
	State    string
	StartsAt string
}

func (d storedDraft) view(lessonID string, issued bool) draftView {
	return draftView{
		LessonID:      lessonID,
		Deadline:      d.Deadline,
		Items:         listOrEmpty(d.Items),
		Hints:         listOrEmpty(d.Hints),
		Warnings:      listOrEmpty(d.Warnings),
		DiscussTopics: listOrEmpty(d.DiscussTopics),
		Issued:        issued,
	}
}

func listOrEmpty[T any](items []T) []T {
	if items == nil {
		return []T{}
	}
	return items
}

func buildDraft(row lessonRow, siblings []sibling) storedDraft {
	deadline := nextDeadline(row.PlaceNo, siblings)
	if row.NoBlocks && row.Blob.Coverage == "all" && row.Blob.Mark == nil {
		return storedDraft{Deadline: deadline}
	}
	return assemble(row.Blob, deadline)
}

func nextDeadline(place int, siblings []sibling) string {
	bestPlace := 0
	best := ""
	for _, item := range siblings {
		if item.PlaceNo <= place || item.State == "skipped" || item.StartsAt == "" {
			continue
		}
		if best == "" || item.PlaceNo < bestPlace {
			best = item.StartsAt
			bestPlace = item.PlaceNo
		}
	}
	return best
}

func assemble(blob lessonBlob, deadline string) storedDraft {
	byID := map[string]Variant{}
	for _, item := range blob.Variants {
		byID[item.ID] = item
	}
	seen := seenIDs(blob)
	used := map[string]bool{}
	explained := explainedBlocks(blob.Blocks, theoryOf(blob.Mark))

	var reinforcement []draftItem
	var unsolved []draftItem
	var warnings []warningView
	tasks := slices.Clone(blob.Tasks)
	slices.SortFunc(tasks, func(a, b Task) int { return cmp.Compare(a.Position, b.Position) })
	limit := 0
	if blob.Mark != nil {
		limit = blob.Mark.PracticePosition
	}
	for _, task := range tasks {
		if task.Position <= limit {
			if !explained[task.TheoryBlock] {
				continue
			}
			picked, ok := pickReinforcement(task, blob.Variants, byID, seen, used)
			if !ok {
				warnings = append(warnings, needVariants(task.TypeID, task.Subvariant))
				continue
			}
			used[picked.ID] = true
			reinforcement = append(reinforcement, itemFrom(picked, "reinforcement"))
			continue
		}
		if !explained[task.TheoryBlock] {
			continue
		}
		variant, ok := byID[task.VariantID]
		if !ok {
			continue
		}
		used[variant.ID] = true
		unsolved = append(unsolved, itemFrom(variant, "unsolved"))
	}

	var fresh []pickedItem
	var later []Series
	for _, series := range blob.Recipe {
		switch series.Source {
		case "new_topic":
			taken, warn := takeNewTopic(series, blob.Variants, explained, seen, used)
			if warn {
				warnings = append(warnings, needVariants(series.TypeID, ""))
			}
			fresh = append(fresh, taken...)
		case "review", "ct_format":
			later = append(later, series)
		}
	}

	threshold, discuss := takeThreshold(blob, seen, used)
	var review []draftItem
	var format []draftItem
	for _, series := range later {
		taken, warn := takePlain(series, blob.Variants, seen, used)
		if warn {
			warnings = append(warnings, needVariants(series.TypeID, ""))
		}
		items := itemsFrom(taken, series.Source)
		if series.Source == "ct_format" {
			format = append(format, items...)
			continue
		}
		review = append(review, items...)
	}

	sortByID(reinforcement)
	sortByID(unsolved)
	sortNewTopic(fresh)
	sortByID(threshold)
	sortByID(review)
	sortByID(format)

	items := make([]draftItem, 0, len(unsolved)+len(reinforcement)+len(fresh)+len(threshold)+len(review)+len(format))
	items = append(items, unsolved...)
	items = append(items, reinforcement...)
	for _, item := range fresh {
		items = append(items, item.item)
	}
	items = append(items, threshold...)
	items = append(items, review...)
	items = append(items, format...)

	draft := storedDraft{
		Items:         items,
		Deadline:      deadline,
		DiscussTopics: discuss,
		Warnings:      warnings,
	}
	solved := solvedCount(blob)
	if len(items) > solved*homeworkRatio {
		draft.Hints = []string{hintLine(solved, len(items))}
	}
	return draft
}

func theoryOf(mark *Mark) string {
	if mark == nil {
		return ""
	}
	return mark.Theory
}

func solvedCount(blob lessonBlob) int {
	if blob.Mark == nil {
		return 0
	}
	n := 0
	for _, task := range blob.Tasks {
		if task.Position <= blob.Mark.PracticePosition {
			n++
		}
	}
	return n
}

func hintLine(solved, total int) string {
	return "на уроке решено " + strconv.Itoa(solved) + ", в ДЗ " + strconv.Itoa(total)
}

func seenIDs(blob lessonBlob) map[string]bool {
	seen := map[string]bool{}
	for _, item := range blob.Variants {
		if item.Seen {
			seen[item.ID] = true
		}
	}
	for _, task := range blob.Tasks {
		seen[task.VariantID] = true
	}
	for _, item := range blob.Past {
		seen[item.VariantID] = true
	}
	return seen
}

func explainedBlocks(blocks []string, theory string) map[string]bool {
	out := map[string]bool{}
	if theory == "" {
		return out
	}
	last := -1
	for i, block := range blocks {
		if block == theory {
			last = i
			break
		}
	}
	if last < 0 {
		return out
	}
	for i := 0; i <= last; i++ {
		out[blocks[i]] = true
	}
	return out
}

func pickReinforcement(task Task, variants []Variant, byID map[string]Variant, seen, used map[string]bool) (Variant, bool) {
	statement := ""
	if item, ok := byID[task.VariantID]; ok {
		statement = item.Statement
	}
	var found []Variant
	for _, item := range variants {
		if item.TypeID != task.TypeID || item.Subvariant != task.Subvariant {
			continue
		}
		if item.ID == task.VariantID || seen[item.ID] || used[item.ID] || item.CheckpointReserve {
			continue
		}
		if item.Statement == statement {
			continue
		}
		found = append(found, item)
	}
	if len(found) == 0 {
		return Variant{}, false
	}
	slices.SortFunc(found, func(a, b Variant) int { return cmp.Compare(a.ID, b.ID) })
	return found[0], true
}

func takeNewTopic(series Series, variants []Variant, explained map[string]bool, seen, used map[string]bool) ([]pickedItem, bool) {
	var pool []Variant
	for _, item := range variants {
		if item.TypeID == series.TypeID && explained[item.TheoryBlock] {
			pool = append(pool, item)
		}
	}
	if len(pool) == 0 {
		return nil, false
	}
	free := freeVariants(pool, seen, used, func(Variant) bool { return true })
	if series.Count > 0 && len(free) == 0 {
		return nil, true
	}
	return pickedFrom(takeN(free, series.Count), "new_topic", used), false
}

func takePlain(series Series, variants []Variant, seen, used map[string]bool) ([]Variant, bool) {
	free := freeVariants(variants, seen, used, func(item Variant) bool {
		if item.TypeID != series.TypeID {
			return false
		}
		return series.Topic == "" || item.Topic == series.Topic
	})
	if series.Count > 0 && len(free) == 0 {
		return nil, true
	}
	taken := takeN(free, series.Count)
	for _, item := range taken {
		used[item.ID] = true
	}
	return taken, false
}

func takeThreshold(blob lessonBlob, seen, used map[string]bool) ([]draftItem, []string) {
	type tally struct {
		full  int
		total int
	}
	stats := map[string]*tally{}
	for _, item := range blob.Past {
		row := stats[item.Topic]
		if row == nil {
			row = &tally{}
			stats[item.Topic] = row
		}
		row.total++
		if item.FirstPoints == item.MaxPoints {
			row.full++
		}
	}
	topics := make([]string, 0, len(stats))
	for topic := range stats {
		topics = append(topics, topic)
	}
	slices.Sort(topics)

	var items []draftItem
	var discuss []string
	for _, topic := range topics {
		row := stats[topic]
		// Ниже 80 %, если full*5 < total*4. Ниже 60 %, если full*5 < total*3.
		if row.full*5 < row.total*4 {
			free := freeVariants(blob.Variants, seen, used, func(item Variant) bool {
				return item.Topic == topic
			})
			for _, item := range takeN(free, row.total-row.full) {
				used[item.ID] = true
				items = append(items, itemFrom(item, "threshold"))
			}
		}
		if row.full*5 < row.total*3 {
			discuss = append(discuss, topic)
		}
	}
	return items, discuss
}

func freeVariants(variants []Variant, seen, used map[string]bool, match func(Variant) bool) []Variant {
	var found []Variant
	for _, item := range variants {
		if seen[item.ID] || used[item.ID] || item.CheckpointReserve || !match(item) {
			continue
		}
		found = append(found, item)
	}
	slices.SortFunc(found, func(a, b Variant) int { return cmp.Compare(a.ID, b.ID) })
	return found
}

func takeN(variants []Variant, n int) []Variant {
	if n < 0 {
		n = 0
	}
	if n > len(variants) {
		n = len(variants)
	}
	return variants[:n]
}

type pickedItem struct {
	item       draftItem
	difficulty int
}

func pickedFrom(variants []Variant, source string, used map[string]bool) []pickedItem {
	out := make([]pickedItem, len(variants))
	for i, item := range variants {
		used[item.ID] = true
		out[i] = pickedItem{item: itemFrom(item, source), difficulty: item.Difficulty}
	}
	return out
}

func itemsFrom(variants []Variant, source string) []draftItem {
	out := make([]draftItem, len(variants))
	for i, item := range variants {
		out[i] = itemFrom(item, source)
	}
	return out
}

func itemFrom(item Variant, source string) draftItem {
	return draftItem{
		VariantID:    item.ID,
		TypeID:       item.TypeID,
		AnswerFormat: item.AnswerFormat,
		Minutes:      item.Minutes,
		Source:       source,
		Statement:    item.Statement,
	}
}

func needVariants(typeID, subvariant string) warningView {
	text := "variants_needed " + typeID
	params := map[string]any{"type_id": typeID}
	if subvariant != "" {
		text += " " + subvariant
		params["subvariant"] = subvariant
	}
	return warningView{Type: "variants_needed", Text: text, Params: params}
}

func sortByID(items []draftItem) {
	slices.SortFunc(items, func(a, b draftItem) int { return cmp.Compare(a.VariantID, b.VariantID) })
}

func sortNewTopic(items []pickedItem) {
	slices.SortFunc(items, func(a, b pickedItem) int {
		if c := cmp.Compare(a.difficulty, b.difficulty); c != 0 {
			return c
		}
		return cmp.Compare(a.item.VariantID, b.item.VariantID)
	})
}
