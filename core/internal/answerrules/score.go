// Пакет answerrules оценивает ответ ученика по правилам каталога.
// Score повторяет tools/examples/check_answer_rules.py на строках таблицы примеров.
package answerrules

import (
	"math/big"
	"regexp"
	"slices"
	"strings"
	"unicode"
)

// Result — исход оценки одного ответа.
// Пустой TrapID значит, что ловушка не сработала.
type Result struct {
	Status     string // scored | malformed
	Normalized string
	Verdict    string // correct | partial | wrong
	Points     int
	TrapID     string
}

const (
	formatChoice1 = "ct.choice1"
	formatChoiceN = "ct.choiceN"
	formatMatch   = "ct.match"
	formatDigits  = "ct.digits"
	formatInteger = "ct.integer"
)

var (
	intPattern    = regexp.MustCompile(`^-?[0-9]+$`)
	digitsPattern = regexp.MustCompile(`^[0-9]+$`)
	matchPattern  = regexp.MustCompile("^([\u0410-\u042F\u0401][0-9])+$")
	pairPattern   = regexp.MustCompile("([\u0410-\u042F\u0401])([0-9])")

	// latinToCyr — латинские двойники кириллицы после приведения к верхнему регистру (AR-R9).
	latinToCyr = map[rune]rune{
		'A': '\u0410', // А
		'B': '\u0412', // В
		'C': '\u0421', // С
		'E': '\u0415', // Е
		'H': '\u041D', // Н
		'K': '\u041A', // К
		'M': '\u041C', // М
		'O': '\u041E', // О
		'P': '\u0420', // Р
		'T': '\u0422', // Т
		'X': '\u0425', // Х
	}
)

// Score оценивает сырой ввод. key: choice — []int, integer — int, digits и match — string.
// raw: choice — []int, иначе string. Неизвестный format вызывает панику.
func Score(format string, key any, raw any, traps map[string]string) Result {
	switch format {
	case formatChoice1:
		return scoreChoice(true, key, raw, traps)
	case formatChoiceN:
		return scoreChoice(false, key, raw, traps)
	case formatInteger:
		return scoreInteger(key, raw, traps)
	case formatDigits:
		return scoreDigits(key, raw, traps)
	case formatMatch:
		return scoreMatch(key, raw, traps)
	default:
		panic("answerrules: unknown format " + format)
	}
}

func scoreChoice(single bool, key, raw any, traps map[string]string) Result {
	ks, ok := key.([]int)
	if !ok {
		panic("answerrules: choice key")
	}
	xs, ok := raw.([]int)
	if !ok {
		panic("answerrules: choice raw")
	}
	if len(xs) == 0 || (single && len(xs) != 1) {
		return malformed()
	}
	for _, x := range xs {
		if x < 1 || x > 5 {
			return malformed()
		}
	}
	norm := uniqueSorted(xs)
	shown := joinInts(norm)
	verdict := "wrong"
	if slices.Equal(norm, sortedInts(ks)) {
		verdict = "correct"
	}
	format := formatChoiceN
	if single {
		format = formatChoice1
	}
	return scored(format, shown, verdict, traps)
}

func scoreInteger(key, raw any, traps map[string]string) Result {
	k, ok := key.(int)
	if !ok {
		panic("answerrules: integer key")
	}
	s, ok := raw.(string)
	if !ok {
		panic("answerrules: integer raw")
	}
	norm, ok := canonicalInteger(s)
	if !ok {
		return malformed()
	}
	verdict := "wrong"
	if norm == itoa(k) {
		verdict = "correct"
	}
	return scored(formatInteger, norm, verdict, traps)
}

func scoreDigits(key, raw any, traps map[string]string) Result {
	k, ok := key.(string)
	if !ok {
		panic("answerrules: digits key")
	}
	s, ok := raw.(string)
	if !ok {
		panic("answerrules: digits raw")
	}
	norm, ok := canonicalDigits(s)
	if !ok {
		return malformed()
	}
	verdict := "wrong"
	switch symDiffSize(norm, k) {
	case 0:
		verdict = "correct"
	case 1:
		verdict = "partial"
	}
	return scored(formatDigits, norm, verdict, traps)
}

func scoreMatch(key, raw any, traps map[string]string) Result {
	k, ok := key.(string)
	if !ok {
		panic("answerrules: match key")
	}
	s, ok := raw.(string)
	if !ok {
		panic("answerrules: match raw")
	}
	norm, ok := canonicalMatch(s, k)
	if !ok {
		return malformed()
	}
	verdict := "wrong"
	switch mismatchCount(norm, k) {
	case 0:
		verdict = "correct"
	case 1:
		verdict = "partial"
	}
	return scored(formatMatch, norm, verdict, traps)
}

