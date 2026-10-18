package model

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

type Driver struct {
	Feature string
	Delta   float64
}

func (m *EnsembleModel) memberProbabilityFor(member EnsembleMember, vector []float64) float64 {
	lgb, xgb, logistic := m.MemberProbabilities(vector)
	switch member {
	case EnsembleMemberLGB:
		return lgb
	case EnsembleMemberXGB:
		return xgb
	case EnsembleMemberLogistic:
		return logistic
	default:
		return (lgb + xgb + logistic) / 3.0
	}
}

func (m *EnsembleModel) Drivers(member EnsembleMember, vector []float64, top int) []Driver {
	if top <= 0 || len(vector) != len(m.FeatureOrder) || len(vector) != len(m.Logistic.Mean) {
		return nil
	}
	base := m.memberProbabilityFor(member, vector)
	probe := make([]float64, len(vector))
	drivers := make([]Driver, 0, len(vector))
	for i, name := range m.FeatureOrder {
		if _, untrained := untrainedFeatures[name]; untrained {
			continue
		}
		copy(probe, vector)
		probe[i] = m.Logistic.Mean[i]
		delta := base - m.memberProbabilityFor(member, probe)
		if delta == 0 || math.IsNaN(delta) {
			continue
		}
		drivers = append(drivers, Driver{Feature: name, Delta: delta})
	}
	sort.SliceStable(drivers, func(a, b int) bool {
		return math.Abs(drivers[a].Delta) > math.Abs(drivers[b].Delta)
	})
	if len(drivers) > top {
		drivers = drivers[:top]
	}
	return drivers
}

func (m *EnsembleModel) LogisticContributions(vector []float64) []float64 {
	out := make([]float64, len(vector))
	for i := range vector {
		std := m.Logistic.Std[i]
		if std == 0 {
			std = 1
		}
		out[i] = m.Logistic.Coef[i] * (vector[i] - m.Logistic.Mean[i]) / std
	}
	return out
}

func FormatDrivers(drivers []Driver) string {
	if len(drivers) == 0 {
		return ""
	}
	parts := make([]string, len(drivers))
	for i, d := range drivers {
		parts[i] = fmt.Sprintf("%s:%+.1f", d.Feature, d.Delta*100)
	}
	return " drivers=" + strings.Join(parts, ",")
}
