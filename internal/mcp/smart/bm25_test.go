package smart

import (
	"reflect"
	"testing"
)

func tokenSet(tokens []string) map[string]bool {
	set := make(map[string]bool, len(tokens))
	for _, t := range tokens {
		set[t] = true
	}
	return set
}

func TestTokenizeLatin(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"hyphenated name", "bazi-mcp", []string{"bazi", "mcp"}},
		{"mixed case lowercased", "BaZi Fortune", []string{"bazi", "fortune"}},
		{"plain words", "weather forecast 2026", []string{"weather", "forecast", "2026"}},
		{"single letters dropped", "a b c", nil},
		{"punctuation only", "!!! ...", nil},
		{"empty", "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tokenize(tt.input)
			if tt.want == nil {
				if len(got) != 0 {
					t.Fatalf("tokenize(%q) = %v, want empty", tt.input, got)
				}
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("tokenize(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// TestTokenizeHan 汉字 run 三层展开:unigram + bigram + 拼音(音节/二元连写/整段连写)。
func TestTokenizeHan(t *testing.T) {
	got := tokenSet(tokenize("八字排盘"))
	want := map[string]bool{
		// unigram
		"八": true, "字": true, "排": true, "盘": true,
		// bigram
		"八字": true, "字排": true, "排盘": true,
		// 每字拼音音节
		"ba": true, "zi": true, "pai": true, "pan": true,
		// 二元连写拼音
		"bazi": true, "zipai": true, "paipan": true,
		// 整段连写拼音(run 长度 4 ≤ 8)
		"bazipaipan": true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tokenize(\"八字排盘\") got %v, want %v", got, want)
	}
}

// TestTokenizeMixed 中英混排:两种 run 各自展开,互不粘连(整段连写只限连续
// run 内,「八字」「排盘」被拉丁词隔开,不产出 bazipaipan)。
func TestTokenizeMixed(t *testing.T) {
	got := tokenSet(tokenize("八字 BaZi 排盘"))
	for _, want := range []string{"八", "八字", "bazi", "排盘", "paipan"} {
		if !got[want] {
			t.Fatalf("tokenize(\"八字 BaZi 排盘\") missing token %q, got %v", want, got)
		}
	}
	if got["bazipaipan"] {
		t.Fatalf("whole-run joined pinyin must not cross runs: bazipaipan generated")
	}
}

// TestTokenizeLongRunNoJoinedPinyin 超长(>8 字)汉字 run 不生成整段连写拼音,
// 单字/二元层照常。整段连写会是几十字符的长串,用长度上限断言其不存在。
func TestTokenizeLongRunNoJoinedPinyin(t *testing.T) {
	got := tokenSet(tokenize("这是一段相当长的中文句子用来验证超长整段连写不生成"))
	for tok := range got {
		if len(tok) > 14 {
			t.Fatalf("unexpected long token %q — whole-run joined pinyin should be capped", tok)
		}
	}
	if !got["中文"] || !got["zhong"] || !got["zhongwen"] {
		t.Fatalf("long run should still emit unigram/bigram/per-char syllables, got %v", got)
	}
}

// bug 复现:目录字段全英文、零汉字,中文 query「八字」经拼音桥命中 bazi-mcp。
func TestSearchChineseQueryHitsEnglishCatalog(t *testing.T) {
	docs := []SearchDoc{
		{Type: "mcp", Name: "BaZi Fortune", ServiceName: "bazi-mcp", Description: "Chinese Eight Characters (Four Pillars) fortune telling"},
		{Type: "mcp", Name: "Weather", ServiceName: "weather", Description: "Weather forecasts for cities"},
		{Type: "mcp", Name: "Calculator", ServiceName: "calculator", Description: "Basic math operations"},
	}
	idx := buildIndex(docs)
	results, total := idx.search("八字", 0, 20)
	if total == 0 || len(results) == 0 {
		t.Fatalf("query 八字 got 0 results, want bazi-mcp matched via pinyin")
	}
	if results[0].Doc.ServiceName != "bazi-mcp" {
		t.Fatalf("query 八字 top result = %q, want bazi-mcp", results[0].Doc.ServiceName)
	}
}

// 反向:英文 query「bazi」命中纯中文目录(索引侧拼音)。
func TestSearchEnglishQueryHitsChineseCatalog(t *testing.T) {
	docs := []SearchDoc{
		{Type: "mcp", Name: "八字排盘", ServiceName: "bazi-paipan", Description: "八字命理、紫微斗数预测"},
		{Type: "mcp", Name: "天气查询", ServiceName: "weather", Description: "查询城市天气"},
	}
	idx := buildIndex(docs)
	results, total := idx.search("bazi", 0, 20)
	if total == 0 {
		t.Fatalf("query bazi got 0 results, want 八字排盘 matched via index-side pinyin")
	}
	if results[0].Doc.Name != "八字排盘" {
		t.Fatalf("query bazi top result = %q, want 八字排盘", results[0].Doc.Name)
	}
}

// 精度:query「八字」时,含字面词组的文档应排在只散见单字的无关文档之前。
func TestSearchHanBigramPrecision(t *testing.T) {
	docs := []SearchDoc{
		{Type: "tool", Name: "ocr_recognize", ServiceName: "ocr", Description: "文字识别,从图片提取文本"},
		{Type: "tool", Name: "bazi_paipan", ServiceName: "bazi", Description: "八字排盘,输出四柱八字命盘"},
	}
	idx := buildIndex(docs)
	results, _ := idx.search("八字", 0, 20)
	if len(results) < 1 || results[0].Doc.Name != "bazi_paipan" {
		t.Fatalf("query 八字 top result = %+v, want bazi_paipan", results)
	}
}

// 回归:纯英文检索行为不变;单汉字查询仍可用;纯标点 query 零结果。
func TestSearchRegression(t *testing.T) {
	docs := []SearchDoc{
		{Type: "mcp", Name: "Weather", ServiceName: "weather", Description: "Weather forecasts"},
		{Type: "tool", Name: "get_weather", ServiceName: "weather", Description: "Get current weather 天气查询"},
	}
	idx := buildIndex(docs)

	if results, total := idx.search("weather forecast", 0, 20); total == 0 || results[0].Doc.ServiceName != "weather" {
		t.Fatalf("english query regression: total=%d results=%+v", total, results)
	}
	if _, total := idx.search("天", 0, 20); total == 0 {
		t.Fatalf("single han char query should match 天气 via unigram")
	}
	if results, total := idx.search("!!!", 0, 20); total != 0 || results != nil {
		t.Fatalf("punctuation-only query should return nothing, got total=%d", total)
	}
}
