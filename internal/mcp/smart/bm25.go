package smart

import (
	"math"
	"sort"
	"strings"
	"unicode"

	"github.com/mozillazg/go-pinyin"
)

const (
	k1 = 1.2
	b  = 0.75
)

type SearchDoc struct {
	ID          string
	Type        string // "mcp", "tool", "resource", "template", "prompt"
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

// search 返回 [offset, offset+limit) 区间的得分降序结果,以及匹配文档总数
// (切片前统计,供调用方输出"共 N 条 / 下一页 offset"的分页头部)。
// offset 由调用方保证 ≥0。
func (idx *bm25Index) search(query string, offset, limit int) ([]SearchResult, int) {
	queryTerms := tokenize(query)
	if len(queryTerms) == 0 {
		return nil, 0
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

	// Sort by score descending. 同分按文档序号破平:scores 来自 map 遍历,不破平
	// 则同分条目在两次请求间顺序随机,offset 翻页时会在两页间漂移(漏条/重复)。
	sort.Slice(results, func(i, j int) bool {
		if results[i].score != results[j].score {
			return results[i].score > results[j].score
		}
		return results[i].idx < results[j].idx
	})

	total := len(results)
	start, end := offset, offset+limit
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}

	out := make([]SearchResult, 0, end-start)
	for i := start; i < end; i++ {
		out = append(out, SearchResult{
			Doc:   idx.docs[results[i].idx],
			Score: results[i].score,
		})
	}
	return out, total
}

// pinyinArgs 无声调小写拼音参数(Normal 样式,每字取第一个读音);只读,可并发共享。
var pinyinArgs = pinyin.NewArgs()

// hanRunMaxJoinedPinyin 整段连写拼音只对长度 ≤ 此值的汉字 run 生成:整句中文的
// 连写串没有检索价值,只会无谓膨胀索引(每字音节与二元连写不受此限)。
const hanRunMaxJoinedPinyin = 8

// tokenize 把文本切成检索 token,建索引(buildIndex)与查询(search)共用,保证
// 两侧对称。拉丁/数字按连续段成一个 token;连续汉字段展开为三层:
//  1. 单字 unigram —— 单字查询仍可命中;
//  2. 相邻二元 bigram —— 中文查询精度:query「八字」优先命中含该词组的文档,
//     而非只散见「字」的无关文档(如「文字识别」);
//  3. 拼音形式(每字音节 + 二元连写 + 整段连写)—— 打通中英鸿沟:query「八字」
//     产出 "bazi" 可命中纯英文目录 bazi-mcp,反向英文 query 命中中文描述亦然。
//     无读音的生僻字该位置跳过;拼音 token 去重(与 unigram 的 tf 语义无关,
//     重复音节只会虚增 docLens)。
// 拼音层使汉字文档的 token 数约为字数的 3 倍,docLen 相应膨胀——百级文档规模下
// 对 BM25 长度归一的影响可接受,换取跨语言召回。
func tokenize(text string) []string {
	text = strings.ToLower(text)
	var tokens []string
	var current []rune // 拉丁/数字 run
	var han []rune     // 汉字 run

	flushLatin := func() {
		if len(current) > 0 {
			tokens = append(tokens, string(current))
			current = nil
		}
	}
	flushHan := func() {
		if len(han) > 0 {
			tokens = append(tokens, hanRunTokens(han)...)
			han = nil
		}
	}

	for _, r := range text {
		switch {
		case unicode.Is(unicode.Han, r):
			flushLatin()
			han = append(han, r)
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			flushHan()
			current = append(current, r)
		default:
			flushLatin()
			flushHan()
		}
	}
	flushLatin()
	flushHan()

	// Filter short tokens(单汉字 3 字节,天然通过;单字母/数字被滤掉)
	filtered := make([]string, 0, len(tokens))
	for _, t := range tokens {
		if len(t) >= 2 || containsHan(t) {
			filtered = append(filtered, t)
		}
	}
	return filtered
}

// hanRunTokens 展开一段连续汉字(纯 Han run,无空格杂质)为三层 token,详见
// tokenize 的层说明。
func hanRunTokens(run []rune) []string {
	n := len(run)
	tokens := make([]string, 0, n*3)
	for i, r := range run {
		tokens = append(tokens, string(r))
		if i+1 < n {
			tokens = append(tokens, string(run[i:i+2]))
		}
	}

	// 拼音层:逐字取第一个读音,无读音(生僻字/造字)记空串并跳过相关连写,
	// 避免跨缺音位置拼出 "ba"+"pan" → "bapan" 这类假连写。
	syllables := make([]string, n)
	for i, r := range run {
		if pys := pinyin.SinglePinyin(r, pinyinArgs); len(pys) > 0 {
			syllables[i] = pys[0]
		}
	}

	seen := make(map[string]bool)
	addPinyin := func(s string) {
		if s != "" && !seen[s] {
			seen[s] = true
			tokens = append(tokens, s)
		}
	}
	allPresent := true
	for _, s := range syllables {
		if s == "" {
			allPresent = false
		}
		addPinyin(s)
	}
	for i := 0; i+1 < n; i++ {
		if syllables[i] != "" && syllables[i+1] != "" {
			addPinyin(syllables[i] + syllables[i+1])
		}
	}
	if n <= hanRunMaxJoinedPinyin && allPresent {
		addPinyin(strings.Join(syllables, ""))
	}
	return tokens
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
