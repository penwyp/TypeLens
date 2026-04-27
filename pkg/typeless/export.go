package typeless

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const exportFileTimeFormat = "20060102-150405"

func DefaultDictionaryExportDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Downloads"), nil
}

func DefaultDictionaryExportFilename(now time.Time) string {
	return "TypeLens-" + now.Format(exportFileTimeFormat) + ".txt"
}

func MergeDictionaryExportTerms(words []DictionaryWord, pending []PendingDictionaryWord) []string {
	seen := make(map[string]struct{}, len(words)+len(pending))
	terms := make([]string, 0, len(words)+len(pending))

	appendTerm := func(term string) {
		term = strings.TrimSpace(term)
		if term == "" {
			return
		}
		key := normalizeDictionaryTermKey(term)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		terms = append(terms, term)
	}

	for _, word := range words {
		appendTerm(word.Term)
	}
	for _, word := range pending {
		if word.Status == AutoImportStatusSynced {
			continue
		}
		appendTerm(word.Term)
	}

	slices.Sort(terms)
	return terms
}

func WriteDictionaryTermsFile(path string, terms []string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return os.ErrInvalid
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	var builder strings.Builder
	for _, term := range terms {
		trimmed := strings.TrimSpace(term)
		if trimmed == "" {
			continue
		}
		builder.WriteString(trimmed)
		builder.WriteByte('\n')
	}
	return os.WriteFile(path, []byte(builder.String()), 0o644)
}
