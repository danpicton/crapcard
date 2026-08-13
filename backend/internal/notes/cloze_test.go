package notes

import (
	"reflect"
	"testing"
)

func TestClozeNumbers(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []int
	}{
		{"no markers", "plain text", nil},
		{"single", "The capital is {{c1::Ottawa}}.", []int{1}},
		{"distinct sorted", "{{c3::a}} then {{c1::b}} and {{c3::c}}", []int{1, 3}},
		{"with hint", "{{c2::Ottawa::capital}}", []int{2}},
		{"zero is not a deletion", "{{c0::x}}", nil},
		{"malformed ignored", "{{c::x}} {{1::x}} {{cx::y}}", nil},
		{"unterminated ignored", "{{c1::never closed", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := clozeNumbers(tt.text); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("clozeNumbers(%q) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}
}

func TestRenderClozeText(t *testing.T) {
	tests := []struct {
		name         string
		text         string
		test         int
		wantQuestion string
		wantAnswer   string
	}{
		{
			name:         "tested deletion becomes a blank",
			text:         "The capital of Canada is {{c1::Ottawa}}.",
			test:         1,
			wantQuestion: "The capital of Canada is [...].",
			wantAnswer:   "The capital of Canada is **Ottawa**.",
		},
		{
			name:         "hint shows in the blank",
			text:         "The capital is {{c1::Ottawa::city name}}.",
			test:         1,
			wantQuestion: "The capital is [city name].",
			wantAnswer:   "The capital is **Ottawa**.",
		},
		{
			name:         "other numbers are revealed on the question",
			text:         "{{c1::Ottawa}} is in {{c2::Canada}}.",
			test:         2,
			wantQuestion: "Ottawa is in [...].",
			wantAnswer:   "Ottawa is in **Canada**.",
		},
		{
			name:         "same number blanks every occurrence",
			text:         "{{c1::a}} and {{c1::b}}",
			test:         1,
			wantQuestion: "[...] and [...]",
			wantAnswer:   "**a** and **b**",
		},
		{
			name:         "text without markers passes through",
			text:         "no markers here",
			test:         1,
			wantQuestion: "no markers here",
			wantAnswer:   "no markers here",
		},
		{
			name:         "hint on an untested deletion is dropped",
			text:         "{{c1::Ottawa::city}} is in {{c2::Canada}}.",
			test:         2,
			wantQuestion: "Ottawa is in [...].",
			wantAnswer:   "Ottawa is in **Canada**.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := renderClozeText(tt.text, tt.test)
			if got.Question != tt.wantQuestion {
				t.Errorf("question = %q, want %q", got.Question, tt.wantQuestion)
			}
			if got.Answer != tt.wantAnswer {
				t.Errorf("answer = %q, want %q", got.Answer, tt.wantAnswer)
			}
		})
	}
}
