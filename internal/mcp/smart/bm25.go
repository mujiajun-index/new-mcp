package smart

import (
	"math"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

const (
	k1 = 1.2
	b  = 0.75
)

type SearchDoc struct {
	ID          string
	Type        string // "mcp" or "tool"
	Name        string
	Description string
	GroupName   string
	ServiceName string
	ToolCount   int
}

type SearchResult struct {
	Doc   SearchDoc
	Score float64
}

type bm25Index struct {
	docs       []SearchDoc
	termFreqs  map[string]map[int]int // term -> {docIdx: freq}
	docLens    []int
	avgDocLen  float64
	docCount   int
	fieldBoost map[string]float64
}

func buildIndex(docs []SearchDoc) *bm25Index {
	idx := &bm25Index{
		docs:      docs,
		termFreqs: make(map[string]map[int]int),
		fieldBoost: map[string]float64{
			"name":         3.0,
			"service_name": 2.0,
			"description":  1.0,
		},
	}

	totalLen := 0
	for i, doc := range docs {
		fields := map[string]string{
			"name":         doc.Name,
			"service_name": doc.ServiceName,
			"description":  doc.Description,
		}

		docTermCount := 0
		for field, text := range fields {
			tokens := tokenize(text)
			boost := int(idx.fieldBoost[field] * 10) // scale to avoid float precision issues
			for _, t := range tokens {
				if idx.termFreqs[t] == nil {
					idx.termFreqs[t] = make(map[int]int)
				}
				idx.termFreqs[t][i] += boost
			}
			docTermCount += len(tokens)
		}

		idx.docLens = append(idx.docLens, docTermCount)
		totalLen += docTermCount
	}

	idx.docCount = len(docs)
	if idx.docCount > 0 {
		idx.avgDocLen = float64(totalLen) / float64(idx.docCount)
	}

	return idx
}

func (idx *bm25Index) search(query string, limit int) []SearchResult {
	queryTerms := tokenize(query)
	if len(queryTerms) == 0 {
		return nil
	}

	// Expand with fuzzy matches
	expanded := make(map[string]bool)
	for _, t := range queryTerms {
		expanded[t] = true
		for term := range idx.termFreqs {
			if levenshtein(t, term) <= 1 && len(t) > 2 {
				expanded[term] = true
			}
		}
	}

	scores := make(map[int]float64)
	for term := range expanded {
		docFreqs, ok := idx.termFreqs[term]
		if !ok {
			continue
		}
		df := float64(len(docFreqs))
		idf := math.Log(1 + (float64(idx.docCount)-df+0.5)/(df+0.5))

		for docIdx, tf := range docFreqs {
			tfFloat := float64(tf)
			dl := float64(idx.docLens[docIdx])
			tfNorm := (tfFloat * (k1 + 1)) / (tfFloat + k1*(1-b+b*dl/idx.avgDocLen))
			scores[docIdx] += idf * tfNorm
		}
	}

	type scored struct {
		idx   int
		score float64
	}
	var results []scored
	for docIdx, score := range scores {
		results = append(results, scored{docIdx, score})
	}

		// Sort by score descending
		sort.Slice(results, func(i, j int) bool {
			return results[i].score > results[j].score
		})

	if limit > len(results) {
		limit = len(results)
	}

	out := make([]SearchResult, limit)
	for i := 0; i < limit; i++ {
		out[i] = SearchResult{
			Doc:   idx.docs[results[i].idx],
			Score: results[i].score,
		}
	}
	return out
}

func tokenize(text string) []string {
	text = strings.ToLower(text)
	// Split CJK characters individually
	var tokens []string
	var current []rune

	for _, r := range text {
		if unicode.Is(unicode.Han, r) {
			if len(current) > 0 {
				tokens = append(tokens, string(current))
				current = nil
			}
			tokens = append(tokens, string(r))
		} else if unicode.IsLetter(r) || unicode.IsDigit(r) {
			current = append(current, r)
		} else {
			if len(current) > 0 {
				tokens = append(tokens, string(current))
				current = nil
			}
		}
	}
	if len(current) > 0 {
		tokens = append(tokens, string(current))
	}

	// Filter short tokens
	filtered := make([]string, 0, len(tokens))
	for _, t := range tokens {
		if len(t) >= 2 || regexp.MustCompile(`\p{Han}`).MatchString(t) {
			filtered = append(filtered, t)
		}
	}
	return filtered
}

func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	la, lb := len(ra), len(rb)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	prev := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}

	for i := 1; i <= la; i++ {
		curr := make([]int, lb+1)
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			curr[j] = min(
				prev[j]+1,
				curr[j-1]+1,
				prev[j-1]+cost,
			)
		}
		prev = curr
	}
	return prev[lb]
}

func min(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}
