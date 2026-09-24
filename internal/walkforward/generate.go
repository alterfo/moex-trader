package walkforward

import (
	"fmt"
	"time"
)

type WindowSpec struct {
	From  time.Time
	Split time.Time
	Till  time.Time
}

func GenerateWindowSpecs(from, till time.Time, trainDays, testDays, stepDays int, anchored bool) ([]WindowSpec, error) {
	if trainDays <= 0 {
		return nil, fmt.Errorf("walk-forward: train-days must be positive")
	}
	if testDays <= 0 {
		return nil, fmt.Errorf("walk-forward: test-days must be positive")
	}
	if stepDays <= 0 {
		return nil, fmt.Errorf("walk-forward: step-days must be positive")
	}
	if !from.Before(till) {
		return nil, fmt.Errorf("walk-forward: from must be before till")
	}

	var specs []WindowSpec
	split := from.AddDate(0, 0, trainDays)
	for {
		testTill := split.AddDate(0, 0, testDays)
		if testTill.After(till) {
			break
		}
		windowFrom := from
		if !anchored {
			windowFrom = split.AddDate(0, 0, -trainDays)
		}
		specs = append(specs, WindowSpec{From: windowFrom, Split: split, Till: testTill})
		split = split.AddDate(0, 0, stepDays)
	}
	return specs, nil
}
