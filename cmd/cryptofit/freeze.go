package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type freezeEntry struct {
	path string
	hash string
	size int64
}

func freezePanel(o options) error {
	var entries []freezeEntry
	for _, dir := range []string{"klines1d", "funding", "klines5m"} {
		root := filepath.Join(o.dataDir, dir)
		names, err := os.ReadDir(root)
		if err != nil {
			return err
		}
		files := make([]string, 0, len(names))
		for _, n := range names {
			if n.IsDir() || !strings.HasSuffix(n.Name(), ".csv") {
				continue
			}
			files = append(files, n.Name())
		}
		sort.Strings(files)
		for _, name := range files {
			path := filepath.Join(root, name)
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			sum := sha256.Sum256(data)
			entries = append(entries, freezeEntry{
				path: filepath.Join(dir, name),
				hash: hex.EncodeToString(sum[:]),
				size: int64(len(data)),
			})
		}
	}
	chain := sha256.New()
	var total int64
	for _, e := range entries {
		total += e.size
		fmt.Fprintf(chain, "%s %s %d\n", e.path, e.hash, e.size)
	}
	hash := hex.EncodeToString(chain.Sum(nil))
	out := o.out
	if out == "" {
		out = filepath.Join("artifacts", "crypto", "panel-freeze.json")
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	var sb strings.Builder
	sb.WriteString("{\n")
	fmt.Fprintf(&sb, "  \"files\": %d,\n", len(entries))
	fmt.Fprintf(&sb, "  \"bytes\": %d,\n", total)
	fmt.Fprintf(&sb, "  \"chain_sha256\": \"%s\",\n", hash)
	sb.WriteString("  \"entries\": [\n")
	for i, e := range entries {
		sep := ","
		if i == len(entries)-1 {
			sep = ""
		}
		fmt.Fprintf(&sb, "    {\"path\": %q, \"sha256\": %q, \"bytes\": %d}%s\n", e.path, e.hash, e.size, sep)
	}
	sb.WriteString("  ]\n}\n")
	if err := os.WriteFile(out, []byte(sb.String()), 0o644); err != nil {
		return err
	}
	fmt.Printf("files %d, bytes %d\nchain sha256 %s\nmanifest %s\n", len(entries), total, hash, out)
	return nil
}
