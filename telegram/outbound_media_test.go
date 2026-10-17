package telegram

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseOutboundMediaResponseDirective(t *testing.T) {
	text, media := parseOutboundMediaResponse("Here is the report\n\ntg://document /tmp/report.pdf monthly report")
	if text != "Here is the report" {
		t.Fatalf("unexpected text: %q", text)
	}
	if len(media) != 1 {
		t.Fatalf("expected 1 media item, got %d", len(media))
	}
	if media[0].Kind != outboundMediaDocument {
		t.Fatalf("expected document media, got %q", media[0].Kind)
	}
	if media[0].Source != "/tmp/report.pdf" {
		t.Fatalf("unexpected source: %q", media[0].Source)
	}
	if media[0].Caption != "monthly report" {
		t.Fatalf("unexpected caption: %q", media[0].Caption)
	}
}

func TestParseOutboundMediaResponseMarkdownImage(t *testing.T) {
	text, media := parseOutboundMediaResponse("Take a look:\n\n![trend](https://example.com/trend.png)")
	if text != "Take a look:" {
		t.Fatalf("unexpected text: %q", text)
	}
	if len(media) != 1 || media[0].Kind != outboundMediaPhoto {
		t.Fatalf("expected one photo media item, got %#v", media)
	}
	if media[0].Caption != "trend" {
		t.Fatalf("unexpected caption: %q", media[0].Caption)
	}
}

func TestParseOutboundMediaResponseMarkdownSandboxLink(t *testing.T) {
	text, media := parseOutboundMediaResponse("PNG 在这里：[quadratic.png](sandbox:/tmp/quadratic.png)")
	if text != "PNG 在这里：[quadratic.png](sandbox:/tmp/quadratic.png)" {
		t.Fatalf("unexpected text: %q", text)
	}
	if len(media) != 0 {
		t.Fatalf("markdown link should not auto-send media, got %#v", media)
	}
}

func TestParseOutboundMediaResponseImplicitDocument(t *testing.T) {
	text, media := parseOutboundMediaResponse("/tmp/output.pdf")
	if text != "/tmp/output.pdf" {
		t.Fatalf("expected path to remain text, got %q", text)
	}
	if len(media) != 0 {
		t.Fatalf("implicit document should not auto-send media, got %#v", media)
	}
}

func TestParseOutboundMediaResponseImplicitAudioDocument(t *testing.T) {
	text, media := parseOutboundMediaResponse("/tmp/output.mp3")
	if text != "/tmp/output.mp3" {
		t.Fatalf("expected path to remain text, got %q", text)
	}
	if len(media) != 0 {
		t.Fatalf("implicit audio path should not auto-send media, got %#v", media)
	}
}

func TestResolveOutboundMediaResponseMediaTag(t *testing.T) {
	text, media, err := resolveOutboundMediaResponse("已生成文件\nMEDIA:/tmp/report.pdf")
	if err != nil {
		t.Fatalf("resolveOutboundMediaResponse: %v", err)
	}
	if text != "已生成文件" {
		t.Fatalf("unexpected text: %q", text)
	}
	if len(media) != 1 || media[0].Kind != outboundMediaDocument {
		t.Fatalf("expected MEDIA tag to resolve to document, got %#v", media)
	}
}

func TestPrepareOutboundMediaResponseDropsMissingLocalFiles(t *testing.T) {
	text, media := parseOutboundMediaResponse("已生成截图\nMEDIA:/tmp/screenshot-that-no-longer-exists.png")
	text, media = prepareOutboundMediaResponse(text, media)
	if len(media) != 0 {
		t.Fatalf("expected missing local media to be dropped, got %#v", media)
	}
	if !strings.Contains(text, "附件文件不存在或已过期") {
		t.Fatalf("expected missing media notice, got %q", text)
	}
}

func TestPrepareOutboundMediaResponseKeepsRemoteFiles(t *testing.T) {
	text, media := parseOutboundMediaResponse("查看图片\nMEDIA:https://example.com/screenshot.png")
	text, media = prepareOutboundMediaResponse(text, media)
	if text != "查看图片" {
		t.Fatalf("unexpected text: %q", text)
	}
	if len(media) != 1 || media[0].Source != "https://example.com/screenshot.png" {
		t.Fatalf("expected remote media to remain, got %#v", media)
	}
}

func TestResolveOutboundMediaResponseWindowsMediaTag(t *testing.T) {
	winPath := `C:\Users\Administrator\.luckyagent\workspace\interview\daily_interview.docx`
	text, media, err := resolveOutboundMediaResponse("DOCX ready\nMEDIA:" + winPath)
	if err != nil {
		t.Fatalf("resolveOutboundMediaResponse: %v", err)
	}
	if text != "DOCX ready" {
		t.Fatalf("unexpected text: %q", text)
	}
	if len(media) != 1 || media[0].Kind != outboundMediaDocument || media[0].Source != winPath {
		t.Fatalf("expected Windows MEDIA tag to resolve to document, got %#v", media)
	}
}

