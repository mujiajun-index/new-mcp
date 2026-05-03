# MCP 网关平台搜索方案调研

## 场景概述

| 维度 | 数据 |
|------|------|
| MCP 服务数量 | ~10,000 |
| 每服务的工具数 | 3-10 |
| 可搜索文档总量 | 30K - 100K |
| 用户可见范围 | 用户自有服务 (5-500) + 所有公开服务 |
| 字段权重 | 服务名 3x, 工具名 2x, 描述 1x |
| 语言支持 | 中文 + 英文 |
| 匹配模式 | 前缀匹配、模糊匹配 |
| 延迟要求 | < 100ms |
| 更新频率 | 低 (服务增删不频繁) |

---

## 方案 1: SQLite FTS5

### BM25 字段权重 -- 原生支持

FTS5 内置 `bm25()` 辅助函数，接受可变参数作为每列权重：

```sql
-- 假设 schema: service_name, tool_name, description
CREATE VIRTUAL TABLE mcp_search USING fts5(
    service_name, tool_name, description,
    tokenize = '...'  -- 见下方中文方案
);

-- 权重: service_name=3.0, tool_name=2.0, description=1.0
SELECT rowid, *
FROM mcp_search
WHERE mcp_search MATCH ?
ORDER BY bm25(mcp_search, 3.0, 2.0, 1.0)
LIMIT 20;
```

`bm25()` 第一个参数后依次为从左到右每列的权重，缺省为 1.0。乘以 -1 保证更好的匹配返回更小的值，直接 `ORDER BY bm25(...)` 即可从优到劣排序。

**结论：原生完美支持字段权重 BM25。**

### 中文分词 -- 核心问题，有解决方案

**问题根源：** FTS5 默认 `unicode61` 分词器按空格/标点切分，CJK 字符属于 Unicode 类别 `Lo`（Letter, other），不在默认的 token 类别 `L* N* Co` 范围内（`L*` 展开为 `Lu Ll Lt Lm Lo`... 实际上 unicode61 默认已包含 Lo）。真正的问题是：中文没有空格分隔词语，`unicode61` 把整段中文字符当作一个 token，导致无法搜索单个中文词。

**解决方案（三选一）：**

