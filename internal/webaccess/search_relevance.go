package webaccess

import (
	"strings"
	"unicode"
)

var genericASCIISearchTerms = map[string]struct{}{
	"a": {}, "an": {}, "and": {}, "best": {}, "com": {}, "current": {},
	"for": {}, "from": {}, "in": {}, "latest": {}, "most": {}, "net": {},
	"of": {}, "official": {}, "on": {}, "or": {}, "org": {}, "popular": {},
	"site": {}, "the": {}, "to": {}, "top": {}, "trending": {}, "website": {},
	"with": {}, "www": {},
}

var genericCJKSearchPhrases = []string{
	"排行榜", "当前", "现在", "最新", "最热", "热门", "排行", "排名", "榜单", "推荐", "官网", "网站",
}

type searchQueryTerm struct {
	value      string
	exactToken bool
}

// searchResultsCoverQuery rejects structurally valid but clearly unrelated
// Bing RSS feeds. Bing occasionally collapses a multi-term request to only its
// first generic word; treating that response as success poisons model context
// and prevents the other free provider from contributing useful evidence.
func searchResultsCoverQuery(query string, results []SearchResult) bool {
	terms := searchQueryTerms(query)
	if len(terms) == 0 {
		return true
	}
	var corpus strings.Builder
	for _, result := range results {
		corpus.WriteString(" ")
		corpus.WriteString(result.Title)
		corpus.WriteString(" ")
		corpus.WriteString(result.Summary)
		corpus.WriteString(" ")
		corpus.WriteString(result.URL)
	}
	text := strings.ToLower(corpus.String())
	asciiTokens := searchASCIITokenSet(text)
	matched := 0
	for _, term := range terms {
		_, exactMatch := asciiTokens[term.value]
		if (term.exactToken && exactMatch) || (!term.exactToken && strings.Contains(text, term.value)) {
			matched++
		}
	}
	required := (len(terms) + 3) / 4
	if len(terms) > 1 && required < 2 {
		required = 2
	}
	if required > 3 {
		required = 3
	}
	return matched >= required
}

func searchQueryTerms(query string) []searchQueryTerm {
	query = strings.ToLower(query)
	for _, phrase := range genericCJKSearchPhrases {
		query = strings.ReplaceAll(query, phrase, " ")
	}
	seen := make(map[string]struct{})
	terms := make([]searchQueryTerm, 0, 8)
	appendTerm := func(term string, exactToken bool) {
		term = strings.TrimSpace(term)
		if term == "" {
			return
		}
		if _, generic := genericASCIISearchTerms[term]; generic {
			return
		}
		key := term
		if exactToken {
			key = "token:" + key
		} else {
			key = "text:" + key
		}
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		terms = append(terms, searchQueryTerm{value: term, exactToken: exactToken})
	}

	var ascii []rune
	var cjk []rune
	flushASCII := func() {
		if len(ascii) > 0 {
			appendTerm(string(ascii), true)
			ascii = ascii[:0]
		}
	}
	flushCJK := func() {
		if len(cjk) == 1 {
			appendTerm(string(cjk), false)
		} else {
			for index := 0; index+1 < len(cjk); index++ {
				appendTerm(string(cjk[index:index+2]), false)
			}
		}
		cjk = cjk[:0]
	}
	for _, character := range query {
		switch {
		case unicode.In(character, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul):
			flushASCII()
			cjk = append(cjk, character)
		case unicode.IsLetter(character) || unicode.IsDigit(character):
			flushCJK()
			ascii = append(ascii, character)
		default:
			flushASCII()
			flushCJK()
		}
	}
	flushASCII()
	flushCJK()
	return terms
}

func searchASCIITokenSet(text string) map[string]struct{} {
	tokens := make(map[string]struct{})
	var current []rune
	flush := func() {
		if len(current) > 0 {
			tokens[string(current)] = struct{}{}
			current = current[:0]
		}
	}
	for _, character := range text {
		if !unicode.In(character, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul) &&
			(unicode.IsLetter(character) || unicode.IsDigit(character)) {
			current = append(current, character)
			continue
		}
		flush()
	}
	flush()
	return tokens
}
