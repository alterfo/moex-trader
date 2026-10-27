package main

import "strings"

var positiveStems = []string{"рост", "вырос", "выросл", "прибыл", "рекорд", "дивиденд", "повыш", "увелич", "выкуп", "контракт", "одобр", "расшир", "улучш", "покупк", "сильн"}
var negativeStems = []string{"пад", "снизил", "снижен", "убыт", "санкц", "штраф", "иск ", "отмен", "дефолт", "банкрот", "расследован", "приостанов", "сократ", "понижен", "риск", "авария", "обвал", "потер"}

func lexScore(title string) float64 {
	t := strings.ToLower(title)
	pos, neg := 0, 0
	for _, s := range positiveStems {
		if strings.Contains(t, s) {
			pos++
		}
	}
	for _, s := range negativeStems {
		if strings.Contains(t, s) {
			neg++
		}
	}
	return float64(pos - neg)
}
