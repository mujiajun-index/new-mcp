package transport

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// 背景:不少社区 stdio 服务会在 stdout 上打印横幅/日志(如 bazi-mcp 的
// "Bazi MCP server is running on stdio."),而 MCP stdio 传输约定 stdout 只能
// 出现 JSON-RPC 消息。官方 go-sdk 的 CommandTransport 对 stdout 做严格的
// JSON 流解析,遇到垃圾字节直接断连(invalid character 'B' ...)。
// StdioFilterTransport 自己拉起子进程,在 stdout 与 SDK 之间垫一层行过滤器:
// 只把「能构成完整 JSON 对象/数组」的内容交给 SDK,其余行丢弃并记入平台日志。
// 连接解析复用官方 mcp.IOTransport(JSON-RPC batch、关闭语义均保留),进程
// 生命周期(关 stdin 优雅退出 → 限时等待 → SIGTERM → SIGKILL → Wait 回收)
// 沿用 go-sdk CommandTransport 的语义。

const (
	// maxFilterBuffer 是跨行累积候选消息的上限,防止损坏流无限撑爆内存。
	// 只限制「不完整」的累积:单行完整 JSON 无论多大都原样放行。
	maxFilterBuffer = 32 << 20
	// maxDropLogs / maxStderrLogs 是逐行记日志的上限,超过后只丢弃/排空
	// 不再记录,避免日志刷屏。
	maxDropLogs   = 100
	maxStderrLogs = 200
	// maxLogRunes 是单条日志内容的截断长度(按 rune,不切碎中文)。
	maxLogRunes = 300
)

// StdioFilterTransport 是 mcp.Transport 实现:拉起子进程,stdout 经行过滤器
// 净化后交给 mcp.IOTransport。替代 mcp.CommandTransport。
type StdioFilterTransport struct {
	Command           *exec.Cmd
	TerminateDuration time.Duration // Close 时等待优雅退出的时长,<=0 用默认 5s
	logf              func(format string, args ...any)
}

// NewStdioFilterTransport 构造带 stdout 过滤的 stdio 传输。logf 为 nil 时用
// 标准库日志(logs/ 文件 + 控制台),带命令名前缀。
func NewStdioFilterTransport(cmd *exec.Cmd, logf func(format string, args ...any)) *StdioFilterTransport {
	if logf == nil {
		tag := "[stdio]"
		if cmd != nil && len(cmd.Args) > 0 {
			tag = "[stdio " + filepath.Base(cmd.Args[0]) + "]"
		}
		logf = func(format string, args ...any) {
			log.Printf("%s "+format, append([]any{tag}, args...)...)
		}
	}
	return &StdioFilterTransport{Command: cmd, logf: logf}
}

// stdioLogf 返回带服务标识的日志函数,供 NewStdioAdapter 注入,便于在平台
// 日志里把过滤/ stderr 输出关联到具体服务。
func stdioLogf(serviceID int64, command string) func(format string, args ...any) {
	tag := fmt.Sprintf("[stdio %s]", filepath.Base(command))
	if serviceID > 0 {
		tag = fmt.Sprintf("[stdio service %d %s]", serviceID, filepath.Base(command))
	}
	return func(format string, args ...any) {
		log.Printf("%s "+format, append([]any{tag}, args...)...)
	}
}

// Connect 启动子进程并建立过滤后的连接。stderr(若调用方未占用)会被持续
// 排空并记入日志——npx 下载报错、上游崩溃栈等都走 stderr,不记则无从排查。
func (t *StdioFilterTransport) Connect(ctx context.Context) (mcp.Connection, error) {
	stdout, err := t.Command.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stdin, err := t.Command.StdinPipe()
	if err != nil {
		return nil, err
	}
	if t.Command.Stderr == nil {
		if stderr, perr := t.Command.StderrPipe(); perr == nil {
			go drainStderr(stderr, t.logf)
		}
	}
	if err := t.Command.Start(); err != nil {
		return nil, err
	}
	// Reader 包 NopCloser:真正的 stdout 管道由 cmd.Wait() 回收;这里只负责
	// 内容过滤。进程终止序列在 stdioFilterConn.Close 里。
	filter := newJSONLineFilter(stdout, t.logf)
	inner, err := (&mcp.IOTransport{Reader: io.NopCloser(filter), Writer: stdin}).Connect(ctx)
	if err != nil {
		return nil, err
	}
	return &stdioFilterConn{Connection: inner, cmd: t.Command, stdin: stdin, terminate: t.TerminateDuration}, nil
}

// stdioFilterConn 包装 IOTransport 的连接,把 Close 升级为完整的进程终止
// 序列(照搬 go-sdk pipeRWC.Close):关 stdin → 限时等待 → SIGTERM →
// SIGKILL → Wait。Windows 上 SIGTERM 不支持,Signal 报错后直接走 Kill。
type stdioFilterConn struct {
	mcp.Connection
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	terminate time.Duration
	closeOnce sync.Once
	closeErr  error
}

func (c *stdioFilterConn) Close() error {
	c.closeOnce.Do(func() { c.closeErr = c.closeNow() })
	return c.closeErr
}

