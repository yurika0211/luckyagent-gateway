package presentation

import (
	"encoding/json"
	"fmt"
	tool "github.com/yurika0211/luckyagent-tools"
	"github.com/yurika0211/luckyagent-tools/memory"
	"html"
	"strings"
)

type TelegramToolTraceStep struct {
	Name    string
	Args    string
	Result  string
	Success bool
}

func HumanizeThinkingProgress(content string) string {
	content = ClipOneLine(content, 180)
	if content == "" {
		return ""
	}
	if IsInternalThinkingProgress(content) {
		return ""
	}
	return EnsureSentenceSuffix("先整理一下当前思路：" + content)
}

func RenderTelegramThinkingCard(content string) string {
	content = CompactProgressCardText(content)
	if content == "" || IsInternalThinkingProgress(content) {
		return ""
	}
	return "<b>💭 Reasoning Trace</b>\n<blockquote expandable>" + html.EscapeString(content) + "</blockquote>"
}

func RenderTelegramSummaryCard(summary string) string {
	summary = CompactProgressCardText(summary)
	if summary == "" {
		return ""
	}
	return "<b>💭 Reasoning Trace</b>\n<blockquote expandable>" + html.EscapeString(summary) + "</blockquote>"
}

func RenderTelegramProgressHistoryCard(parts []string) string {
	body := make([]string, 0, len(parts))
	for _, part := range parts {
		part = CompactProgressCardText(part)
		if part == "" {
			continue
		}
		body = append(body, html.EscapeString(part))
	}
	if len(body) == 0 {
		return ""
	}
	return "<b>💭 Reasoning Trace</b>\n<blockquote expandable>" + strings.Join(body, "\n") + "</blockquote>"
}