func TestResolveOutboundMediaResponseStripsGeneratedReferencesForMedia(t *testing.T) {
	winPath := `C:\Users\Administrator\.luckyagent\workspace\interview\daily_interview.docx`
	text, media, err := resolveOutboundMediaResponse("DOCX ready\nMEDIA:" + winPath + "\n\nReferences:\n[1] Tool result. Tool: terminal.\n[2] Local file. C:\\tmp\\report.docx")
	if err != nil {
		t.Fatalf("resolveOutboundMediaResponse: %v", err)
	}
	if text != "DOCX ready" {
		t.Fatalf("expected generated references to be stripped, got %q", text)
	}
	if len(media) != 1 || media[0].Kind != outboundMediaDocument {
		t.Fatalf("expected media document, got %#v", media)
	}
}

func TestParseOutboundMediaResponseWindowsTGDirective(t *testing.T) {
	winPath := `C:\Users\Administrator\.luckyagent\workspace\interview\daily_interview.docx`
	text, media := parseOutboundMediaResponse("Here is the DOCX\n\ntg://document " + winPath + " interview")
	if text != "Here is the DOCX" {
		t.Fatalf("unexpected text: %q", text)
	}
	if len(media) != 1 || media[0].Kind != outboundMediaDocument || media[0].Source != winPath || media[0].Caption != "interview" {
		t.Fatalf("expected Windows tg document directive, got %#v", media)
	}
}

func TestResolveOutboundMediaResponseDoesNotSendConfigJSON(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"api_key":"secret"}`), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	text, media, err := resolveOutboundMediaResponse("配置在这里 " + configPath)
	if err != nil {
		t.Fatalf("resolveOutboundMediaResponse: %v", err)
	}
	if text != "配置在这里 "+configPath {
		t.Fatalf("expected config path to remain text, got %q", text)
	}
	if len(media) != 0 {
		t.Fatalf("config.json should not be sent as media, got %#v", media)
	}

	text, media, err = resolveOutboundMediaResponse("MEDIA:" + configPath)
	if err != nil {
		t.Fatalf("resolveOutboundMediaResponse MEDIA config: %v", err)
	}
	if text != "MEDIA:"+configPath {
		t.Fatalf("expected sensitive MEDIA tag to remain text, got %q", text)
	}
	if len(media) != 0 {
		t.Fatalf("explicit config.json MEDIA tag should be blocked, got %#v", media)
	}

	text, media, err = resolveOutboundMediaResponse("tg://document " + configPath)
	if err != nil {
		t.Fatalf("resolveOutboundMediaResponse tg config: %v", err)
	}
	if text != "tg://document "+configPath {
		t.Fatalf("expected sensitive tg directive to remain text, got %q", text)
	}
	if len(media) != 0 {
		t.Fatalf("explicit config.json tg directive should be blocked, got %#v", media)
	}
}

func TestExtractLocalFilesIgnoresCodeBlocksAndInlineCode(t *testing.T) {
	tmpDir := t.TempDir()
	realPath := filepath.Join(tmpDir, "real.png")
	if err := os.WriteFile(realPath, []byte("png"), 0600); err != nil {
		t.Fatalf("write real path: %v", err)
	}

	content := "这个先发给你：" + realPath + "\n\n```bash\ncat " + realPath + "\n```\n还有 `" + realPath + "` 不要当附件。"
	text, media, err := extractLocalFiles(content)
	if err != nil {
		t.Fatalf("extractLocalFiles: %v", err)
	}
	if !strings.Contains(text, "这个先发给你：") {
		t.Fatalf("expected explanatory text, got %q", text)
	}
	if !strings.Contains(text, "```bash") {
		t.Fatalf("code block should remain untouched in text, got %q", text)
	}
	if len(media) != 1 || media[0].Source != realPath {
		t.Fatalf("expected exactly one extracted media file, got %#v", media)
	}
}

func TestExtractLocalFilesSandboxPath(t *testing.T) {
	tmpDir := t.TempDir()
	realPath := filepath.Join(tmpDir, "chart.png")
	if err := os.WriteFile(realPath, []byte("png"), 0600); err != nil {
		t.Fatalf("write sandbox file: %v", err)
	}
	content := "文件在这里 sandbox:" + realPath
	text, media, err := extractLocalFiles(content)
	if err != nil {
		t.Fatalf("extractLocalFiles: %v", err)
	}
	if text != "文件在这里" {
		t.Fatalf("unexpected text: %q", text)
	}
	if len(media) != 1 || media[0].Source != "sandbox:"+realPath {
		t.Fatalf("expected sandbox path media, got %#v", media)
	}
}