func (c *stdioFilterConn) closeNow() error {
	// 内层 Close 关闭 stdin(子进程据此优雅退出)并停止 SDK 的读取循环。
	if err := c.Connection.Close(); err != nil {
		return fmt.Errorf("closing connection: %w", err)
	}

	resChan := make(chan error, 1)
	go func() { resChan <- c.cmd.Wait() }()
	td := c.terminate
	if td <= 0 {
		td = 5 * time.Second
	}
	wait := func() (error, bool) {
		select {
		case err := <-resChan:
			return err, true
		case <-time.After(td):
			return nil, false
		}
	}
	if err, ok := wait(); ok {
		return err
	}
	if err := c.cmd.Process.Signal(syscall.SIGTERM); err == nil {
		if err, ok := wait(); ok {
			return err
		}
	}
	if err := c.cmd.Process.Kill(); err != nil {
		return err
	}
	if err, ok := wait(); ok {
		return err
	}
	return fmt.Errorf("unresponsive subprocess")
}

// --- stdout 行过滤器 ---

// jsonLineFilter 是垫在子进程 stdout 前的 io.Reader:逐行读取,只有能构成
// 完整 JSON 对象/数组(JSON-RPC 消息的合法形态)的内容才放行,横幅/日志等
// 垃圾行丢弃并记日志。输出仍是换行分隔的 JSON 字节流,交给 SDK 的解码器。
// 跨行的 pretty JSON 也能容忍:行并入候选缓冲,直到恰好构成完整 JSON 才放行。
type jsonLineFilter struct {
	r     *bufio.Reader
	rest  []byte // 待交付给上层的内容(已放行的完整消息)
	acc   []byte // 跨行累积中的候选消息
	err   error  // 上游的终结错误(EOF/管道关闭),缓存到 rest 交付完再报
	drops int
	logf  func(format string, args ...any)
}

func newJSONLineFilter(r io.Reader, logf func(format string, args ...any)) *jsonLineFilter {
	return &jsonLineFilter{r: bufio.NewReader(r), logf: logf}
}

func (f *jsonLineFilter) Read(p []byte) (int, error) {
	for len(f.rest) == 0 {
		if f.err != nil {
			return 0, f.err
		}
		line, rerr := f.r.ReadBytes('\n')
		if len(line) > 0 {
			f.feed(line)
		}
		if rerr != nil {
			f.err = rerr
			f.flush()
		}
	}
	n := copy(p, f.rest)
	f.rest = f.rest[n:]
	return n, nil
}

// feed 处理一行(可能带 '\n' 结尾)。无候选在累积时,不以 '{'/'[' 开头的行
// 直接判为垃圾丢弃;有候选时任何行都并入(容忍多行 pretty JSON 与 \r\n)。
// 并入后若恰好构成完整 JSON 值则整段放行。
func (f *jsonLineFilter) feed(line []byte) {
	if len(f.acc) == 0 {
		// 前导空白与 UTF-8 BOM 都不是 JSON 值的开头,一并剥掉。
		t := bytes.TrimLeft(line, " \t\r\n\uFEFF")
		if len(t) == 0 {
			return // 空行
		}
		if t[0] != '{' && t[0] != '[' {
			f.drop(t)
			return
		}
		f.acc = append(f.acc, t...)
	} else {
		// 原样并入:换行本身是合法的 JSON 空白,不额外插分隔符,放行时
		// 保留的是上游原始字节。
		f.acc = append(f.acc, line...)
	}
	if json.Valid(f.acc) {
		// 行尾的空白(含 \r\n)统一剥掉再补单个 '\n',保证交付给 SDK 的
		// 每条消息都是规范的换行分隔形态。
		f.rest = append(f.rest, bytes.TrimRight(f.acc, " \t\r\n")...)
		f.rest = append(f.rest, '\n')
		f.acc = f.acc[:0]
	} else if len(f.acc) > maxFilterBuffer {
		f.drop(f.acc)
		f.acc = f.acc[:0]
	}
}

// flush 在上游结束时冲刷仍在累积的候选:构成完整 JSON 则放行,否则丢弃。
func (f *jsonLineFilter) flush() {
	if len(f.acc) == 0 {
		return
	}
	if json.Valid(f.acc) {
		f.rest = append(f.rest, bytes.TrimRight(f.acc, " \t\r\n")...)
		f.rest = append(f.rest, '\n')
	} else {
		f.drop(f.acc)
	}
	f.acc = f.acc[:0]
}

func (f *jsonLineFilter) drop(b []byte) {
	if f.drops >= maxDropLogs {
		return
	}
	f.drops++
	f.logf("dropped non-JSON stdout: %s", truncateRunes(string(b), maxLogRunes))
	if f.drops == maxDropLogs {
		f.logf("further non-JSON stdout output suppressed (%d lines logged)", maxDropLogs)
	}
}

// --- stderr 排空与日志 ---

// drainStderr 持续排空子进程 stderr 并逐行记入日志。必须一直读到 EOF:
// 停读会让管道写满,子进程会卡在 stderr 写入上。超过行数上限后只排空不记录。
func drainStderr(r io.Reader, logf func(format string, args ...any)) {
	br := bufio.NewReader(r)
	logged := 0
	suppressed := false
	var buf []byte
	for {
		chunk, err := br.ReadSlice('\n')
		buf = append(buf, chunk...)
		if err == bufio.ErrBufferFull {
			// 超长行继续累积;超上限则整段丢弃,防内存膨胀。
			if len(buf) > 1<<20 {
				buf = buf[:0]
			}
			continue
		}
		if len(buf) > 0 {
			if logged < maxStderrLogs {
				logf("stderr: %s", truncateRunes(string(buf), maxLogRunes))
				logged++
			} else if !suppressed {
				logf("stderr: exceeded %d lines, further output suppressed", maxStderrLogs)
				suppressed = true
			}
		}
		buf = buf[:0]
		if err != nil { // io.EOF 或管道关闭
			return
		}
	}
}

// truncateRunes 按 rune 截断,避免把中文/emoji 从中间切碎。
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "...(truncated)"
}