func CompactProgressCardText(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	lines := strings.Split(content, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

func RenderTelegramToolTraceCard(steps []TelegramToolTraceStep) string {
	return RenderTelegramToolTraceCardWithDetails(steps, false)
}

func RenderTelegramToolTraceCardWithDetails(steps []TelegramToolTraceStep, detailed bool) string {
	return RenderTelegramToolTraceCardWithTemplateDetails(steps, detailed, nil)
}

func RenderTelegramToolTraceCardWithTemplateDetails(steps []TelegramToolTraceStep, detailed bool, templates map[string]string) string {
	segments := make([]string, 0, len(steps))
	shown := 0
	for _, step := range steps {
		visibility := TelegramToolTraceVisibility(step.Name)
		if visibility == "hidden" || visibility == "agent" {
			continue
		}
		line := FormatTelegramToolTraceLineWithTemplates(shown+1, step, visibility, detailed, templates)
		if strings.TrimSpace(line) == "" {
			continue
		}
		segments = append(segments, line)
		shown++
		if shown >= 6 {
			break
		}
	}
	if shown == 0 {
		return ""
	}

	var body strings.Builder
	body.WriteString("<b>🛠 Tool Trace</b> · ")
	body.WriteString(fmt.Sprintf("%d tools", shown))
	body.WriteString("\n\n")
	body.WriteString(strings.Join(segments, "\n\n"))
	if detailed {
		body.WriteString("\n\n<i>展开的参数与结果仅用于本次追踪。</i>")
	}
	return body.String()
}

func RenderTelegramAgentTraceCard(steps []TelegramToolTraceStep) string {
	segments := make([]string, 0, len(steps))
	shown := 0
	total := 0
	completed, failed := 0, 0
	for _, step := range steps {
		if TelegramToolTraceVisibility(step.Name) != "agent" {
			continue
		}
		total++
		if step.Success {
			completed++
		} else {
			failed++
		}
	}
	for _, step := range steps {
		if TelegramToolTraceVisibility(step.Name) != "agent" {
			continue
		}
		line := FormatTelegramAgentTraceLine(shown+1, step)
		if strings.TrimSpace(line) == "" {
			continue
		}
		segments = append(segments, line)
		shown++
		if shown >= 6 {
			break
		}
	}
	if shown == 0 {
		return ""
	}
	body := fmt.Sprintf("Task Overview · total: %d · completed: %d · failed: %d\n\n%s", total, completed, failed, strings.Join(segments, "\n"))
	body += fmt.Sprintf("\nDone · %d agent steps · %d succeeded, %d failed", total, completed, failed)
	return "<b>🧭 Agent Trace</b>\n<pre><code>" + html.EscapeString(body) + "</code></pre>"
}

func ParseTelegramMemoryTracePayload(payload string) (memory.SearchTrace, bool) {
	var trace memory.SearchTrace
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return trace, false
	}
	if err := json.Unmarshal([]byte(payload), &trace); err != nil {
		return trace, false
	}
	return trace, strings.TrimSpace(trace.Query) != "" || len(trace.Results) > 0 || len(trace.Hops) > 0
}

func RenderTelegramMemoryTraceCard(trace memory.SearchTrace, level string, maxResults, maxHops int) string {
	if strings.TrimSpace(trace.Query) == "" && len(trace.Results) == 0 && len(trace.Hops) == 0 {
		return ""
	}
	if maxResults <= 0 {
		maxResults = 6
	}
	if maxHops <= 0 {
		maxHops = 8
	}
	level = strings.ToLower(strings.TrimSpace(level))
	if level == "" {
		level = "summary"
	}

	lines := make([]string, 0, 4+maxResults+maxHops)
	if query := ClipOneLine(trace.Query, 120); query != "" {
		lines = append(lines, "query: "+query)
	}
	meta := fmt.Sprintf("depth=%d results=%d hops=%d", trace.GraphDepth, len(trace.Results), len(trace.Hops))
	if trace.DurationMS > 0 {
		meta += fmt.Sprintf(" duration=%dms", trace.DurationMS)
	}
	lines = append(lines, meta)
	if source := ClipOneLine(trace.Source, 96); source != "" {
		lines = append(lines, "source: "+source)
	}

	resultRefs := MemoryTraceResultRefs(trace.Results)
	if len(trace.Results) > 0 {
		lines = append(lines, "direct:")
		for i, result := range trace.Results {
			if i >= maxResults {
				lines = append(lines, fmt.Sprintf("  ... %d more nodes", len(trace.Results)-i))
				break
			}
			lines = append(lines, "  "+ClipOneLine(FormatMemoryTraceNode(result), 180))
			if level == "full" && strings.TrimSpace(result.ContentPreview) != "" {
				lines = append(lines, "      "+ClipOneLine(result.ContentPreview, 160))
			}
		}
	}

	if len(trace.Hops) > 0 {
		lines = append(lines, "paths:")
		for i, hop := range trace.Hops {
			if i >= maxHops {
				lines = append(lines, fmt.Sprintf("  ... %d more links", len(trace.Hops)-i))
				break
			}
			lines = append(lines, "  "+ClipOneLine(FormatMemoryTraceLink(hop, resultRefs), 220))
		}
	}

	if len(trace.Temporal) > 0 {
		lines = append(lines, "notes: "+ClipOneLine(strings.Join(LimitMemoryTraceStrings(trace.Temporal, 2), " | "), 180))
	}
	if len(trace.Warnings) > 0 {
		lines = append(lines, "warnings: "+ClipOneLine(strings.Join(LimitMemoryTraceStrings(trace.Warnings, 2), " | "), 180))
	}
	return "<b>Memory Trace</b>\n<pre><code>" + html.EscapeString(strings.Join(lines, "\n")) + "</code></pre>"
}

func FirstMemoryTraceText(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func MemoryTraceResultRefs(results []memory.SearchTraceResult) map[string]string {
	refs := make(map[string]string, len(results))
	for _, result := range results {
		if result.ID == "" {
			continue
		}
		refs[result.ID] = FirstMemoryTraceText(result.Ref, result.ID)
	}
	return refs
}

func FormatMemoryTraceNode(result memory.SearchTraceResult) string {
	ref := FirstMemoryTraceText(result.Ref, result.ID, "(unknown)")
	kind := "graph"
	if result.DirectScore > 0 {
		kind = "direct"
	}
	return fmt.Sprintf("[%d] %s (%s %s/%s score=%.2f)", result.Rank, ref, kind, result.Category, result.Tier, result.Score)
}

func FormatMemoryTraceLink(hop memory.SearchTraceHop, resultRefs map[string]string) string {
	from := FirstMemoryTraceText(hop.FromRef, resultRefs[hop.FromID], hop.FromID, "(unknown)")
	to := FirstMemoryTraceText(hop.ToRef, resultRefs[hop.ToID], hop.ToID, "(unknown)")
	edge := strings.TrimSpace(hop.Kind)
	if via := strings.TrimSpace(hop.Via); via != "" {
		if edge != "" {
			edge += ":" + via
		} else {
			edge = via
		}
	}
	if edge == "" {
		edge = "link"
	}
	if hop.Depth > 0 {
		edge = fmt.Sprintf("d%d %s", hop.Depth, edge)
	}
	if hop.Boost != 0 {
		edge += fmt.Sprintf(" %+.2f", hop.Boost)
	}
	return fmt.Sprintf("%s --[%s]--> %s", from, edge, to)
}

func LimitMemoryTraceStrings(values []string, limit int) []string {
	if limit <= 0 || len(values) <= limit {
		return values
	}
	return values[:limit]
}

func HumanizeToolCallProgress(step int, name, args string) string {
	if step <= 0 {
		step = 1
	}
	n := strings.TrimSpace(name)
	if n == "" {
		n = "unknown_tool"
	}
	lowerName := strings.ToLower(n)
	parsed := ParseToolCallArgs(args)

	if strings.Contains(lowerName, "skill") {
		return "先接入现成的处理链路，确认这一步该怎么处理。"
	}

	switch lowerName {
	case "web_search":
		if q := PickArg(parsed, "query", "q", "keyword"); q != "" {
			return fmt.Sprintf("先查一轮公开资料，重点看 %s。", QuoteAndTrim(q, 90))
		}
		return "先查一轮公开资料，看看有没有可靠线索。"
	case "web_fetch":
		if u := PickArg(parsed, "url", "uri", "link"); u != "" {
			return fmt.Sprintf("先打开原文核对细节：%s。", ClipOneLine(u, 100))
		}
		return "先读取网页正文，确认原文内容。"
	case "shell":
		if cmd := PickArg(parsed, "cmd", "command", "script"); cmd != "" {
			return fmt.Sprintf("先在终端核对现状，执行：%s。", ClipOneLine(cmd, 100))
		}
		return "先在终端核对当前环境状态。"
	case "file_read":
		if p := PickArg(parsed, "path", "file", "filepath", "filename"); p != "" {
			return fmt.Sprintf("先查看文件 %s，确认当前内容。", ClipOneLine(p, 100))
		}
		return "先读取相关文件，确认判断依据。"
	case "file_write":
		if p := PickArg(parsed, "path", "file", "filepath", "filename"); p != "" {
			return fmt.Sprintf("先把修改写入 %s。", ClipOneLine(p, 100))
		}
		return "先把这次改动真正落盘。"
	case "recall":
		if q := PickArg(parsed, "query", "q", "keyword"); q != "" {
			return fmt.Sprintf("先回看之前的上下文，确认 %s 有没有旧线索。", QuoteAndTrim(q, 90))
		}
		return "先回看之前的上下文，避免漏掉已有信息。"
	case "remember":
		return "先把这条信息记下来，后面可以直接复用。"
	case "current_time":
		return "先确认当前时间和时区，避免时间线出错。"
	}

	if keyHint := PickArg(parsed,
		"query", "url", "path", "cmd", "command",
		"title", "task_id", "name"); keyHint != "" {
		return fmt.Sprintf("先用工具 %s 处理 %s，补齐这一步所需信息。", n, ClipOneLine(keyHint, 100))
	}
	return fmt.Sprintf("先调用 %s，补齐这一小段信息。", n)
}

func HumanizeToolResultProgress(step int, name, result string) string {
	if step <= 0 {
		step = 1
	}
	n := strings.TrimSpace(name)
	lowerName := strings.ToLower(n)

	switch lowerName {
	case "web_search":
		return "搜索结果已经拿到，开始筛选有效信息。"
	case "web_fetch":
		return "原文已经读完，开始提取关键细节。"
	case "shell":
		return "命令已经执行完，正在根据输出继续判断。"
	case "file_read":
		return "文件已经看过，开始收拢重点。"
	case "file_write":
		return "改动已经写入，接着做一轮确认。"
	case "recall":
		return "之前的上下文已经回看完，继续往下收束。"
	case "remember":
		return "这条信息已经记下，后面可以直接复用。"
	case "current_time":
		return "时间线已经核对完成。"
	}

	if strings.Contains(lowerName, "skill") {
		return "处理链路已经跑完，开始收束结论。"
	}

	if summary := HumanizeToolResult(name, result); summary != "" {
		summary = strings.TrimPrefix(summary, "我")
		summary = strings.TrimSpace(summary)
		return EnsureSentenceSuffix("这一步已经完成，" + summary)
	}
	return "这一步已经有结果，继续往下整理。"
}

func WrapFinalConclusion(finalOutput string) string {
	finalOutput = strings.TrimSpace(finalOutput)
	if finalOutput == "" {
		return "这轮暂时还没有整理出能直接给你的结论。"
	}
	return finalOutput
}

func HumanizeToolCall(name, args string) string {
	n := strings.TrimSpace(name)
	if n == "" {
		n = "unknown_tool"
	}

	parsed := ParseToolCallArgs(args)

	switch n {
	case "web_search":
		if q := PickArg(parsed, "query", "q", "keyword"); q != "" {
			return fmt.Sprintf("正在联网搜索：%s", QuoteAndTrim(q, 80))
		}
		return "正在联网搜索相关信息"

	case "web_fetch":
		if u := PickArg(parsed, "url", "uri", "link"); u != "" {
			return fmt.Sprintf("正在读取网页：%s", ClipOneLine(u, 90))
		}
		return "正在读取网页内容"

	case "shell":
		if cmd := PickArg(parsed, "cmd", "command", "script"); cmd != "" {
			return fmt.Sprintf("正在执行命令：%s", ClipOneLine(cmd, 90))
		}
		return "正在执行终端命令"

	case "file_read":
		if p := PickArg(parsed, "path", "file", "filepath", "filename"); p != "" {
			return fmt.Sprintf("正在读取文件：%s", ClipOneLine(p, 90))
		}
		return "正在读取文件"

	case "file_write":
		if p := PickArg(parsed, "path", "file", "filepath", "filename"); p != "" {
			return fmt.Sprintf("正在写入文件：%s", ClipOneLine(p, 90))
		}
		return "正在写入文件"

	case "current_time":
		return "正在查询当前时间"

	case "remember":
		if content := PickArg(parsed, "content", "text", "memory"); content != "" {
			return fmt.Sprintf("正在保存记忆：%s", ClipOneLine(content, 80))
		}
		return "正在保存记忆"

	case "recall":
		if q := PickArg(parsed, "query", "q", "keyword"); q != "" {
			return fmt.Sprintf("正在检索记忆：%s", QuoteAndTrim(q, 80))
		}
		return "正在检索记忆"
	}

	if keyHint := PickArg(parsed,
		"query", "url", "path", "cmd", "command",
		"title", "task_id", "name"); keyHint != "" {
		return fmt.Sprintf("正在调用工具 %s：%s", n, ClipOneLine(keyHint, 90))
	}
	return fmt.Sprintf("正在调用工具 %s", n)
}

func HumanizeToolResult(name, result string) string {
	n := strings.TrimSpace(name)
	text := ClipOneLine(result, 160)
	if text == "" {
		return ""
	}

	switch n {
	case "web_search":
		if strings.Contains(text, "Results for:") {
			return "已经拿到一批相关结果，里面有几个比较可靠的入口。"
		}
		if strings.Contains(strings.ToLower(text), "no results found") {
			return "已经搜过一轮，但这次还没有拿到像样的结果。"
		}
		return "已经查到一批和这个问题比较接近的资料。"

	case "web_fetch":
		if strings.Contains(strings.ToLower(text), "failed to fetch") {
			return "尝试抓取网页正文，但这页没有顺利拿到。"
		}
		return "网页正文已经过了一遍，里面有几处细节可以直接使用。"

	case "file_read":
		return "已经找到文件里的关键片段。"

	case "recall":
		if strings.Contains(text, "没有找到") {
			return "已经回看过之前的记录，但这件事暂时没有现成线索。"
		}
		return "已经把之前相关的上下文顺了一遍。"

	case "current_time":
		return "已经把时间信息核对过了。"
	}

	return fmt.Sprintf("已经拿到 %s 这一步的结果。", n)
}

func TelegramToolTraceVisibility(name string) string {
	lower := strings.ToLower(strings.TrimSpace(name))
	switch {
	case lower == "":
		return "visible"
	case lower == "__memory_trace":
		return "hidden"
	case lower == "skill_read",
		lower == "remember",
		lower == "recall",
		lower == "rag_search",
		lower == "rag_index",
		strings.HasPrefix(lower, "cron"):
		return "compact"
	case strings.HasPrefix(lower, "skill_") && strings.HasSuffix(lower, "_run"):
		return "compact"
	case strings.HasPrefix(lower, "delegate_"),
		strings.HasPrefix(lower, "autonomy_"),
		strings.HasPrefix(lower, "heartbeat_"):
		return "agent"
	case strings.HasPrefix(lower, "skill_"):
		return "hidden"
	default:
		return "visible"
	}
}

func FormatTelegramToolTraceLine(index int, step TelegramToolTraceStep, visibility string, detailed bool) string {
	return FormatTelegramToolTraceLineWithTemplates(index, step, visibility, detailed, nil)
}

func FormatTelegramToolTraceLineWithTemplates(index int, step TelegramToolTraceStep, visibility string, detailed bool, templates map[string]string) string {
	name := strings.TrimSpace(step.Name)
	if name == "" {
		name = "unknown_tool"
	}
	record := tool.NewTraceRecordWithTemplates(name, step.Args, step.Result, 0, templates)
	success := step.Success
	if record.Error != "" {
		success = false
	}
	status := "✅"
	if !success {
		status = "❌"
	}
	displayName := name
	if visibility == "compact" {
		displayName = CompactToolTraceName(name)
	}

	var line strings.Builder
	line.WriteString(fmt.Sprintf("<b>%d. %s <code>%s</code></b>\n", index, status, html.EscapeString(displayName)))
	line.WriteString("📝 ")
	line.WriteString(html.EscapeString(record.Annotation))
	if !success && record.Error != "" {
		line.WriteString("\n⚠️ <b>错误：</b>")
		line.WriteString(html.EscapeString(record.Error))
	}
	if detailed {
		line.WriteString("\n<blockquote expandable>⚙️ 参数：")
		line.WriteString(html.EscapeString(ClipOneLine(step.Args, 180)))
		if result := ClipOneLine(step.Result, 240); result != "" {
			line.WriteString("\n📊 结果：")
			line.WriteString(html.EscapeString(result))
		}
		line.WriteString("</blockquote>")
	}
	return line.String()
}

func FormatTelegramAgentTraceLine(index int, step TelegramToolTraceStep) string {
	name := strings.TrimSpace(step.Name)
	if name == "" {
		name = "unknown_agent_step"
	}
	status := "✅"
	if !step.Success {
		status = "⚠️"
	}
	detail := CompactAgentTraceDetail(step.Args)
	line := fmt.Sprintf("[%d] %s %s %s", index, CompactAgentTraceName(name), detail, status)
	if result := ClipOneLine(step.Result, 120); result != "" {
		if step.Success {
			line += "\n    Result: " + result
		} else {
			line += "\n    Error: " + result
		}
	}
	return line
}

func CompactToolTraceName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "unknown_tool"
	}
	return name
}

