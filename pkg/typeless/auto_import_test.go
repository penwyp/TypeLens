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
	for _, want := range []string{"TypeLens", "ClaudeProbe", "agent_os"} {
		if !slices.Contains(tokens, want) {
			t.Fatalf("tokens %v does not contain %q", tokens, want)
		}
	}
}

func TestExtractTokensFromMessageMergesProtectedPhrases(t *testing.T) {
	tokens := extractTokensFromMessage("请解释 Claude Code 的 keyword extraction 方案")
	for _, want := range []string{"Claude_Code", "keyword_extraction"} {
		if !slices.Contains(tokens, want) {
			t.Fatalf("tokens %v does not contain %q", tokens, want)
		}
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

func TestFilterVisiblePendingWordsHidesSyncedAndFailed(t *testing.T) {
	words := []PendingDictionaryWord{
		{Term: "Pending", Status: AutoImportStatusPending},
		{Term: "Syncing", Status: AutoImportStatusSyncing},
		{Term: "Failed", Status: AutoImportStatusFailed},
		{Term: "Synced", Status: AutoImportStatusSynced},
	}

	visible := FilterVisiblePendingWords(words)
	if len(visible) != 2 {
		t.Fatalf("FilterVisiblePendingWords() len = %d, want 2: %#v", len(visible), visible)
	}
	for _, want := range []string{"Pending", "Syncing"} {
		if !slices.ContainsFunc(visible, func(word PendingDictionaryWord) bool { return word.Term == want }) {
			t.Fatalf("FilterVisiblePendingWords() missing %q: %#v", want, visible)
		}
	}
	for _, rejected := range []string{"Failed", "Synced"} {
		if slices.ContainsFunc(visible, func(word PendingDictionaryWord) bool { return word.Term == rejected }) {
			t.Fatalf("FilterVisiblePendingWords() unexpectedly contains %q: %#v", rejected, visible)
		}
	}
}

func TestExtractAutoImportCandidatesDropsPlainEnglishNoise(t *testing.T) {
	candidates := extractAutoImportCandidates([]autoImportMessage{
		{Platform: AutoImportPlatformCodex, Text: "please update the response and create the build result"},
		{Platform: AutoImportPlatformCodex, Text: "请继续处理 TypeLens 和 ClaudeProbe"},
		{Platform: AutoImportPlatformClaude, Text: "请继续处理 TypeLens、agent_os 和 sub2api"},
	})

	got := make([]string, 0, len(candidates))
	for _, item := range candidates {
		got = append(got, item.NormalizedTerm)
	}
	for _, rejected := range []string{"update", "response", "build", "result"} {
		if slices.Contains(got, rejected) {
			t.Fatalf("candidates unexpectedly contain %q: %v", rejected, got)
		}
	}
	for _, expected := range []string{"typelens", "claudeprobe", "agent_os", "sub2api"} {
		if !slices.Contains(got, expected) {
			t.Fatalf("candidates missing %q: %v", expected, got)
		}
	}
	for _, rejected := range []string{"type", "lens", "claude", "probe", "agent"} {
		if slices.Contains(got, rejected) {
			t.Fatalf("candidates unexpectedly contain fragment %q: %v", rejected, got)
		}
	}
}

func TestLimitAutoImportCandidatesKeepsOnlyTopLimit(t *testing.T) {
	candidates := make([]AutoImportCandidate, 0, autoImportMaxFinalCandidates+20)
	for index := 0; index < autoImportMaxFinalCandidates+20; index++ {
		term := "ProjToken" + string(rune('A'+(index%26))) + string(rune('a'+((index/26)%26))) + string(rune('a'+((index/676)%26)))
		candidates = append(candidates, AutoImportCandidate{
			Term:           term,
			NormalizedTerm: normalizeDictionaryTermKey(term),
			Hits:           autoImportMaxFinalCandidates + 20 - index,
		})
	}

	ranked := limitAutoImportCandidates(candidates)
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
