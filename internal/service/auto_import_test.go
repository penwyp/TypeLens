package service

import (
	"testing"
	"time"

	"github.com/penwyp/typelens/pkg/typeless"
)

func TestIsRetryableAutoImportWord(t *testing.T) {
	now := time.Date(2026, 4, 27, 15, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		word typeless.PendingDictionaryWord
		want bool
	}{
		{
			name: "pending always retries",
			word: typeless.PendingDictionaryWord{Status: typeless.AutoImportStatusPending},
			want: true,
		},
		{
			name: "syncing resumes after restart",
			word: typeless.PendingDictionaryWord{Status: typeless.AutoImportStatusSyncing},
			want: true,
		},
		{
			name: "recent failed waits for retry delay",
			word: typeless.PendingDictionaryWord{
				Status:    typeless.AutoImportStatusFailed,
				UpdatedAt: now.Add(-failedAutoImportRetryDelay / 2).Format(time.RFC3339),
			},
			want: false,
		},
		{
			name: "old failed retries",
			word: typeless.PendingDictionaryWord{
				Status:    typeless.AutoImportStatusFailed,
				UpdatedAt: now.Add(-failedAutoImportRetryDelay).Format(time.RFC3339),
			},
			want: true,
		},
		{
			name: "synced does not retry",
			word: typeless.PendingDictionaryWord{Status: typeless.AutoImportStatusSynced},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isRetryableAutoImportWord(tt.word, now); got != tt.want {
				t.Fatalf("isRetryableAutoImportWord() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsSyncableAutoImportWord(t *testing.T) {
	for _, status := range []string{
		typeless.AutoImportStatusPending,
		typeless.AutoImportStatusSyncing,
		typeless.AutoImportStatusFailed,
	} {
		if !isSyncableAutoImportWord(typeless.PendingDictionaryWord{Status: status}) {
			t.Fatalf("isSyncableAutoImportWord(%q) = false, want true", status)
		}
	}
	if isSyncableAutoImportWord(typeless.PendingDictionaryWord{Status: typeless.AutoImportStatusSynced}) {
		t.Fatalf("isSyncableAutoImportWord(synced) = true, want false")
	}
}

func TestHasSyncableAutoImportWords(t *testing.T) {
	words := []typeless.PendingDictionaryWord{
		{Term: "synced", Status: typeless.AutoImportStatusSynced},
		{Term: "failed", Status: typeless.AutoImportStatusFailed},
	}
	if !hasSyncableAutoImportWords(words) {
		t.Fatalf("hasSyncableAutoImportWords() = false, want true")
	}
}

func TestNextSyncableAutoImportWordDoesNotRetryCurrentRunFailure(t *testing.T) {
	runStartedAt := time.Date(2026, 4, 27, 15, 0, 0, 0, time.UTC)
	words := []typeless.PendingDictionaryWord{
		{
			Term:      "fresh-failure",
			Status:    typeless.AutoImportStatusFailed,
			UpdatedAt: runStartedAt.Add(time.Second).Format(time.RFC3339),
		},
		{
			Term:   "pending",
			Status: typeless.AutoImportStatusPending,
		},
	}

	word, ok := nextSyncableAutoImportWord(words, runStartedAt)
	if !ok {
		t.Fatalf("nextSyncableAutoImportWord() ok = false, want true")
	}
	if word.Term != "pending" {
		t.Fatalf("nextSyncableAutoImportWord() term = %q, want pending", word.Term)
	}
}

func TestNextSyncableAutoImportWordRetriesOldFailure(t *testing.T) {
	runStartedAt := time.Date(2026, 4, 27, 15, 0, 0, 0, time.UTC)
	words := []typeless.PendingDictionaryWord{
		{
			Term:      "old-failure",
			Status:    typeless.AutoImportStatusFailed,
			UpdatedAt: runStartedAt.Add(-time.Second).Format(time.RFC3339),
		},
	}

	word, ok := nextSyncableAutoImportWord(words, runStartedAt)
	if !ok {
		t.Fatalf("nextSyncableAutoImportWord() ok = false, want true")
	}
	if word.Term != "old-failure" {
		t.Fatalf("nextSyncableAutoImportWord() term = %q, want old-failure", word.Term)
	}
}

func TestNextFailedAutoImportRetryDelay(t *testing.T) {
	now := time.Date(2026, 4, 27, 15, 0, 0, 0, time.UTC)
	delay, ok := nextFailedAutoImportRetryDelay([]typeless.PendingDictionaryWord{
		{
			Term:      "recent",
			Status:    typeless.AutoImportStatusFailed,
			UpdatedAt: now.Add(-failedAutoImportRetryDelay / 2).Format(time.RFC3339),
		},
	}, now)
	if !ok {
		t.Fatalf("nextFailedAutoImportRetryDelay() ok = false, want true")
	}
	if delay != failedAutoImportRetryDelay/2 {
		t.Fatalf("nextFailedAutoImportRetryDelay() delay = %s, want %s", delay, failedAutoImportRetryDelay/2)
	}

	delay, ok = nextFailedAutoImportRetryDelay([]typeless.PendingDictionaryWord{
		{
			Term:      "old",
			Status:    typeless.AutoImportStatusFailed,
			UpdatedAt: now.Add(-failedAutoImportRetryDelay).Format(time.RFC3339),
		},
	}, now)
	if !ok || delay != 0 {
		t.Fatalf("nextFailedAutoImportRetryDelay() = (%s, %v), want (0, true)", delay, ok)
	}

	_, ok = nextFailedAutoImportRetryDelay([]typeless.PendingDictionaryWord{
		{Term: "synced", Status: typeless.AutoImportStatusSynced},
	}, now)
	if ok {
		t.Fatalf("nextFailedAutoImportRetryDelay() ok = true, want false")
	}
}
