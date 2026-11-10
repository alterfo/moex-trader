package docscheck

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const repoRoot = "../.."

func markdownFiles(t *testing.T) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(repoRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".claude", ".kilo", ".ralphex", ".revmux", "node_modules", "artifacts", "bin", "correspondence":
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".md") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk repo: %v", err)
	}
	return files
}

func isRussian(path string) bool {
	return strings.HasSuffix(path, ".ru.md")
}

func isPlan(path string) bool {
	return strings.Contains(filepath.ToSlash(path), "docs/plans/")
}

var coreDocs = map[string]bool{
	"AGENTS.md":                 true,
	"README.md":                 true,
	"docs/VISION.md":            true,
	"docs/ARCHITECTURE.md":      true,
	"docs/DEVELOPMENT.md":       true,
	"docs/GO_LIVE_CHECKLIST.md": true,
}

func TestEveryDocHasBothLanguages(t *testing.T) {
	for _, path := range markdownFiles(t) {
		if isRussian(path) {
			english := strings.TrimSuffix(path, ".ru.md") + ".md"
			if _, err := os.Stat(english); err != nil {
				t.Errorf("%s has no English original %s", path, english)
			}
			continue
		}
		rel, err := filepath.Rel(repoRoot, path)
		if err != nil || !coreDocs[filepath.ToSlash(rel)] {
			continue
		}
		russian := strings.TrimSuffix(path, ".md") + ".ru.md"
		if _, err := os.Stat(russian); err != nil {
			t.Errorf("%s has no Russian translation %s", path, russian)
		}
	}
}

var (
	fencedBlock = regexp.MustCompile("(?s)```.*?```")
	inlineCode  = regexp.MustCompile("`[^`\\n]*`")
)

func TestEnglishDocsContainNoCyrillic(t *testing.T) {
	cyrillic := regexp.MustCompile(`[\x{0400}-\x{04FF}]`)
	for _, path := range markdownFiles(t) {
		if isRussian(path) {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		prose := inlineCode.ReplaceAll(fencedBlock.ReplaceAll(data, nil), nil)
		if cyrillic.Match(prose) {
			t.Errorf("%s is canonical English but contains Cyrillic text", path)
		}
	}
}

func TestArchitectureCoversEveryPackage(t *testing.T) {
	var packages []string
	for _, group := range []string{"cmd", "internal"} {
		entries, err := os.ReadDir(filepath.Join(repoRoot, group))
		if err != nil {
			t.Fatalf("read %s: %v", group, err)
		}
		for _, entry := range entries {
			if entry.IsDir() {
				packages = append(packages, group+"/"+entry.Name())
			}
		}
	}
	for _, doc := range []string{"ARCHITECTURE.md", "ARCHITECTURE.ru.md"} {
		data, err := os.ReadFile(filepath.Join(repoRoot, "docs", doc))
		if err != nil {
			t.Fatalf("read %s: %v", doc, err)
		}
		text := string(data)
		for _, pkg := range packages {
			if !strings.Contains(text, "`"+pkg+"`") && !strings.Contains(text, "`"+pkg+"/") && !strings.Contains(text, "`"+pkg+",") {
				t.Errorf("%s does not mention package %s", doc, pkg)
			}
		}
	}
}

var linkPattern = regexp.MustCompile(`\]\(([^)#\s]+)(#[^)]*)?\)`)

func TestRelativeLinksResolve(t *testing.T) {
	for _, path := range markdownFiles(t) {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, match := range linkPattern.FindAllStringSubmatch(string(data), -1) {
			target := match[1]
			if strings.Contains(target, "://") || strings.HasPrefix(target, "mailto:") {
				continue
			}
			if _, err := os.Stat(filepath.Join(filepath.Dir(path), target)); err != nil {
				t.Errorf("%s links to missing %s", path, target)
			}
		}
	}
}
