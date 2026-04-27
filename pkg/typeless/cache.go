package typeless

import (
	"os"
	"path/filepath"
)

func DefaultCachePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".typelens", "cache.json"), nil
}
