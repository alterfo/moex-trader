package main

import "testing"

func TestParseAnswer(t *testing.T) {
	cases := []struct {
		in    string
		score float64
		ok    bool
	}{
		{`{"direction":"up","confidence":4}`, 4, true},
		{`{"direction":"DOWN","confidence":9}`, -5, true},
		{`{"direction":"neutral","confidence":3}`, 0, true},
		{`{"direction":"sideways","confidence":3}`, 0, false},
		{`not json`, 0, false},
	}
	for _, c := range cases {
		got, _, err := parseAnswer(c.in)
		if (err == nil) != c.ok || (c.ok && got != c.score) {
			t.Fatalf("%q: got %v err %v", c.in, got, err)
		}
	}
}
