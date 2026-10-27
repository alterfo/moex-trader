package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

const playbookFeatureSHA = "299a7f9b73751d5c8ecbeaab1eba4d598351fb0a14bab65662a46323adab0c70"

func fileSHA(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func requirePlaybookFile(path string) {
	if path == "" {
		fatal(fmt.Errorf("this mode needs -candfile pointing at the playbook feature file"))
	}
	got, err := fileSHA(path)
	if err != nil {
		fatal(err)
	}
	if got != playbookFeatureSHA {
		fatal(fmt.Errorf("candidate file SHA-256 %s differs from the playbook feature file %s: the signal is not the lead's signal", got, playbookFeatureSHA))
	}
}

func availNames(t *tfmCtx, kind string, a, b int) (int, int, int) {
	var counts []int
	for d := a; d <= b; d++ {
		n := 0
		for _, v := range sleeveSignal(t, d, t.ds.byDate[d], kind) {
			if !isNaNf(v) {
				n++
			}
		}
		counts = append(counts, n)
	}
	if len(counts) == 0 {
		return 0, 0, 0
	}
	sorted := append([]int{}, counts...)
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && sorted[j] < sorted[j-1]; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}
	return sorted[0], sorted[len(sorted)/2], sorted[len(sorted)-1]
}

func isNaNf(v float64) bool { return v != v }