func canonicalInteger(raw string) (string, bool) {
	s := foldMinus(stripSpace(raw))
	s = strings.TrimPrefix(s, "+")
	if !intPattern.MatchString(s) {
		return "", false
	}
	n := new(big.Int)
	if _, ok := n.SetString(s, 10); !ok {
		return "", false
	}
	return n.String(), true
}

func canonicalDigits(raw string) (string, bool) {
	s := foldMinus(stripSpace(raw))
	if !digitsPattern.MatchString(s) || hasDupRune(s) {
		return "", false
	}
	rs := []rune(s)
	slices.Sort(rs)
	return string(rs), true
}

func canonicalMatch(raw, key string) (string, bool) {
	s := latinLookalikes(strings.ToUpper(foldMinus(stripSpace(raw))))
	if !matchPattern.MatchString(s) {
		return "", false
	}
	pairs := pairPattern.FindAllStringSubmatch(s, -1)
	letters := make([]rune, len(pairs))
	for i, p := range pairs {
		letters[i] = []rune(p[1])[0]
	}
	if !slices.Equal(sortedRunes(letters), sortedRunes(taskLetters(key))) {
		return "", false
	}
	slices.SortFunc(pairs, func(a, b []string) int {
		if a[1] != b[1] {
			return strings.Compare(a[1], b[1])
		}
		return strings.Compare(a[2], b[2])
	})
	var b strings.Builder
	for _, p := range pairs {
		b.WriteString(p[1])
		b.WriteString(p[2])
	}
	return b.String(), true
}

func mismatchCount(norm, key string) int {
	got := pairMap(norm)
	want := pairMap(key)
	wrong := 0
	for letter, digit := range want {
		if got[letter] != digit {
			wrong++
		}
	}
	return wrong
}

func pairMap(s string) map[string]string {
	ms := pairPattern.FindAllStringSubmatch(s, -1)
	out := make(map[string]string, len(ms))
	for _, m := range ms {
		out[m[1]] = m[2]
	}
	return out
}

func scored(format, shown, verdict string, traps map[string]string) Result {
	res := Result{
		Status:     "scored",
		Normalized: shown,
		Verdict:    verdict,
		Points:     pointsFor(format, verdict),
	}
	if verdict == "wrong" {
		if id, ok := traps[shown]; ok {
			res.TrapID = id
		}
	}
	return res
}

func malformed() Result {
	return Result{Status: "malformed"}
}

func pointsFor(format, verdict string) int {
	switch verdict {
	case "correct":
		switch format {
		case formatChoice1, formatChoiceN:
			return 1
		case formatMatch, formatDigits, formatInteger:
			return 2
		default:
			panic("answerrules: unknown format " + format)
		}
	case "partial":
		return 1
	default:
		return 0
	}
}

func stripSpace(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, s)
}

func foldMinus(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '\u2212', '\u2013', '\u2010':
			return '-'
		default:
			return r
		}
	}, s)
}

func latinLookalikes(s string) string {
	return strings.Map(func(r rune) rune {
		if c, ok := latinToCyr[r]; ok {
			return c
		}
		return r
	}, s)
}

func taskLetters(s string) []rune {
	var out []rune
	for _, r := range s {
		if (r >= '\u0410' && r <= '\u042F') || r == '\u0401' {
			out = append(out, r)
		}
	}
	return out
}

func hasDupRune(s string) bool {
	seen := make(map[rune]struct{}, len(s))
	for _, r := range s {
		if _, ok := seen[r]; ok {
			return true
		}
		seen[r] = struct{}{}
	}
	return false
}

func symDiffSize(a, b string) int {
	inA := make(map[rune]bool)
	inB := make(map[rune]bool)
	for _, r := range a {
		inA[r] = true
	}
	for _, r := range b {
		inB[r] = true
	}
	n := 0
	for r := range inA {
		if !inB[r] {
			n++
		}
	}
	for r := range inB {
		if !inA[r] {
			n++
		}
	}
	return n
}

func uniqueSorted(xs []int) []int {
	seen := make(map[int]struct{}, len(xs))
	out := make([]int, 0, len(xs))
	for _, x := range xs {
		if _, ok := seen[x]; ok {
			continue
		}
		seen[x] = struct{}{}
		out = append(out, x)
	}
	slices.Sort(out)
	return out
}

func sortedInts(xs []int) []int {
	out := append([]int(nil), xs...)
	slices.Sort(out)
	return out
}

func sortedRunes(rs []rune) []rune {
	out := append([]rune(nil), rs...)
	slices.Sort(out)
	return out
}

func joinInts(xs []int) string {
	var b strings.Builder
	for i, x := range xs {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(itoa(x))
	}
	return b.String()
}

func itoa(n int) string {
	return new(big.Int).SetInt64(int64(n)).String()
}