| 方案 | 实现方式 | 效果 | 复杂度 |
|------|----------|------|--------|
| **A. unicode61 categories 扩展** | `tokenize="unicode61 categories 'L* N* Co'"` | 每个 CJK 字符为独立 token (unigram) | 极低，但粒度粗 |
| **B. simple tokenizer 扩展** | 使用 [wangfenjin/simple](https://github.com/wangfenjin/simple) (C++ 扩展) | 支持中文分词 + 拼音搜索 | 中等，需要编译 C 扩展 |
| **C. trigram tokenizer** | `tokenize="trigram"` | 三字符滑动窗口，支持子串匹配 | 极低，但索引体积大 |

**推荐方案 B (simple tokenizer)：**

- 792 stars, 活跃维护 (v0.7.1, 2026-02)
- 支持中文按字分词 + 拼音搜索 (含多音字)
- 集成 jieba 分词器用于更精准的词组匹配
- 提供 `simple_query()` 自动构造 FTS5 查询
- 提供 `simple_highlight()` 连续高亮
- 缺点：需要编译 C/C++ 共享库 (~几 MB)

**最简方案 A (unigram，零依赖)：**

如果接受"逐字匹配"，可直接用 unicode61 + categories：

```sql
-- unicode61 默认 categories 已是 "L* N* Co"，包含 Lo (CJK 字符)
-- 但问题是整段中文被当作一个 token
-- 需要用 "tokenchars" 或改为逐字切分

-- 实际上最简单的方式是在应用层预处理：
-- 入库时将中文字符间插入空格，使 unicode61 逐字切分
-- 查询时同理处理查询词
```

### 多租户过滤

```sql
-- 方案：先 FTS 搜索，再过滤可见性
SELECT m.* FROM mcp_search m
WHERE m.mcp_search MATCH ?
  AND m.rowid IN (
    SELECT id FROM services
    WHERE owner_id = ? OR visibility = 'public'
  )
ORDER BY bm25(mcp_search, 3.0, 2.0, 1.0)
LIMIT 20;
```

注意：FTS5 表不支持普通 WHERE 条件走索引，需要用子查询/JOIN 方式。对于 100K 文档量级，先 FTS 搜索再过滤的效率是可以接受的。

### 性能预估 (100K 文档)

| 操作 | 预期延迟 |
|------|----------|
| 单词精确搜索 | 1-5 ms |
| 前缀搜索 (需 prefix index) | 5-15 ms |
| 布尔组合查询 | 5-20 ms |
| 带过滤条件的全文搜索 | 5-30 ms |

参考数据：Simonw 的 benchmark 中，100K 行数据 FTS5 单标签查询 3.28ms，AND 查询 2.59ms。FTS5 在百万级文档上 < 50ms。100K 文档远低于此阈值。

需要创建 prefix index 加速前缀搜索：
```sql
CREATE VIRTUAL TABLE mcp_search USING fts5(
    service_name, tool_name, description,
    prefix='2 3 4',   -- 对 2/3/4 字符前缀建索引
    tokenize = '...'
);
```

### 优缺点

| 优点 | 缺点 |
|------|------|
| 已在技术栈中，零额外依赖 | 中文分词需要额外方案 (simple tokenizer 或应用层预处理) |
| 原生 BM25 + 字段权重 | 多租户过滤需要 JOIN/子查询，不能直接走 FTS 索引 |
| 100K 文档轻松应对，延迟 < 30ms | FTS5 不支持模糊匹配 (fuzzy)，只支持前缀 (*) 和精确 |
| 数据一致性好 (同 DB 事务) | 无内置拼写纠错 |
| 索引体积小 (~7MB/100K 文档) | 需要手动维护 content table 同步 (trigger) |

---

## 方案 2: MySQL FULLTEXT

### BM25 与字段权重

MySQL InnoDB FULLTEXT 基于 TF-IDF/BM25 变体，但**不提供显式的字段权重参数**。

```sql
CREATE TABLE mcp_tools (
    id INT PRIMARY KEY AUTO_INCREMENT,
    service_name VARCHAR(255),
    tool_name VARCHAR(255),
    description TEXT,
    owner_id INT,
    visibility ENUM('public', 'private'),
    FULLTEXT ft_idx (service_name, tool_name, description) WITH PARSER ngram
);

-- MySQL 不支持按列设权重，MATCH 的分数是各列得分的简单加和
SELECT *, MATCH(service_name, tool_name, description)
    AGAINST('search_term' IN BOOLEAN MODE) AS score
FROM mcp_tools
WHERE MATCH(service_name, tool_name, description)
    AGAINST('search_term' IN BOOLEAN MODE)
  AND (owner_id = ? OR visibility = 'public')
ORDER BY score DESC
LIMIT 20;
```

**模拟字段权重的方式：**

```sql
-- 方式 1：多次 MATCH 分别赋权 (丑陋但有效)
SELECT *,
    (MATCH(service_name) AGAINST(?) * 3.0 +
     MATCH(tool_name) AGAINST(?) * 2.0 +
     MATCH(description) AGAINST(?)) AS weighted_score
FROM mcp_tools
WHERE MATCH(service_name, tool_name, description) AGAINST(?)
  AND (owner_id = ? OR visibility = 'public')
ORDER BY weighted_score DESC
LIMIT 20;

-- 方式 2：在 BOOLEAN MODE 中用权重运算符
-- 但 < > 权重运算符粒度太粗
```

方式 1 能工作但需要三个 FULLTEXT 索引（每列一个）+ 一个联合索引，索引体积膨胀。

### 中文支持 (ngram parser)

MySQL 5.7.6+ 内置 ngram 分词器，原生支持 CJK：

```sql
-- ngram_token_size=2 (默认，推荐中文场景)
-- 将 "中华人民共和国" 切分为: "中华" "华人" "人民" "民共" "共和" "和国"
CREATE FULLTEXT INDEX ft_idx
ON mcp_tools(service_name, tool_name, description)
WITH PARSER ngram;
```

**ngram 特性：**
- `ngram_token_size` 范围 1-10，默认 2
- 搜索 "abc" 被转换为 "ab bc" 的联合匹配
- 不需要额外编译/插件，MySQL 内置
- 短语搜索被自动转换为 ngram 短语搜索

### 前缀搜索与模糊搜索

```sql
-- 前缀搜索 (BOOLEAN MODE 通配符)
WHERE MATCH(...) AGAINST('搜索词*' IN BOOLEAN MODE)

-- 模糊搜索：MySQL FULLTEXT 不直接支持
-- 可用 LIKE 补充，但走不了 FULLTEXT 索引
WHERE MATCH(...) AGAINST(? IN NATURAL LANGUAGE MODE)
   OR service_name LIKE CONCAT(?, '%')  -- 全表扫描，100K 级别可接受
```

### 多租户过滤

```sql
-- 可以直接在 WHERE 中加条件，MySQL 优化器处理
WHERE MATCH(...) AGAINST(?)
  AND (owner_id = ? OR visibility = 'public')
```

比 SQLite FTS5 更自然，MySQL 优化器能在 FULLTEXT 索引结果上叠加普通索引过滤。

### 性能 (100K 文档)

100K 行对 MySQL FULLTEXT 是极小规模，预期延迟 < 20ms。但需要注意：
- ngram 索引体积较大 (token 数量是原文的 ~N 倍)
- Innodb FT 缓存 (`innodb_ft_cache_size`) 会影响索引更新速度
- 索引更新非实时，有内部缓存刷新延迟

### 优缺点

| 优点 | 缺点 |
|------|------|
| 内置 ngram 中文分词，零额外依赖 | 不支持原生字段权重，需要 hack (多列 MATCH) |
| 多租户 WHERE 条件自然组合 | 模糊搜索 (fuzzy) 不支持 |
| MySQL 生态成熟，运维熟悉 | 索引体积大 (ngram=2 时约为原始数据的 2-3x) |
| 100K 文档规模性能充裕 | 50% 阈值问题 (NATURAL MODE 中出现超50%文档的词被忽略) |
| 事务内数据一致性好 | BOOLEAN MODE 无此限制，但排序方式不同 |

---

## 方案 3: 自定义 Go 倒排索引

### 架构设计

```go
type SearchIndex struct {
    mu           sync.RWMutex
    inverted     map[string][]Posting   // term -> posting list
    docStore     map[uint32]*Document
    fieldWeights map[string]float64     // "service": 3.0, "tool": 2.0, "desc": 1.0
    totalDocs    int
    avgDocLen    float64
}

type Posting struct {
    DocID    uint32
    Freq     map[string]int   // field -> term frequency
    DocLen   map[string]int   // field -> total tokens
}

type Document struct {
    ID          uint32
    ServiceName string
    ToolName    string
    Description string
    OwnerID     uint32
    Visibility  string
    Tokens      map[string][]string  // field -> tokens
}
```

### 内存估算 (100K 文档)

| 组件 | 估算 | 说明 |
|------|------|------|
| 倒排索引 (posting lists) | ~15-30 MB | 假设 100K 文档，平均 50 unique tokens/文档，每个 posting ~20 bytes |
| 文档存储 | ~20-40 MB | 100K 文档 * 200-400 bytes/文档 (元数据 + token 缓存) |
| BM25 统计 | ~2-5 MB | IDF 映射 + 文档长度统计 |
| **总计** | **~40-75 MB** | 完全可接受，可放入内存 |

对比数据：Milvus 项目实测 Go map 的 BM25 统计约 12-20 bytes/entry，100K entry 约需 1.2-2 MB。

### 中文分词处理

需要集成中文分词器，选项：
- **gojieba** (`github.com/yanyiwu/gojieba`)：Go 版结巴分词，最成熟，词典 ~10MB
- **sego** (`github.com/huichen/sego`)：Go 分词器，更轻量
- **简单方案：按字符切分** (unigram)，不需要外部词典

### BM25 字段权重实现

```go
func (idx *SearchIndex) Score(docID uint32, queryTerms []string) float64 {
    score := 0.0
    fields := []string{"service", "tool", "desc"}
    weights := map[string]float64{"service": 3.0, "tool": 2.0, "desc": 1.0}

    for _, term := range queryTerms {
        idf := idx.computeIDF(term)
        for _, field := range fields {
            tf := idx.getTermFreq(docID, field, term)
            docLen := idx.getDocLen(docID, field)
            avgLen := idx.avgFieldLen[field]
            w := weights[field]
            score += w * idf * bm25Tf(tf, docLen, avgLen)
        }
    }
    return score
}
```

### 多租户搜索

```go
func (idx *SearchIndex) Search(query string, userID uint32) []Result {
    terms := tokenize(query)
    candidates := idx.lookup(terms)  // 倒排索引查找

    // 在内存中直接过滤
    visible := filter(candidates, func(d *Document) bool {
        return d.OwnerID == userID || d.Visibility == "public"
    })

    // BM25 字段权重排序
    sort.Slice(visible, func(i, j int) bool {
        return idx.Score(visible[i].ID, terms) > idx.Score(visible[j].ID, terms)
    })
    return visible[:20]
}
```

**这是内存中过滤，速度极快。** 100K 文档的全量扫描在 Go 中只需 < 1ms。

### 开发复杂度

| 模块 | 工作量 | 说明 |
|------|--------|------|
| 倒排索引核心 | 2-3 天 | map + posting list |
| BM25 字段权重评分 | 1-2 天 | 数学公式实现 |
| 前缀匹配 | 1 天 | 前缀树 / 排序 posting list 二分查找 |
| 模糊匹配 | 2-3 天 | Levenshtein 自动机 或简单编辑距离 |
| 中文分词集成 | 1 天 | 集成 gojieba |
| 增量更新 | 1-2 天 | 文档增删的索引维护 |
| 持久化 | 1-2 天 | JSON/gob 序列化到磁盘 |
| **总计** | **~10-14 天** | 功能完整但粗糙 |

### 优缺点

| 优点 | 缺点 |
|------|------|
| 完全掌控，可精确匹配业务需求 | 开发和维护成本高 |
| 内存中过滤多租户，性能极佳 | 需要自己实现所有搜索特性 |
| 无外部依赖 (除分词器) | 模糊匹配/拼写纠错实现复杂 |
| 字段权重完全可定制 | 无持久化保障 (需要自建 WAL 或定期快照) |
| 40-75 MB 内存可接受 | 单进程，无法水平扩展 (但 100K 文档不需要) |
| 可嵌入 Go 二进制 | 测试覆盖需要自己保证 |

---

## 方案 4: github.com/wizenheimer/blaze

### 基本信息

| 项目 | 数据 |
|------|------|
| Stars | 528 |
| 语言 | Go (98.5%) |
| License | MIT |
| 创建时间 | 2025-10-07 |
| 最后推送 | 2025-10-14 |
| 贡献者 | 1 人 |
| 状态 | **教育/实验性** |

### 功能特性

- 倒排索引 + Skip List + Roaring Bitmap
- BM25 排名
- 布尔查询 (AND/OR/NOT)，类型安全的 Query Builder
- 短语搜索
- 位置追踪
- 二进制序列化 (持久化)
- 线程安全 (mutex 保护)

### 适用性评估

| 需求 | 支持情况 |
|------|----------|
| BM25 评分 | 支持 (内置) |
| 字段权重 | **不支持** -- 没有 field-aware 的 BM25，所有文本作为单字段 |
| 中文分词 | **不支持** -- 内置分析器仅支持英文 tokenization/stemming |
| 前缀匹配 | **不支持** -- 仅支持 term/phrase/boolean |
| 模糊匹配 | **不支持** |
| 多租户 | **不支持** -- 无过滤机制，需自己后处理 |
| 持久化 | 支持二进制序列化 |

### 关键问题

1. **作者自己的 CAUTION 声明**："Blaze is an educational implementation. For production use, see Bleve."
2. 单一贡献者，项目仅活跃 1 周 (2025-10-07 ~ 2025-10-14)
3. 无 field-weighted BM25 -- 这是本项目的核心需求
4. 无中文支持
5. 无前缀/模糊匹配

### 结论

**不推荐用于本项目。** Blaze 是一个优秀的教育项目，但缺少字段权重、中文分词、前缀/模糊匹配等关键特性，且作者明确标注为教育用途。

---

## 方案 5: github.com/anyproto/tantivy-go (Tantivy Go 绑定)

### 基本信息

| 项目 | 数据 |
|------|------|
| 库 | `github.com/anyproto/tantivy-go` |
| Stars | 38 |
| License | MIT |
| 最新版本 | v1.0.6 (2026-01-06) |
| 贡献者 | 4 人 |
| 使用场景 | Anytype (开源知识管理工具) 生产环境使用 1+ 年 |
| 底层引擎 | Tantivy (Rust, ~12K stars) |

### 功能特性

| 特性 | 支持情况 |
|------|----------|
| BM25 评分 | 支持 (Tantivy 内置) |
| 字段权重 | 支持 (query-time boost) |
| 中文分词 | 支持 (内置 Jieba tokenizer) |
| 前缀匹配 | 支持 |
| 模糊匹配 | 支持 (Tantivy fuzzy query) |
| 持久化 | 支持 (Tantivy 段式存储) |
| 多线程 | 支持 (库本身线程安全) |

### 架构

通过 CGo 桥接 Rust 编译的静态库 (.a)：

```
Go App -> CGo -> libtantivy_go.a (Rust static lib)
```

支持的平台：Linux (x86_64, ARM64), macOS (x86_64, ARM64), Windows, iOS, Android。

### 关键代码示例

```go
// 内置 tokenizer 常量
const TokenizerJieba = "jieba"     // 中文分词
const TokenizerSimple = "simple_tokenizer"
const TokenizerNgram = "ngram"
const TokenizerEdgeNgram = "edge_ngram"

// 创建 schema 和索引
schema := tantivy.NewSchemaBuilder().
    AddTextField("service_name", tantivy.TextOptions{Tokenizer: TokenizerJieba, Index: true}).
    AddTextField("tool_name", tantivy.TextOptions{Tokenizer: TokenizerJieba, Index: true}).
    AddTextField("description", tantivy.TextOptions{Tokenizer: TokenizerJieba, Index: true}).
    Build()
```

### 缺点与风险

| 风险 | 严重程度 | 说明 |
|------|----------|------|
| CGo 依赖 | 中 | 需要 Rust 交叉编译工具链，构建复杂度增加 |
| 二进制体积 | 中 | Rust 静态库 + Jieba 词典增加 ~10-15 MB |
| 调试困难 | 中 | 跨 Go/Rust 边界的 bug 难以定位 |
| 社区规模小 | 中 | 38 stars, 4 贡献者，主要靠 Anytype 团队维护 |
| Docker 构建 | 中 | 需要安装 Rust 工具链或预编译库 |

### 性能

Tantivy 性能接近 Lucene，远超 Bleve。对于 100K 文档：
- 搜索延迟：< 5ms
- 索引吞吐：10K+ docs/sec
- 内存占用：低 (段式存储，mmap)

### 优缺点

| 优点 | 缺点 |
|------|------|
| 功能最完整 (BM25 + 字段权重 + 中文 + 前缀 + 模糊) | CGo 依赖，构建复杂 |
| Tantivy 引擎性能优异 | 社区小，主要靠 Anytype 维护 |
| 生产验证 (Anytype 1+ 年) | 调试困难 (跨语言) |
| 内置 Jieba 中文分词 | 二进制体积增加 ~10-15 MB |
| 支持全平台 (含移动端) | 版本升级可能遇到 Rust 兼容问题 |

---

## 补充方案: Bleve (github.com/blevesearch/bleve)

调研中发现 Bleve 是 Go 生态中最成熟的全文搜索库，值得纳入对比。

### 基本信息

| 项目 | 数据 |
|------|------|
| Stars | ~10,000+ |
| License | Apache 2.0 |
| 成熟度 | 生产级，Couchbase 等项目使用 |
| 最新版本 | 活跃维护 |

### 功能特性

| 特性 | 支持情况 |
|------|----------|
| BM25 评分 | 支持 (v1.x 新增，query-time boosting) |
| 字段权重 | 支持 (field boost at query time) |
| 中文分词 | 支持 (内置 CJK analyzer) |
| 前缀匹配 | 支持 (Prefix query) |
| 模糊匹配 | 支持 (Fuzzy query) |
| 持久化 | 支持 (Scorch 引擎，段式存储) |

### 优缺点

| 优点 | 缺点 |
|------|------|
| Go 生态最成熟的搜索库 | 性能不如 Tantivy |
| 纯 Go，无 CGo 依赖 | 索引体积较大 |
| 内置 30+ 语言分析器 (含 CJK) | API 相对复杂 |
| 10K+ stars，社区活跃 | BM25 支持较新 (2024+) |
| 支持 vector search (混合搜索) | 100K 文档可能偏重 |

---

## 综合对比

| 维度 | SQLite FTS5 | MySQL FULLTEXT | 自定义 Go | Blaze | Tantivy-Go | Bleve |
|------|-------------|----------------|-----------|-------|------------|-------|
| **字段权重 BM25** | 原生完美 | 需要 hack | 完全自定义 | 不支持 | 支持 | 支持 |
| **中文支持** | simple 扩展/预处理 | 原生 ngram | 集成 gojieba | 不支持 | 内置 Jieba | 内置 CJK |
| **前缀匹配** | 支持 (prefix index) | 支持 (BOOLEAN * ) | 自实现 | 不支持 | 支持 | 支持 |
| **模糊匹配** | 不支持 | 不支持 | 自实现 | 不支持 | 支持 | 支持 |
| **多租户** | 子查询/JOIN | WHERE 条件 | 内存过滤 | 后处理 | 查询时过滤 | 查询时过滤 |
| **性能 (100K)** | < 30ms | < 20ms | < 10ms | N/A | < 5ms | < 30ms |
| **额外依赖** | simple C 扩展 | MySQL | gojieba | 无 | Rust/CGo | 无 |
| **开发成本** | 低 (1-2 天) | 低 (1 天) | 高 (2 周) | N/A | 中 (3-5 天) | 低 (2-3 天) |
| **维护成本** | 低 | 低 | 高 | 不推荐 | 中 | 低 |
| **生产就绪** | 是 | 是 | 取决于实现 | 否 | 是 (Anytype) | 是 |

---

## 推荐方案

### 第一推荐: SQLite FTS5 + simple tokenizer

**理由：**
1. SQLite 已在技术栈中，零架构变动
2. BM25 字段权重是原生特性，`bm25(table, 3.0, 2.0, 1.0)` 一行搞定
3. 100K 文档对 SQLite FTS5 毫无压力，延迟 < 30ms
4. 数据一致性天然保障 (同库事务)
5. simple tokenizer 提供中文分词 + 拼音搜索，满足中英文需求
6. 前缀匹配通过 `prefix` 选项支持

**模糊匹配的补充方案：**
- 应用层实现 Levenshtein 距离对 FTS 结果二次过滤
- 或使用 SQLite 的 spellfix1 扩展做拼写建议

**实施路径：**
```
第 1 天: 建 FTS5 表 + BM25 查询 + 多租户过滤
第 2 天: 编译集成 simple tokenizer，验证中文搜索
第 3 天: 前缀索引调优 + 应用层模糊匹配
```

### 第二推荐 (如需更强搜索能力): Bleve

如果模糊匹配是硬需求且不想引入 CGo：

```go
// Bleve 纯 Go，支持 fuzzy + CJK + BM25 + 前缀
index, _ := bleve.Open("mcp_search.bleve")
query := bleve.NewDisjunctionQuery(
    bleve.NewFuzzyQuery("search_term"),     // 模糊匹配
    bleve.NewPrefixQuery("search_term"),     // 前缀匹配
)
query.AddBoost("service_name", 3.0)
query.AddBoost("tool_name", 2.0)
searchReq := bleve.NewSearchRequest(query)
searchReq.Size = 20
results, _ := index.Search(searchReq)
```

### 第三推荐 (如需极致性能): tantivy-go

如果团队接受 CGo 且需要 Tantivy 级别的搜索性能。

### 不推荐

- **Blaze**: 教育项目，缺少核心特性
- **自定义 Go 倒排索引**: 除非有特殊定制需求，ROI 不如 FTS5 或 Bleve
- **MySQL FULLTEXT**: 除非 MySQL 已是主数据库且有强一致性需求，否则不如 FTS5 灵活

---

## SQLite FTS5 实施参考

### Schema 设计

```sql
-- 主表
CREATE TABLE mcp_services (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id INTEGER NOT NULL,
    visibility TEXT NOT NULL DEFAULT 'private',  -- 'public' / 'private'
    service_name TEXT NOT NULL,
    description TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE mcp_tools (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    service_id INTEGER NOT NULL REFERENCES mcp_services(id),
    tool_name TEXT NOT NULL,
    description TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- FTS5 虚拟表 (content table 模式，避免数据冗余)
CREATE VIRTUAL TABLE mcp_search USING fts5(
    service_name,
    tool_name,
    description,
    content='mcp_search_content',   -- 内容表
    content_rowid='id',
    prefix='2 3 4',
    tokenize='simple'               -- 或 'unicode61 categories "L* N* Co"'
);

-- 内容表 (物化视图)
CREATE TABLE mcp_search_content (
    id INTEGER PRIMARY KEY,
    service_name TEXT,
    tool_name TEXT,
    description TEXT
);

-- 同步触发器
CREATE TRIGGER mcp_search_ai AFTER INSERT ON mcp_search_content BEGIN
    INSERT INTO mcp_search(rowid, service_name, tool_name, description)
    VALUES (new.id, new.service_name, new.tool_name, new.description);
END;

CREATE TRIGGER mcp_search_ad AFTER DELETE ON mcp_search_content BEGIN
    INSERT INTO mcp_search(mcp_search, rowid, service_name, tool_name, description)
    VALUES('delete', old.id, old.service_name, old.tool_name, old.description);
END;

CREATE TRIGGER mcp_search_au AFTER UPDATE ON mcp_search_content BEGIN
    INSERT INTO mcp_search(mcp_search, rowid, service_name, tool_name, description)
    VALUES('delete', old.id, old.service_name, old.tool_name, old.description);
    INSERT INTO mcp_search(rowid, service_name, tool_name, description)
    VALUES (new.id, new.service_name, new.tool_name, new.description);
END;
```

### 搜索查询 (Go 代码)

```go
func (s *SearchService) SearchTools(ctx context.Context, query string, userID int64, limit int) ([]*SearchResult, error) {
    // 构造 FTS5 查询
    ftsQuery := formatFTS5Query(query) // 转义特殊字符，添加前缀 *

    rows, err := s.db.QueryContext(ctx, `
        SELECT
            sc.id,
            sc.service_name,
            sc.tool_name,
            sc.description,
            bm25(mcp_search, 3.0, 2.0, 1.0) AS score
        FROM mcp_search ms
        JOIN mcp_search_content sc ON ms.rowid = sc.id
        JOIN mcp_tools mt ON sc.id = mt.id
        JOIN mcp_services svc ON mt.service_id = svc.id
        WHERE ms.mcp_search MATCH ?
          AND (svc.owner_id = ? OR svc.visibility = 'public')
        ORDER BY score
        LIMIT ?
    `, ftsQuery, userID, limit)
    // ...
}
```

### 索引大小预估

| 组件 | 预估大小 |
|------|----------|
| FTS5 索引 (100K 文档, 平均 100 tokens/文档) | ~15-30 MB |
| prefix 索引 (2/3/4) | 额外 ~10-20 MB |
| 总计 | ~25-50 MB |

完全可接受。
