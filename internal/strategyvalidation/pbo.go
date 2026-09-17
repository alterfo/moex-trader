package strategyvalidation

import "math"

type PBOResult struct {
	Trials  int
	Overfit int
	PBO     float64
}

func ProbabilityOfBacktestOverfitting(matrix [][]float64, s int) PBOResult {
	result := PBOResult{}
	if len(matrix) < s || s < 2 || s%2 != 0 {
		return result
	}
	strategies := len(matrix[0])
	if strategies < 2 {
		return result
	}
	for _, row := range matrix {
		if len(row) != strategies {
			return result
		}
	}
	blockStarts := make([]int, s+1)
	for block := 0; block < s; block++ {
		size := len(matrix) / s
		if block < len(matrix)%s {
			size++
		}
		blockStarts[block+1] = blockStarts[block] + size
	}
	combination := firstCombination(s, s/2)
	for {
		inSample := splitRows(matrix, blockStarts, combination, true)
		outOfSample := splitRows(matrix, blockStarts, combination, false)
		inSharpe := make([]float64, strategies)
		outSharpe := make([]float64, strategies)
		for column := 0; column < strategies; column++ {
			inSharpe[column] = sharpeAt(matrix, inSample, column)
			outSharpe[column] = sharpeAt(matrix, outOfSample, column)
		}
		winner := 0
		for column := 1; column < strategies; column++ {
			if inSharpe[column] > inSharpe[winner] {
				winner = column
			}
		}
		rank := 1
		for column := 0; column < strategies; column++ {
			if outSharpe[column] < outSharpe[winner] {
				rank++
			}
		}
		omega := float64(rank) / float64(strategies+1)
		logit := math.Log(omega / (1 - omega))
		if logit <= 0 {
			result.Overfit++
		}
		result.Trials++
		if !nextCombination(combination, s) {
			break
		}
	}
	if result.Trials > 0 {
		result.PBO = float64(result.Overfit) / float64(result.Trials)
	}
	return result
}

func sharpeAt(matrix [][]float64, rows []int, column int) float64 {
	values := make([]float64, 0, len(rows))
	for _, row := range rows {
		values = append(values, matrix[row][column])
	}
	return SharpeRatio(values)
}

func splitRows(matrix [][]float64, blockStarts []int, combination []int, inSample bool) []int {
	selected := make([]bool, len(blockStarts)-1)
	for _, block := range combination {
		selected[block] = true
	}
	var rows []int
	for block := 0; block < len(blockStarts)-1; block++ {
		if selected[block] != inSample {
			continue
		}
		for row := blockStarts[block]; row < blockStarts[block+1]; row++ {
			rows = append(rows, row)
		}
	}
	return rows
}

func firstCombination(n, k int) []int {
	result := make([]int, k)
	for i := 0; i < k; i++ {
		result[i] = i
	}
	return result
}

func nextCombination(combination []int, n int) bool {
	k := len(combination)
	for i := k - 1; i >= 0; i-- {
		if combination[i] < i+n-k {
			combination[i]++
			for j := i + 1; j < k; j++ {
				combination[j] = combination[j-1] + 1
			}
			return true
		}
	}
	return false
}
