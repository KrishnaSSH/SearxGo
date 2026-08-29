package server

import "testing"

func TestTitleMatchesQuery(t *testing.T) {
	yes := [][2]string{
		{"einstein", "Albert Einstein"},
		{"albert einstein facts", "Albert Einstein"},
		{"python programming language", "Python (programming language)"},
		{"Barack Obama", "Barack Obama"},
		{"golang", "Golang"},
	}
	no := [][2]string{
		{"how to tie a tie", "Necktie"},
		{"best pizza near me", "Pizza"},
		{"fsf", "Free Software Foundation"}, // too short, must be exact
		{"weather tomorrow", "Weather"},
		{"", "Something"},
	}
	for _, c := range yes {
		if !titleMatchesQuery(c[0], c[1]) {
			t.Errorf("titleMatchesQuery(%q, %q) = false, want true", c[0], c[1])
		}
	}
	for _, c := range no {
		if titleMatchesQuery(c[0], c[1]) {
			t.Errorf("titleMatchesQuery(%q, %q) = true, want false", c[0], c[1])
		}
	}
}