func CompactAgentTraceName(name string) string {
	lower := strings.ToLower(strings.TrimSpace(name))
	switch {
	case strings.HasPrefix(lower, "delegate_"):
		return "delegate"
	case strings.HasPrefix(lower, "autonomy_"):
		return "autonomy"
	case strings.HasPrefix(lower, "heartbeat_"):
		return "heartbeat"
	default:
		return name
	}
}

func CompactAgentTraceDetail(args string) string {
	parsed := ParseToolCallArgs(args)
	if mode := PickArg(parsed, "mode", "agent", "agent_id", "role"); mode != "" {
		return ClipOneLine(mode, 48)
	}
	if task := PickArg(parsed, "task", "title", "name", "command", "query", "prompt"); task != "" {
		return ClipOneLine(task, 48)
	}
	return ""
}

func IsInternalThinkingProgress(content string) bool {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return true
	}
	if ExtractRoundNumber(trimmed) > 0 {
		return true
	}

	lower := strings.ToLower(strings.Trim(trimmed, " .!"))
	return lower == "thinking" || lower == "thinking.."
}

func ParseToolCallArgs(raw string) map[string]any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil
	}
	return m
}

func PickArg(args map[string]any, keys ...string) string {
	if len(args) == 0 {
		return ""
	}
	for _, key := range keys {
		v, ok := args[key]
		if !ok || v == nil {
			continue
		}
		switch x := v.(type) {
		case string:
			if s := strings.TrimSpace(x); s != "" {
				return s
			}
		case []any:
			if len(x) > 0 {
				s := strings.TrimSpace(fmt.Sprintf("%v", x[0]))
				if s != "" {
					return s
				}
			}
		default:
			s := strings.TrimSpace(fmt.Sprintf("%v", v))
			if s != "" && s != "<nil>" {
				return s
			}
		}
	}
	return ""
}

func ClipOneLine(s string, max int) string {
	s = strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
	if s == "" {
		return s
	}
	if max <= 0 {
		max = 80
	}
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func QuoteAndTrim(s string, max int) string {
	s = ClipOneLine(s, max)
	if s == "" {
		return s
	}
	return "「" + s + "」"
}

func EnsureSentenceSuffix(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	if strings.HasSuffix(s, "。") || strings.HasSuffix(s, "！") || strings.HasSuffix(s, "？") ||
		strings.HasSuffix(s, ".") || strings.HasSuffix(s, "!") || strings.HasSuffix(s, "?") {
		return s
	}
	return s + "。"
}
