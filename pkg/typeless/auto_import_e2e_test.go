package typeless

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestScanAutoImportCandidatesE2E(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	codexRoot := filepath.Join(root, "codex")
	claudeRoot := filepath.Join(root, "claude")
	mustMkdirAll(t, filepath.Join(codexRoot, "sessions", "2026", "04", "27"))
	mustMkdirAll(t, filepath.Join(claudeRoot, "projects", "demo"))

	writeFile(t, filepath.Join(codexRoot, "history.jsonl"), strings.Join([]string{
		`{"session_id":"1","ts":1,"text":"请把 TypeLens 自动导入做好，并处理 ClaudeProbe 和 agent_os。"}`,
		`{"session_id":"2","ts":2,"text":"把 TiDBCluster 的分词结果看一下。"}`,
	}, "\n")+"\n")

	writeFile(t, filepath.Join(codexRoot, "sessions", "2026", "04", "27", "session.jsonl"), strings.Join([]string{
		`{"type":"response_item","payload":{"role":"user","content":[{"type":"input_text","text":"帮我扫描 TypeLens、ClaudeProbe、agent_os 和 sub2api。"}]}}`,
		`{"type":"response_item","payload":{"role":"assistant","content":[{"type":"output_text","text":"ignore me"}]}}`,
	}, "\n")+"\n")

	writeFile(t, filepath.Join(claudeRoot, "history.jsonl"), `{"display":"把 TypeLens 自动导入预览结果输出出来"}`+"\n")
	writeFile(t, filepath.Join(claudeRoot, "projects", "demo", "project.jsonl"), strings.Join([]string{
		`{"type":"user","message":{"role":"user","content":"请继续处理 ClaudeProbe、TiDBCluster 和 agent_os"}}`,
		`{"type":"assistant","message":{"role":"assistant","content":"ignore me"}}`,
	}, "\n")+"\n")

	result, err := ScanAutoImportCandidates(context.Background(), []AutoImportSource{
		{Platform: AutoImportPlatformCodex, Enabled: true, Workdir: codexRoot},
		{Platform: AutoImportPlatformClaude, Enabled: true, Workdir: claudeRoot},
	}, nil, nil)
	if err != nil {
		t.Fatalf("ScanAutoImportCandidates() error = %v", err)
	}

	if result.ScannedFiles != 4 {
		t.Fatalf("ScannedFiles = %d, want 4", result.ScannedFiles)
	}
	if result.ParsedMessages != 5 {
		t.Fatalf("ParsedMessages = %d, want 5", result.ParsedMessages)
	}
	if result.FilteredCandidates == 0 || len(result.Items) == 0 {
		t.Fatalf("FilteredCandidates = %d, items=%v, want non-empty", result.FilteredCandidates, result.Items)
	}

	assertContainsCandidate(t, result.Items, "TypeLens")
	assertContainsCandidate(t, result.Items, "ClaudeProbe")
	assertContainsCandidate(t, result.Items, "agent_os")
	assertContainsCandidate(t, result.Items, "sub2api")
	assertContainsCandidate(t, result.Items, "TiDBCluster")

	typeLens := findCandidate(t, result.Items, "typelens")
	if typeLens.Hits < 3 {
		t.Fatalf("typelens hits = %d, want >= 3", typeLens.Hits)
	}
	if len(typeLens.Examples) == 0 {
		t.Fatalf("typelens examples empty")
	}
}

func TestScanAutoImportCandidatesE2EHandlesLongJSONLLine(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	codexRoot := filepath.Join(root, "codex")
	mustMkdirAll(t, codexRoot)
	longBody := strings.Repeat("TypeLens agent_os ClaudeProbe ", 180000)
	writeFile(t, filepath.Join(codexRoot, "history.jsonl"), fmt.Sprintf(`{"session_id":"1","ts":1,"text":%q}`+"\n", longBody))

	result, err := ScanAutoImportCandidates(context.Background(), []AutoImportSource{
		{Platform: AutoImportPlatformCodex, Enabled: true, Workdir: codexRoot},
	}, nil, nil)
	if err != nil {
		t.Fatalf("ScanAutoImportCandidates() long line error = %v", err)
	}
	if result.ParsedMessages != 1 {
		t.Fatalf("ParsedMessages = %d, want 1", result.ParsedMessages)
	}
	assertContainsCandidate(t, result.Items, "TypeLens")
	assertContainsCandidate(t, result.Items, "agent_os")
	assertContainsCandidate(t, result.Items, "ClaudeProbe")
}

func BenchmarkScanAutoImportCandidatesParallel(b *testing.B) {
	root := b.TempDir()
	codexRoot := filepath.Join(root, "codex")
	claudeRoot := filepath.Join(root, "claude")
	mustMkdirAllB(b, filepath.Join(codexRoot, "sessions"))
	mustMkdirAllB(b, filepath.Join(claudeRoot, "projects"))

	var codexLines []string
	for index := 0; index < 64; index++ {
		filePath := filepath.Join(codexRoot, "sessions", fmt.Sprintf("session-%02d.jsonl", index))
		codexLines = codexLines[:0]
		for line := 0; line < 120; line++ {
			codexLines = append(codexLines, fmt.Sprintf(`{"type":"response_item","payload":{"role":"user","content":[{"type":"input_text","text":"TypeLens ClaudeProbe agent_os sub2api line-%d-%d"}]}}`, index, line))
		}
		if err := os.WriteFile(filePath, []byte(strings.Join(codexLines, "\n")+"\n"), 0o644); err != nil {
			b.Fatalf("write %s: %v", filePath, err)
		}
	}
	if err := os.WriteFile(filepath.Join(claudeRoot, "history.jsonl"), []byte(`{"display":"TypeLens ClaudeProbe agent_os sub2api benchmark"}`+"\n"), 0o644); err != nil {
		b.Fatalf("write claude history: %v", err)
	}

	sources := []AutoImportSource{
		{Platform: AutoImportPlatformCodex, Enabled: true, Workdir: codexRoot},
		{Platform: AutoImportPlatformClaude, Enabled: true, Workdir: claudeRoot},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err := ScanAutoImportCandidates(context.Background(), sources, nil, nil)
		if err != nil {
			b.Fatalf("ScanAutoImportCandidates() error = %v", err)
		}
		if len(result.Items) == 0 {
			b.Fatalf("empty result")
		}
	}
}

func assertContainsCandidate(t *testing.T, items []AutoImportCandidate, term string) {
	t.Helper()
	if !slices.ContainsFunc(items, func(item AutoImportCandidate) bool {
		return item.Term == term || item.NormalizedTerm == normalizeDictionaryTermKey(term)
	}) {
		t.Fatalf("items do not contain %q: %#v", term, items)
	}
}

func findCandidate(t *testing.T, items []AutoImportCandidate, normalizedTerm string) AutoImportCandidate {
	t.Helper()
	for _, item := range items {
		if item.NormalizedTerm == normalizedTerm {
			return item
		}
	}
	t.Fatalf("candidate %q not found", normalizedTerm)
	return AutoImportCandidate{}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func mustMkdirAllB(b *testing.B, path string) {
	b.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		b.Fatalf("mkdir %s: %v", path, err)
	}
}
