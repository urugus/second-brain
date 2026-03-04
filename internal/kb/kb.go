package kb

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/urugus/second-brain/internal/model"
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

// Read returns the full file content including any front matter.
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

// ReadBody returns only the markdown body, stripping any YAML front matter.
func (k *KB) ReadBody(relPath string) (string, error) {
	content, err := k.Read(relPath)
	if err != nil {
		return "", err
	}
	return StripFrontMatter(content), nil
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

func (k *KB) ExtractTitle(relPath string) (string, error) {
	content, err := k.Read(relPath)
	if err != nil {
		return "", err
	}

	// Strip front matter so we don't accidentally pick up a YAML key as a title.
	body := StripFrontMatter(content)

	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, "# ")), nil
		}
	}

	// Fallback: filename without extension
	base := filepath.Base(relPath)
	ext := filepath.Ext(base)
	return strings.TrimSuffix(base, ext), nil
}

// ReadMetadata parses YAML front matter from a KB file and returns the
// metadata. Returns a zero KBMetadata if no front matter is present.
func (k *KB) ReadMetadata(relPath string) (model.KBMetadata, error) {
	content, err := k.Read(relPath)
	if err != nil {
		return model.KBMetadata{}, err
	}
	meta, _ := ParseFrontMatter(content)
	return meta, nil
}

// WriteWithMetadata writes a KB file with YAML front matter metadata prepended.
func (k *KB) WriteWithMetadata(relPath string, body string, meta model.KBMetadata) error {
	content := MarshalFrontMatter(meta, body)
	return k.Write(relPath, content)
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
