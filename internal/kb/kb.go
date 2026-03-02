package kb

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type KB struct {
	rootDir string
}

type SearchResult struct {
	Path    string
	Line    int
	Content string
}

func New(rootDir string) *KB {
	return &KB{rootDir: rootDir}
}

func (k *KB) List() ([]string, error) {
	var files []string
	err := filepath.Walk(k.rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if strings.HasSuffix(info.Name(), ".md") {
			rel, err := filepath.Rel(k.rootDir, path)
			if err != nil {
				return err
			}
			files = append(files, rel)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk knowledge base: %w", err)
	}
	return files, nil
}

func (k *KB) Read(relPath string) (string, error) {
	absPath, err := k.checkPath(relPath)
	if err != nil {
		return "", err
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}
	return string(data), nil
}

func (k *KB) checkPath(relPath string) (string, error) {
	absPath := filepath.Join(k.rootDir, relPath)

	absPath, err := filepath.Abs(absPath)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}
	rootAbs, err := filepath.Abs(k.rootDir)
	if err != nil {
		return "", fmt.Errorf("resolve root: %w", err)
	}
	if !strings.HasPrefix(absPath, rootAbs+string(filepath.Separator)) && absPath != rootAbs {
		return "", fmt.Errorf("path %q is outside knowledge base", relPath)
	}
	return absPath, nil
}

func (k *KB) Write(relPath string, content string) error {
	absPath, err := k.checkPath(relPath)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}
	return os.WriteFile(absPath, []byte(content), 0o644)
}

func (k *KB) Exists(relPath string) bool {
	absPath, err := k.checkPath(relPath)
	if err != nil {
		return false
	}
	_, err = os.Stat(absPath)
	return err == nil
}

func (k *KB) Search(query string) ([]SearchResult, error) {
	query = strings.ToLower(query)
	var results []SearchResult

	files, err := k.List()
	if err != nil {
		return nil, err
	}

	for _, relPath := range files {
		absPath := filepath.Join(k.rootDir, relPath)
		f, err := os.Open(absPath)
		if err != nil {
			continue
		}

		scanner := bufio.NewScanner(f)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := scanner.Text()
			if strings.Contains(strings.ToLower(line), query) {
				results = append(results, SearchResult{
					Path:    relPath,
					Line:    lineNum,
					Content: line,
				})
			}
		}
		f.Close()
	}

	return results, nil
}
