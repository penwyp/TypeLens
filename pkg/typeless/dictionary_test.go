package typeless

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestUniqueTrimmedTerms(t *testing.T) {
	t.Parallel()

	terms := []string{
		" Claude ",
		"claude",
		"CLAUDE",
		"",
		" codex ",
		"Codex",
		"claude code",
	}

	got := uniqueTrimmedTerms(terms)
	want := []string{"Claude", "codex", "claude code"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("uniqueTrimmedTerms() = %#v, want %#v", got, want)
	}
}

func TestNormalizeDictionaryTermKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "trim and lower", in: " Claude ", want: "claude"},
		{name: "empty after trim", in: "  ", want: ""},
		{name: "keep inner spaces", in: "Claude Code", want: "claude code"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := normalizeDictionaryTermKey(tt.in); got != tt.want {
				t.Fatalf("normalizeDictionaryTermKey() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsSkippableDictionaryAddError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "400 duplicate",
			err:  assertError(`调用 Typeless Node 桥接失败: Error: HTTP 400: {"code":10302,"detail":"Term already exists, cannot add duplicate","status":"FAIL","msg":"HTTPException"}`),
			want: true,
		},
		{
			name: "401 unauthorized",
			err:  assertError("调用 Typeless Node 桥接失败: Error: HTTP 401: unauthorized"),
			want: true,
		},
		{
			name: "429 too many requests",
			err:  assertError("调用 Typeless Node 桥接失败: Error: HTTP 429: too many requests"),
			want: true,
		},
		{
			name: "500 server error",
			err:  assertError("调用 Typeless Node 桥接失败: Error: HTTP 500: internal server error"),
			want: false,
		},
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := isSkippableDictionaryAddError(tt.err); got != tt.want {
				t.Fatalf("isSkippableDictionaryAddError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsSkippableDictionaryDeleteError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "400 delete conflict",
			err:  assertError("调用 Typeless Node 桥接失败: Error: HTTP 400: invalid word id"),
			want: true,
		},
		{
			name: "404 missing word",
			err:  assertError("调用 Typeless Node 桥接失败: Error: HTTP 404: word not found"),
			want: true,
		},
		{
			name: "401 unauthorized should not skip",
			err:  assertError("调用 Typeless Node 桥接失败: Error: HTTP 401: unauthorized"),
			want: false,
		},
		{
			name: "429 throttled should not skip",
			err:  assertError("调用 Typeless Node 桥接失败: Error: HTTP 429: too many requests"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := isSkippableDictionaryDeleteError(tt.err); got != tt.want {
				t.Fatalf("isSkippableDictionaryDeleteError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildResetPlan(t *testing.T) {
	t.Parallel()

	existingWords := []DictionaryWord{
		{ID: "1", Term: "claude"},
		{ID: "2", Term: "codex"},
		{ID: "3", Term: "anthropic"},
	}
	uniqueTerms := []string{"Claude", "cursor", "claude code"}

	plan := buildResetPlan(existingWords, uniqueTerms)
	if plan.Kept != 1 {
		t.Fatalf("buildResetPlan().Kept = %d, want 1", plan.Kept)
	}

	gotDelete := []string{plan.DeleteWords[0].Term, plan.DeleteWords[1].Term}
	wantDelete := []string{"codex", "anthropic"}
	if !reflect.DeepEqual(gotDelete, wantDelete) {
		t.Fatalf("buildResetPlan().DeleteWords = %#v, want %#v", gotDelete, wantDelete)
	}

	wantAdd := []string{"cursor", "claude code"}
	if !reflect.DeepEqual(plan.AddTerms, wantAdd) {
		t.Fatalf("buildResetPlan().AddTerms = %#v, want %#v", plan.AddTerms, wantAdd)
	}
}

func TestResolveNodeBinaryFromPath(t *testing.T) {
	tempDir := t.TempDir()
	nodePath := filepath.Join(tempDir, "node")
	if err := os.WriteFile(nodePath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", tempDir)

	got, err := resolveNodeBinary()
	if err != nil {
		t.Fatalf("resolveNodeBinary() error = %v", err)
	}
	if got != nodePath {
		t.Fatalf("resolveNodeBinary() = %q, want %q", got, nodePath)
	}
}

func assertError(message string) error {
	return &staticError{message: message}
}

type staticError struct {
	message string
}

func (e *staticError) Error() string {
	return e.message
}
