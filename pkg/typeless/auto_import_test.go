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
	for _, want := range []string{"TypeLens", "ClaudeProbe", "Claude", "Probe", "agent_os", "agent"} {
		if !slices.Contains(tokens, want) {
			t.Fatalf("tokens %v does not contain %q", tokens, want)
		}
	}
	if !slices.Contains(tokens, "导入") {
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

func TestRankAutoImportCandidatesDropsPlainEnglishNoise(t *testing.T) {
	candidates := []AutoImportCandidate{
		{Term: "update", NormalizedTerm: "update", Hits: 9},
		{Term: "response", NormalizedTerm: "response", Hits: 8},
		{Term: "TypeLens", NormalizedTerm: "typelens", Hits: 3},
		{Term: "ClaudeProbe", NormalizedTerm: "claudeprobe", Hits: 2},
		{Term: "agent_os", NormalizedTerm: "agent_os", Hits: 2},
	}

	ranked := rankAutoImportCandidates(candidates, 20)
	got := make([]string, 0, len(ranked))
	for _, item := range ranked {
		got = append(got, item.NormalizedTerm)
	}

	for _, rejected := range []string{"update", "response"} {
		if slices.Contains(got, rejected) {
			t.Fatalf("ranked candidates unexpectedly contain %q: %v", rejected, got)
		}
	}
	for _, expected := range []string{"typelens", "claudeprobe", "agent_os"} {
		if !slices.Contains(got, expected) {
			t.Fatalf("ranked candidates missing %q: %v", expected, got)
		}
	}
}

func TestRankAutoImportCandidatesKeepsOnlyTopLimit(t *testing.T) {
	candidates := make([]AutoImportCandidate, 0, autoImportMaxFinalCandidates+20)
	for index := 0; index < autoImportMaxFinalCandidates+20; index++ {
		term := "ProjToken" + string(rune('A'+(index%26))) + string(rune('a'+((index/26)%26))) + string(rune('a'+((index/676)%26)))
		candidates = append(candidates, AutoImportCandidate{
			Term:           term,
			NormalizedTerm: normalizeDictionaryTermKey(term),
			Hits:           autoImportMaxFinalCandidates + 20 - index,
		})
	}

	ranked := rankAutoImportCandidates(candidates, autoImportMaxFinalCandidates+100)
	if len(ranked) != autoImportMaxFinalCandidates {
		t.Fatalf("ranked len = %d, want %d", len(ranked), autoImportMaxFinalCandidates)
	}
	if ranked[0].Hits < ranked[len(ranked)-1].Hits {
		t.Fatalf("ranked order is unexpected: first=%d last=%d", ranked[0].Hits, ranked[len(ranked)-1].Hits)
	}
}

func TestExtractChineseCandidatesAvoidsLongSegmentExplosion(t *testing.T) {
	tokens := extractChineseCandidates("自动导入预览结果")
	if len(tokens) > 24 {
		t.Fatalf("extractChineseCandidates() produced too many tokens: %d %#v", len(tokens), tokens)
	}
}
