package strategyvalidation

import (
	"math"
	"testing"
)

func TestPBODominantStrategyHasLowPBO(t *testing.T) {
	matrix := dominantMatrix(30, 5)
	result := ProbabilityOfBacktestOverfitting(matrix, 10)
	if result.Trials != 252 {
		t.Fatalf("Trials = %d, want 252", result.Trials)
	}
	if result.PBO >= 0.2 {
		t.Fatalf("dominant strategy PBO = %.4f, want < 0.2", result.PBO)
	}
}

func TestPBOSplitMixNoiseIsIndeterminate(t *testing.T) {
	matrix := splitMixNoiseMatrix(40, 10)
	result := ProbabilityOfBacktestOverfitting(matrix, 10)
	if result.Trials != 252 {
		t.Fatalf("Trials = %d, want 252", result.Trials)
	}
	if result.PBO < 0.3 || result.PBO > 0.7 {
		t.Fatalf("split-mix noise PBO = %.4f, want 0.3..0.7", result.PBO)
	}
}

func TestPBODegenerateInputsReturnZero(t *testing.T) {
	cases := map[string]struct {
		matrix [][]float64
		s      int
	}{
		"empty":           {matrix: [][]float64{}, s: 4},
		"too_few_rows":    {matrix: [][]float64{{0.01, 0.02}, {0.01, 0.02}}, s: 4},
		"single_strategy": {matrix: [][]float64{{0.01}, {0.02}, {0.01}, {0.02}}, s: 2},
		"odd_s":           {matrix: [][]float64{{0.01, 0.02}, {0.02, 0.01}, {0.01, 0.02}, {0.02, 0.01}}, s: 3},
		"ragged_rows":     {matrix: [][]float64{{0.01, 0.02}, {0.02}, {0.01, 0.02}, {0.02, 0.01}}, s: 2},
	}
	for name, tc := range cases {
		if got := ProbabilityOfBacktestOverfitting(tc.matrix, tc.s); got.PBO != 0 || got.Trials != 0 {
			t.Fatalf("%s: PBO = %+v, want zero result", name, got)
		}
	}
}

func TestPBOCombinationCountForSmallBlockCount(t *testing.T) {
	result := ProbabilityOfBacktestOverfitting(dominantMatrix(12, 2), 6)
	if result.Trials != 20 {
		t.Fatalf("Trials = %d, want 20", result.Trials)
	}
}

func TestDefaultSplits(t *testing.T) {
	cases := []struct {
		periods int
		want    int
	}{
		{0, 0},
		{1, 0},
		{2, 2},
		{3, 2},
		{4, 4},
		{15, 14},
		{16, 16},
		{40, 16},
	}
	for _, tc := range cases {
		if got := DefaultSplits(tc.periods); got != tc.want {
			t.Fatalf("DefaultSplits(%d) = %d, want %d", tc.periods, got, tc.want)
		}
	}
}

func dominantMatrix(rows, strategies int) [][]float64 {
	matrix := make([][]float64, rows)
	for row := 0; row < rows; row++ {
		matrix[row] = make([]float64, strategies)
		matrix[row][0] = 0.02 + 0.01*math.Sin(float64(row)*0.7)
		for strategy := 1; strategy < strategies; strategy++ {
			matrix[row][strategy] = 0.001 * math.Sin(float64(row)*3+float64(strategy)*1.7)
		}
	}
	return matrix
}

func splitMixNoiseMatrix(rows, strategies int) [][]float64 {
	seed := uint64(0x9e3779b97f4a7c15)
	matrix := make([][]float64, rows)
	for row := 0; row < rows; row++ {
		matrix[row] = make([]float64, strategies)
		for strategy := 0; strategy < strategies; strategy++ {
			matrix[row][strategy] = splitMix64(&seed)
		}
	}
	return matrix
}

func splitMix64(seed *uint64) float64 {
	*seed += 0x9e3779b97f4a7c15
	z := *seed
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	z = z ^ (z >> 31)
	normalized := float64(z) / float64(math.MaxUint64)
	return normalized*0.02 - 0.01
}
