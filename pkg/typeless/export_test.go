package typeless

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestDefaultDictionaryExportFilename(t *testing.T) {
	got := DefaultDictionaryExportFilename(time.Date(2026, 4, 27, 11, 30, 45, 0, time.Local))
	want := "TypeLens-20260427-113045.txt"
	if got != want {
		t.Fatalf("DefaultDictionaryExportFilename() = %q, want %q", got, want)
	}
}

func TestMergeDictionaryExportTerms(t *testing.T) {
	terms := MergeDictionaryExportTerms(
		[]DictionaryWord{
			{Term: "Claude"},
			{Term: "TypeLens"},
		},
		[]PendingDictionaryWord{
			{Term: "TypeLens", Status: AutoImportStatusPending},
			{Term: "agent_os", Status: AutoImportStatusPending},
			{Term: "ignored", Status: AutoImportStatusSynced},
		},
	)
	want := []string{"Claude", "TypeLens", "agent_os"}
	if !reflect.DeepEqual(terms, want) {
		t.Fatalf("MergeDictionaryExportTerms() = %#v, want %#v", terms, want)
	}
}

func TestWriteDictionaryTermsFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dict.txt")
	if err := WriteDictionaryTermsFile(path, []string{"Claude", "", "TypeLens"}); err != nil {
		t.Fatalf("WriteDictionaryTermsFile() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	want := "Claude\nTypeLens\n"
	if string(data) != want {
		t.Fatalf("WriteDictionaryTermsFile() content = %q, want %q", string(data), want)
	}
}
