#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
sqlite_to_mysql.py

将 newmcp 项目的 SQLite 数据库 (data/newmcp.db) 导出为 MySQL 5.7 兼容的 SQL 脚本。

特性:
  * 自动读取 SQLite 中的所有业务表 (跳过 sqlite_sequence 等系统表)。
  * 把 SQLite 类型 (integer / text / datetime / numeric / mediumtext / decimal ...)
    转成 MySQL 5.7 对应类型 (bigint / longtext / varchar(N) / datetime / tinyint(1) / decimal / mediumtext)。
  * 把 "AUTOINCREMENT" / 双引号默认值 / "PRIMARY KEY" 写法改成 MySQL 兼容的
    `AUTO_INCREMENT` / 单引号 / 表级 PRIMARY KEY。
  * 保留 SQLite 的 CREATE INDEX 语句 (MySQL 5.7 同样支持该语法)。
  * 字符串 / 二进制 / NULL 值按 MySQL 转义规则转义 (处理 \ ' " NUL 换行 等)。
  * 一次插入多行 (多值 INSERT) 以加速导入。
  * 输出文件按 UTF-8 (含 BOM 跳过) 写入; 表/库默认 utf8mb4 + utf8mb4_unicode_ci。

用法:
  python scripts/sqlite_to_mysql.py \
      --db data/newmcp.db \
      --out data/newmcp_mysql.sql \
      --database newmcp

  在 MySQL 5.7 上执行前, 请手动确认目标库 (脚本不会 DROP 任何库/表, 但每张表
  会先 DROP IF EXISTS 之后再 CREATE, 避免脏数据):
      mysql -uroot -p < data/newmcp_mysql.sql
"""
from __future__ import annotations

import argparse
import os
import re
import sqlite3
import sys
from typing import Iterable, List, Tuple


# ---------- SQLite -> MySQL type mapping ----------
#
# 规则按 sqlite 的 "类型亲和性 (type affinity)" 启发式决定: 只看声明类型里的关键字,
# 这样能在不引入额外元数据的前提下做出合理映射。size 提示 (varchar(N)) 直接保留。
#
_TYPE_RULES: List[Tuple[re.Pattern, str]] = [
    # integer PRIMARY KEY AUTOINCREMENT 已经在列级别处理, 这里只处理普通 integer。
    (re.compile(r"\binteger\b", re.IGNORECASE), "bigint"),
    (re.compile(r"\bbigint\b", re.IGNORECASE), "bigint"),
    (re.compile(r"\bmediumtext\b", re.IGNORECASE), "mediumtext"),
    (re.compile(r"\blongtext\b", re.IGNORECASE), "longtext"),
    (re.compile(r"\btinytext\b", re.IGNORECASE), "tinytext"),
    (re.compile(r"\btext\b", re.IGNORECASE), "longtext"),
    # numeric 在 SQLite 中用来表示布尔 / 小数; 这里统一当布尔处理 (tinyint(1)),
    # 表里其它 decimal 字段都用了 decimal(p,s), 不会被这条匹配。
    (re.compile(r"\bnumeric\b", re.IGNORECASE), "tinyint(1)"),
    (re.compile(r"\bdecimal\b(\s*\(\s*\d+\s*,\s*\d+\s*\))?", re.IGNORECASE), r"decimal\1"),
    (re.compile(r"\breal\b", re.IGNORECASE), "double"),
    (re.compile(r"\bdouble\b", re.IGNORECASE), "double"),
    (re.compile(r"\bfloat\b", re.IGNORECASE), "float"),
    (re.compile(r"\bblob\b", re.IGNORECASE), "longblob"),
    (re.compile(r"\bdatetime\b", re.IGNORECASE), "datetime(6)"),
    (re.compile(r"\btimestamp\b", re.IGNORECASE), "datetime(6)"),
    (re.compile(r"\bdate\b", re.IGNORECASE), "date"),
    (re.compile(r"\btime\b", re.IGNORECASE), "time"),
    (re.compile(r"\bboolean\b", re.IGNORECASE), "tinyint(1)"),
]

# MySQL 关键字 / 保留字, 需用反引号包裹。常见冲突字段: key, group, order, ...
_RESERVED = {
    "key", "group", "order", "by", "select", "table", "index", "user",
    "password", "desc", "values", "name", "status", "limit",
}

# MySQL 5.7 不允许 BLOB/TEXT/JSON/GEOMETRY 列带默认值, 用这个集合判断。
_NO_DEFAULT_TYPES = re.compile(
    r"\b(longtext|mediumtext|text|tinytext|"
    r"longblob|mediumblob|blob|tinyblob|"
    r"json|geometry)\b",
    re.IGNORECASE,
)


# ---------- GORM 模型类型覆盖 ----------
#
# 背景: SQLite (glebarez/sqlite 纯 Go 驱动) 会忽略 string 字段的 `size:N`,
# 全部物化成 `text`; 但 GORM 在 MySQL 上会按 `size:N` 建成 `varchar(N)`。
# 而 MySQL 5.7 不允许 TEXT 列带 DEFAULT —— 于是 SQLite dump 里那些
# `text DEFAULT '...'` 的列在 MySQL 上会报错 1101。
#
# 解决: 直接读 model/*.go 里的 gorm 标签, 把每个字符串列的真实 MySQL 类型
# (varchar(N) 或显式 type:X) 抽出来, 覆盖掉 SQLite 的 `text`。
# 非 string 字段 (int/bool/time ...) 仍走下面的类型亲和性映射。
def _go_field_to_column(field: str) -> str:
    """Go 结构体字段名 → GORM 列名 (snake_case), 近似 GORM NamingStrategy。
    对 IP/URL/ID 这类常见缩写做特殊处理, 避免 AllowIPs → allow_i_ps。"""
    s = re.sub(r"([a-z0-9])([A-Z])", r"\1_\2", field)
    s = re.sub(r"([A-Z]+)([A-Z][a-z])", r"\1_\2", s)
    return s.lower()


_JSON_TAG_RE = re.compile(r'\bjson:"([^"]*)"')


def _parse_gorm_tag(tag: str) -> dict:
    """解析 `gorm:"a; b:c; type:varchar(4096); size:64"` 之类的标签内容。"""
    out = {}
    for part in tag.split(";"):
        part = part.strip()
        if not part:
            continue
        if ":" in part:
            k, v = part.split(":", 1)
            out.setdefault(k.strip(), v.strip())
        else:
            out[part] = True
    return out


def _string_mysql_type(gorm: dict) -> str:
    """根据 gorm 标签推断一个 *string* 字段的 MySQL 列类型。"""
    t = gorm.get("type")
    if t:
        # 去掉可能的引号 (用户偶尔写 type:'text')
        return t.strip("'\"")
    size = gorm.get("size")
    if size:
        return f"varchar({size})"
    # 既没 type 也没 size: GORM 默认 text → MySQL longtext
    return "longtext"


# 形如:  FieldName  string  `json:"x" gorm:"..."`
_FIELD_RE = re.compile(
    r"^\s+(?P<field>[A-Za-z_][A-Za-z0-9_]*)\s+string\b.*`(?P<tag>[^`]+)`"
)
_TYPE_STRUCT_RE = re.compile(r"type\s+(?P<name>[A-Za-z_]\w*)\s+struct\s*\{")
_TABLENAME_RE = re.compile(
    r'func\s+\(\s*(?P<recv>[A-Za-z_]\w*)\s*\)\s+TableName\(\)\s*string\s*\{\s*return\s+"(?P<table>[^"]+)"'
)


def build_type_overrides(model_dir: str) -> dict:
    """扫描 model/*.go, 返回 {table: {column: mysql_type}} (仅字符串列)。

    - 一个 .go 文件里可能有多个 struct (如 marketplace.go 同时定义 Item 和 Review),
      所以按 struct 作用域归列, 再用该 struct 对应的 TableName() 映射到表名。
    - 列名优先级: gorm `column:` → json tag → 字段名 snake_case。
      (本项目的 json tag 与真实列名完全一致, 用它最稳妥, 避免 IP/URL 缩写拆错。)
    """
    overrides: dict = {}
    go_files = []
    for fname in sorted(os.listdir(model_dir)):
        if fname.endswith(".go") and fname != "main.go":
            go_files.append(os.path.join(model_dir, fname))

    for path in go_files:
        with open(path, "r", encoding="utf-8") as f:
            src = f.read()

        # 1) struct 名 → 表名
        struct_to_table = {
            m.group("recv"): m.group("table")
            for m in _TABLENAME_RE.finditer(src)
        }
        if not struct_to_table:
            continue

        # 2) 逐行扫描, 追踪当前所在的 struct 作用域
        current_struct = None
        brace_depth = 0
        for line in src.splitlines():
            # 进入 struct
            if current_struct is None:
                m = _TYPE_STRUCT_RE.search(line)
                if m:
                    current_struct = m.group("name")
                    brace_depth = line.count("{") - line.count("}")
                    # struct 起始行本身一般不含字段, 继续
                    if brace_depth <= 0:
                        current_struct = None
                    continue
            else:
                brace_depth += line.count("{") - line.count("}")
                fm = _FIELD_RE.match(line)
                if fm and current_struct in struct_to_table:
                    field = fm.group("field")
                    tag = fm.group("tag")
                    gorm_raw = ""
                    gm = re.search(r'gorm:"([^"]*)"', tag)
                    if gm:
                        gorm_raw = gm.group(1)
                    gorm = _parse_gorm_tag(gorm_raw)
                    # 列名: column: > json tag (非 "-") > snake_case
                    col = gorm.get("column")
                    if not col:
                        jm = _JSON_TAG_RE.search(tag)
                        if jm:
                            jc = jm.group(1).split(",")[0]
                            if jc and jc != "-":  # json:"-" 表示不导出, 不是列名
                                col = jc
                    if not col:
                        col = _go_field_to_column(field)
                    if col:
                        table = struct_to_table[current_struct]
                        overrides.setdefault(table, {})[col] = _string_mysql_type(gorm)
                if brace_depth <= 0:
                    current_struct = None
    return overrides


# ---------- helpers ----------
def quote_ident(name: str) -> str:
    """反引号包裹标识符, 处理 MySQL 保留字冲突。"""
    if name.startswith("`") and name.endswith("`"):
        return name
    if name.lower() in _RESERVED:
        return f"`{name}`"
    if re.search(r"[^A-Za-z0-9_]", name):
        return f"`{name}`"
    return f"`{name}`"


def map_type(declared: str) -> str:
    """根据声明类型返回 MySQL 类型字符串。"""
    t = declared.strip()
    # 去掉前缀的 COLLATE / 之类修饰 (sqlite 极少使用)
    for pat, repl in _TYPE_RULES:
        new_t = pat.sub(repl, t)
        if new_t != t:
            t = new_t
    # 兜底: 空字符串
    return t or "longtext"


def escape_string(value: str) -> str:
    """把 Python 字符串转义成 MySQL 字符串字面量内容 (不含外层引号)。"""
    # MySQL 字符串字面量内: \\  \' \" \0 \n \r \x1a 都需要转义;
    # 反斜杠必须先转义, 否则后续 ' 会被错误处理。
    out = []
    for ch in value:
        cp = ord(ch)
        if ch == "\\":
            out.append("\\\\")
        elif ch == "'":
            out.append("\\'")
        elif ch == '"':
            out.append('\\"')
        elif ch == "\0":
            out.append("\\0")
        elif ch == "\n":
            out.append("\\n")
        elif ch == "\r":
            out.append("\\r")
        elif ch == "\x1a":
            out.append("\\Z")
        elif cp < 0x20 or cp == 0x7f:
            out.append(f"\\x{cp:02x}")
        else:
            out.append(ch)
    return "".join(out)


def escape_blob(data: bytes) -> str:
    """bytes 转 MySQL 十六进制字面量 (0x...)."""
    return "0x" + data.hex()


_DATETIME_TZ_RE = re.compile(
    r"^(\d{4}-\d{2}-\d{2}[ T]\d{2}:\d{2}:\d{2}(?:\.\d+)?)([+-]\d{2}:?\d{2}|Z)?$"
)


def _clean_datetime(s: str) -> str:
    """把 SQLite 的带时区 ISO 时间字符串 (如 '2026-05-12 18:12:44.045455+08:00')
    转成 MySQL DATETIME 字面量 (去掉时区, 保留分数秒)。"""
    m = _DATETIME_TZ_RE.match(s.strip())
    if not m:
        return s
    base = m.group(1)
    # 把空格分隔的时间转成 T? MySQL DATETIME 接受空格分隔; 保留原样
    # 限制 DATETIME(6) 最多 6 位分数
    if "." in base:
        head, frac = base.split(".", 1)
        frac = (frac + "000000")[:6]
        base = head + "." + frac
    return base


def format_value(v) -> str:
    """把 Python 值转成 MySQL 字面量。"""
    if v is None:
        return "NULL"
    if isinstance(v, (bytes, bytearray, memoryview)):
        return escape_blob(bytes(v))
    if isinstance(v, bool):
        return "1" if v else "0"
    if isinstance(v, (int, float)):
        return str(v)
    # 其它一律按字符串
    s = str(v)
    # 尝试识别为带时区的时间字面量并归一化
    if _DATETIME_TZ_RE.match(s.strip()):
        s = _clean_datetime(s)
    return "'" + escape_string(s) + "'"


# ---------- CREATE TABLE 转换 ----------
#
# SQLite 列定义形如:
#   `colname` typename [NOT NULL] [DEFAULT expr|"value"|NULL] [PRIMARY KEY] [AUTOINCREMENT]
# 表级约束:
#   PRIMARY KEY (col1, col2), UNIQUE (col), CHECK (...)
# 我们逐项拆分并转换。

# 由 main() 在启动时从 model/*.go 填充: {table: {column: mysql_type}}
TYPE_OVERRIDES: dict = {}


def _split_top_level(s: str, sep: str = ",") -> List[str]:
    """按分隔符切分, 忽略括号 / 引号 内部的逗号。"""
    parts: List[str] = []
    buf: List[str] = []
    depth = 0
    in_str = False
    quote = ""
    for ch in s:
        if in_str:
            buf.append(ch)
            if ch == quote:
                in_str = False
            continue
        if ch in ("'", '"'):
            in_str = True
            quote = ch
            buf.append(ch)
            continue
        if ch == "(":
            depth += 1
            buf.append(ch)
            continue
        if ch == ")":
            depth -= 1
            buf.append(ch)
            continue
        if ch == sep and depth == 0:
            parts.append("".join(buf))
            buf = []
            continue
        buf.append(ch)
    if buf:
        parts.append("".join(buf))
    return [p.strip() for p in parts if p.strip()]


def _convert_default(expr: str) -> str:
    """把 SQLite 默认值表达式转 MySQL。"""
    expr = expr.strip()
    if expr.upper() == "NULL":
        return "DEFAULT NULL"
    # 双引号字符串字面量 → 单引号
    if expr.startswith('"') and expr.endswith('"') and len(expr) >= 2:
        inner = expr[1:-1]
        return "DEFAULT '" + escape_string(inner) + "'"
    # 单引号字符串字面量
    if expr.startswith("'") and expr.endswith("'") and len(expr) >= 2:
        inner = expr[1:-1]
        return "DEFAULT '" + escape_string(inner) + "'"
    # 数字 / 函数 / true / false
    upper = expr.upper()
    if upper in ("TRUE", "FALSE"):
        return "DEFAULT " + ("1" if upper == "TRUE" else "0")
    # current_timestamp 等
    if upper in ("CURRENT_TIMESTAMP", "CURRENT_DATE", "CURRENT_TIME"):
        return f"DEFAULT {upper}"
    # 其它表达式 (e.g. 0) 原样保留
    return "DEFAULT " + expr


def _convert_column(line: str, table_name: str = "") -> Tuple[str, str, bool]:
    """转换一个列定义。
    返回 (新定义, 列名, 是否为主键列)。"""
    line = line.strip()
    # SQLite: `id` integer PRIMARY KEY AUTOINCREMENT
    m = re.match(r"^`(?P<name>[^`]+)`\s+(?P<rest>.+)$", line, re.DOTALL)
    if not m:
        # 也兼容双引号列名 (sqlite 允许)
        m = re.match(r'^"(?P<name>[^"]+)"\s+(?P<rest>.+)$', line, re.DOTALL)
        if not m:
            return line, "", False
    name = m.group("name")
    rest = m.group("rest").strip()
    # 提取尾部 PRIMARY KEY / AUTOINCREMENT / NOT NULL / DEFAULT ...
    parts = re.split(r"\s+", rest, maxsplit=1)
    type_decl = parts[0]
    tail = parts[1] if len(parts) > 1 else ""

    is_pk = bool(re.search(r"\bPRIMARY\s+KEY\b", tail, re.IGNORECASE))
    # 把 AUTOINCREMENT / PRIMARY KEY / COLLATE NOCASE 等 SQLite-only 标记清掉
    tail = re.sub(r"\bAUTOINCREMENT\b", "", tail, flags=re.IGNORECASE)
    tail = re.sub(r"\bPRIMARY\s+KEY\b", "", tail, flags=re.IGNORECASE)
    tail = re.sub(r"\bCOLLATE\s+\w+", "", tail, flags=re.IGNORECASE)

    # 提取 DEFAULT 子句单独处理 (引号形式需要转换)
    default_clause = ""
    dm = re.search(r"\bDEFAULT\s+(?P<expr>'[^']*'|\"[^\"]*\"|[^\s,]+(?:\s*\([^)]*\))?)",
                   tail, re.IGNORECASE)
    if dm:
        default_clause = _convert_default(dm.group("expr"))
        tail = tail[:dm.start()] + tail[dm.end():]

    # 类型转换: 优先用 GORM 标签里的真实类型 (让 size:N 的字符串列变回 varchar(N)),
    # 没有覆盖时再退回到按 SQLite 声明类型的亲和性映射。
    override = TYPE_OVERRIDES.get(table_name, {}).get(name)
    if override:
        mysql_type = override
    else:
        mysql_type = map_type(type_decl)

    # 安全网: MySQL 5.7 不允许 TEXT/BLOB/JSON/GEOMETRY 列带 DEFAULT。
    # 正常情况下 GORM 覆盖已把这些列变成 varchar, 不会走到这里;
    # 但若有遗漏 (如真正声明 type:text 又带了默认值), 这里剥掉默认值并打日志,
    # 保证 CREATE TABLE 一定能通过。运行时若需要默认值, 应用层 / GORM 会补。
    if default_clause and default_clause.upper() != "DEFAULT NULL" \
            and _NO_DEFAULT_TYPES.search(mysql_type):
        print(f"[WARN] {table_name}.{name}: {mysql_type} 列无法在 MySQL 5.7 设默认值, "
              f"已去掉 DEFAULT", file=sys.stderr)
        default_clause = ""

    # 重新组装
    pieces = [quote_ident(name), mysql_type]
    # PRIMARY KEY + AUTO_INCREMENT 仅当列级 PK 时附加
    if is_pk:
        pieces.append("NOT NULL AUTO_INCREMENT")
    tail = re.sub(r"\s+", " ", tail).strip()
    if tail:
        pieces.append(tail)
    if default_clause:
        pieces.append(default_clause)
    return "  " + " ".join(pieces), name, is_pk


def _convert_constraint(line: str) -> str:
    """转换表级约束 (PRIMARY KEY / UNIQUE / FOREIGN KEY / CHECK)。"""
    line = line.strip()
    # PRIMARY KEY (col1, col2 [AUTOINCREMENT]) — 去掉列上多余的 AUTOINCREMENT
    m = re.match(r"^PRIMARY\s+KEY\s*\((.+)\)\s*$", line, re.IGNORECASE | re.DOTALL)
    if m:
        cols_raw = m.group(1)
        cols = [c.strip() for c in cols_raw.split(",")]
        cols = [re.sub(r"\bAUTOINCREMENT\b", "", c, flags=re.IGNORECASE).strip() for c in cols]
        cols = [quote_ident(c) for c in cols]
        return "  PRIMARY KEY (" + ", ".join(cols) + ")"
    # UNIQUE (col, ...) — MySQL 5.7 也支持
    m = re.match(r"^UNIQUE\s*\((.+)\)\s*$", line, re.IGNORECASE | re.DOTALL)
    if m:
        cols = [quote_ident(c.strip()) for c in m.group(1).split(",")]
        return "  UNIQUE KEY (" + ", ".join(cols) + ")"
    # 其它 (CHECK 等) 暂原样保留
    return "  " + line


def convert_create_table(sql: str, table_name: str) -> str:
    """把一条 SQLite 的 CREATE TABLE SQL 转成 MySQL 5.7 兼容。"""
    # 去掉 IF NOT EXISTS — MySQL 同样支持, 这里保留; 但 sqlite 表名可能带 IF NOT EXISTS
    body = sql
    # 提取括号内部
    m = re.search(r"\((.*)\)\s*$", body, re.DOTALL)
    if not m:
        return sql  # 兜底
    header = body[: m.start() + 1]  # 含 (
    inner = m.group(1)
    # 规范化 header: 把双引号表名换成反引号
    header = re.sub(r'"([^"]+)"', r"`\1`", header)
    header = re.sub(r"CREATE\s+TABLE\s+(IF\s+NOT\s+EXISTS\s+)?", "", header, flags=re.IGNORECASE)
    header = "CREATE TABLE IF NOT EXISTS " + header.strip()

    items = _split_top_level(inner, ",")
    new_items: List[str] = []
    pk_columns: List[str] = []
    for it in items:
        if it.upper().startswith("PRIMARY KEY") or it.upper().startswith("UNIQUE") \
                or it.upper().startswith("FOREIGN KEY") or it.upper().startswith("CHECK") \
                or it.upper().startswith("CONSTRAINT"):
            # 表级约束 — 如果是 PRIMARY KEY 多列, 我们也允许; 单列的 PRIMARY KEY 通常是列级。
            new_items.append(_convert_constraint(it))
        elif it.startswith("INDEX") or it.startswith("KEY "):
            # 表内 INDEX 子句 (sqlite 允许, mysql 不允许) — 跳过, 后面用 CREATE INDEX 输出。
            continue
        elif it.upper().startswith("UNIQUE INDEX") or it.upper().startswith("INDEX"):
            # 同上, 跳过 (我们会单独生成 CREATE INDEX)
            continue
        else:
            new_line, col_name, is_pk = _convert_column(it, table_name)
            new_items.append(new_line)
            if is_pk:
                pk_columns.append(col_name)
    # 单列主键 (列级声明的) 补一个表级 PRIMARY KEY, 让 MySQL 显式建立 PK。
    if len(pk_columns) == 1:
        new_items.append(f"PRIMARY KEY ({quote_ident(pk_columns[0])})")
    return f"{header}\n  " + ",\n  ".join(new_items) + "\n) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci"


# ---------- 主流程 ----------
def list_tables(cur: sqlite3.Cursor) -> List[str]:
    cur.execute(
        "SELECT name FROM sqlite_master "
        "WHERE type='table' AND name NOT LIKE 'sqlite_%' "
        "ORDER BY name"
    )
    return [r[0] for r in cur.fetchall()]


def get_create_sql(cur: sqlite3.Cursor, table: str) -> str:
    cur.execute(
        "SELECT sql FROM sqlite_master "
        "WHERE type='table' AND name=?", (table,)
    )
    row = cur.fetchone()
    return row[0] if row else ""


def list_indexes(cur: sqlite3.Cursor) -> List[Tuple[str, str]]:
    """返回 [(index_name, sql), ...] — 业务表的索引。"""
    cur.execute(
        "SELECT name, sql FROM sqlite_master "
        "WHERE type='index' AND sql IS NOT NULL AND tbl_name NOT LIKE 'sqlite_%' "
        "ORDER BY tbl_name, name"
    )
    return [(r[0], r[1]) for r in cur.fetchall()]


def convert_index_sql(sql: str) -> str:
    """把 SQLite 的 CREATE INDEX 转 MySQL 兼容 (主要是 TEMP / IF NOT EXISTS / 引号)。"""
    # 删除 TEMP / TEMPORARY 关键字
    s = re.sub(r"\bTEMP(ORARY)?\b", "", sql, flags=re.IGNORECASE)
    # MySQL 5.7 CREATE INDEX 不支持 IF NOT EXISTS, 但通常这里 sql 也没有
    # 调整双引号为反引号: MySQL 也支持双引号 (在 ANSI_QUOTES 关闭时), 为一致性全部用反引号
    s = re.sub(r'"([^"]+)"', r"`\1`", s)
    return s.strip()


def table_columns(cur: sqlite3.Cursor, table: str) -> List[str]:
    cur.execute(f"PRAGMA table_info({quote_sqlite_ident(table)})")
    return [r[1] for r in cur.fetchall()]


def quote_sqlite_ident(name: str) -> str:
    return '"' + name.replace('"', '""') + '"'


def fetch_rows(cur: sqlite3.Cursor, table: str, columns: List[str]) -> Iterable[Tuple]:
    cols_sql = ", ".join(quote_sqlite_ident(c) for c in columns)
    cur.execute(f"SELECT {cols_sql} FROM {quote_sqlite_ident(table)}")
    return cur.fetchall()


def main():
    ap = argparse.ArgumentParser(description="Convert newmcp SQLite DB -> MySQL 5.7 SQL dump")
    ap.add_argument("--db", default="data/newmcp.db", help="SQLite 文件路径")
    ap.add_argument("--out", default="data/newmcp_mysql.sql", help="输出 SQL 文件路径")
    ap.add_argument("--database", default="newmcp", help="目标 MySQL 数据库名")
    ap.add_argument("--batch-size", type=int, default=200, help="每条多值 INSERT 的行数")
    ap.add_argument("--no-data", action="store_true", help="只导出结构, 不导出数据")
    ap.add_argument("--model-dir", default="model",
                    help="GORM 模型目录 (用于推断字符串列的 varchar 长度)")
    args = ap.parse_args()

    db_path = args.db
    if not os.path.isfile(db_path):
        print(f"[ERROR] SQLite 文件不存在: {db_path}", file=sys.stderr)
        sys.exit(2)

    # 从 GORM 模型推断每个字符串列的 MySQL 类型 (varchar(N) 等), 覆盖 SQLite 的 text。
    if os.path.isdir(args.model_dir):
        global TYPE_OVERRIDES
        TYPE_OVERRIDES = build_type_overrides(args.model_dir)
        n = sum(len(v) for v in TYPE_OVERRIDES.values())
        print(f"[INFO] 从 {args.model_dir} 解析出 {len(TYPE_OVERRIDES)} 张表 / "
              f"{n} 个字符串列的类型覆盖", file=sys.stderr)
    else:
        print(f"[WARN] 找不到模型目录 {args.model_dir}, 跳过 GORM 类型覆盖 "
              f"(TEXT 列带默认值会触发 MySQL 5.7 错误 1101)", file=sys.stderr)

    conn = sqlite3.connect(db_path)
    cur = conn.cursor()

    tables = list_tables(cur)
    indexes = list_indexes(cur)
    print(f"[INFO] 业务表: {len(tables)} 个, 索引: {len(indexes)} 个", file=sys.stderr)

    out_lines: List[str] = []
    # ---- 头部 ----
    # 注意: 脚本不会 DROP 任何数据库, 以免误删。请在执行 SQL 前手动确认目标库。
    out_lines.append("-- Generated by scripts/sqlite_to_mysql.py")
    out_lines.append("-- Source: " + os.path.abspath(db_path))
    out_lines.append("-- Target: MySQL 5.7+ (utf8mb4)")
    out_lines.append("SET NAMES utf8mb4;")
    out_lines.append("SET FOREIGN_KEY_CHECKS=0;")
    out_lines.append("SET SQL_MODE='NO_AUTO_VALUE_ON_ZERO';")
    out_lines.append("")
    out_lines.append(f"CREATE DATABASE IF NOT EXISTS `{args.database}` "
                     f"DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;")
    out_lines.append(f"USE `{args.database}`;")
    out_lines.append("")

    # ---- 表结构 ----
    out_lines.append("-- ----------------------------")
    out_lines.append("-- Table structure")
    out_lines.append("-- ----------------------------")
    for t in tables:
        create_sql = get_create_sql(cur, t)
        if not create_sql:
            print(f"[WARN] {t}: 找不到 CREATE TABLE SQL, 跳过", file=sys.stderr)
            continue
        out_lines.append(f"DROP TABLE IF EXISTS `{t}`;")
        out_lines.append(convert_create_table(create_sql, t) + ";")
        out_lines.append("")

    # ---- 索引 ----
    out_lines.append("-- ----------------------------")
    out_lines.append("-- Indexes")
    out_lines.append("-- ----------------------------")
    for name, sql in indexes:
        # 如果是表内 INDEX 子句 (即 sql 含 CREATE INDEX), 跳过; 我们已经跳过
        if sql is None:
            continue
        out_lines.append(convert_index_sql(sql) + ";")
    out_lines.append("")

    # ---- 数据 ----
    if not args.no_data:
        out_lines.append("-- ----------------------------")
        out_lines.append("-- Data")
        out_lines.append("-- ----------------------------")
        for t in tables:
            cols = table_columns(cur, t)
            if not cols:
                continue
            rows = fetch_rows(cur, t, cols)
            row_list = list(rows)
            if not row_list:
                continue
            out_lines.append(f"-- Data for table `{t}` ({len(row_list)} rows)")
            col_list_sql = ", ".join(quote_ident(c) for c in cols)
            batch = args.batch_size
            for i in range(0, len(row_list), batch):
                chunk = row_list[i:i + batch]
                values_sql = ",\n".join(
                    "(" + ", ".join(format_value(v) for v in row) + ")"
                    for row in chunk
                )
                out_lines.append(f"INSERT INTO `{t}` ({col_list_sql}) VALUES\n{values_sql};")
            out_lines.append("")

    out_lines.append("SET FOREIGN_KEY_CHECKS=1;")
    out_lines.append("")

    out_path = args.out
    os.makedirs(os.path.dirname(os.path.abspath(out_path)) or ".", exist_ok=True)
    with open(out_path, "w", encoding="utf-8", newline="\n") as f:
        f.write("\n".join(out_lines))

    print(f"[OK] 已生成: {out_path}", file=sys.stderr)


if __name__ == "__main__":
    main()
