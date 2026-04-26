package typeless

import (
	"regexp"
	"testing"
)

func TestMatchTranscriptRecord(t *testing.T) {
	t.Parallel()

	record := TranscriptRecord{Text: "Hello Codex from Typeless"}
	tests := []struct {
		name    string
		options TranscriptQueryOptions
		want    bool
	}{
		{
			name: "empty filter matches",
			want: true,
		},
		{
			name: "keyword is case insensitive",
			options: TranscriptQueryOptions{
				Keyword: "codex",
			},
			want: true,
		},
		{
			name: "keyword miss",
			options: TranscriptQueryOptions{
				Keyword: "claude",
			},
			want: false,
		},
		{
			name: "regex match",
			options: TranscriptQueryOptions{
				Regex: regexp.MustCompile(`Typeless$`),
			},
			want: true,
		},
		{
			name: "regex miss",
			options: TranscriptQueryOptions{
				Regex: regexp.MustCompile(`^Typeless`),
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := matchTranscriptRecord(record, tt.options); got != tt.want {
				t.Fatalf("matchTranscriptRecord() = %v, want %v", got, tt.want)
			}
		})
	}
}
