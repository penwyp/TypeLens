package typeless

import (
	"slices"
	"testing"
)

func TestParseCodexSessionLine(t *testing.T) {
	line := []byte(`{"type":"response_item","payload":{"role":"user","content":[{"type":"input_text","text":"请处理 TypeLens 和 ClaudeProbe"}]}}`)
	texts, err := parseAutoImportLine(nil, AutoImportPlatformCodex, "/tmp/session.jsonl", line)
	if err != nil {
		t.Fatalf("parseAutoImportLine() error = %v", err)
	}
	if len(texts) != 1 || texts[0] != "请处理 TypeLens 和 ClaudeProbe" {
		t.Fatalf("unexpected texts: %#v", texts)
	}
}

func TestParseClaudeProjectLine(t *testing.T) {
	line := []byte(`{"type":"user","message":{"role":"user","content":"把 sub2api 自动导入做掉"}}`)
	texts, err := parseAutoImportLine(nil, AutoImportPlatformClaude, "/tmp/project.jsonl", line)
	if err != nil {
		t.Fatalf("parseAutoImportLine() error = %v", err)
	}
	if len(texts) != 1 || texts[0] != "把 sub2api 自动导入做掉" {
		t.Fatalf("unexpected texts: %#v", texts)
	}
}

func TestExtractTokensFromMessage(t *testing.T) {
	tokens := extractTokensFromMessage("请处理 TypeLens、ClaudeProbe、agent_os 和自动导入逻辑")
	for _, want := range []string{"TypeLens", "ClaudeProbe", "Claude", "Probe", "agent_os", "agent", "os"} {
		if !slices.Contains(tokens, want) {
			t.Fatalf("tokens %v does not contain %q", tokens, want)
		}
	}
	if !slices.ContainsFunc(tokens, func(token string) bool {
		return len([]rune(token)) >= 4 && slices.Contains([]rune(token), '导')
	}) {
		t.Fatalf("tokens %v does not contain expected Chinese candidate", tokens)
	}
}

func TestExtractAutoImportCandidatesDeduplicatesAcrossPlatforms(t *testing.T) {
	candidates := extractAutoImportCandidates([]autoImportMessage{
		{Platform: AutoImportPlatformCodex, Text: "请处理 TypeLens"},
		{Platform: AutoImportPlatformClaude, Text: "TypeLens 这里也有"},
	})
	var matches []AutoImportCandidate
	for _, candidate := range candidates {
		if candidate.NormalizedTerm == "typelens" {
			matches = append(matches, candidate)
		}
	}
	if len(matches) != 1 {
		t.Fatalf("typelens candidates = %d, want 1, all=%v", len(matches), candidates)
	}
	if matches[0].Hits != 2 {
		t.Fatalf("typelens hits = %d, want 2", matches[0].Hits)
	}
}

func TestMergePendingCandidates(t *testing.T) {
	existing := []PendingDictionaryWord{
		{Term: "TypeLens", Status: AutoImportStatusPending},
	}
	candidates := []AutoImportCandidate{
		{Term: "TypeLens", Platform: AutoImportPlatformCodex},
		{Term: "ClaudeProbe", Platform: AutoImportPlatformClaude, Examples: []string{"example"}},
	}
	words, added := MergePendingCandidates(existing, candidates)
	if added != 1 {
		t.Fatalf("MergePendingCandidates() added = %d, want 1", added)
	}
	if len(words) != 2 {
		t.Fatalf("MergePendingCandidates() len = %d, want 2", len(words))
	}
	if words[1].Term != "ClaudeProbe" || words[1].Status != AutoImportStatusPending {
		t.Fatalf("unexpected merged word: %#v", words[1])
	}
}
