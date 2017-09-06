package main

import ("testing")

func TestValidCommitSubjectMessage(t *testing.T) {
	type testcase struct {
		Message string
		MaxLen int
		MinLen int
		Expected bool
	}

	tests := []testcase{
		{"Hello World", 72, 3, true},
		{"Hello World", 5, 3, false},
		{"Hello", 72, 10, false},
	}

	for i, test := range tests {
		if commitMessageSubjectIsValid(test.Message, test.MaxLen, test.MinLen) != test.Expected {
			t.Errorf("Case %d failed", i)
		}
	}
}

func TestValidCommitBodyMessage(t *testing.T) {
	type testcase struct {
		Message string
		MaxLen int
		Expected bool
	}

	tests := []testcase{
		{"Subjects can be long sometimes, even longer than 20 chars\n\nMany\nShort\nLines", 20, true},
		{"Subject\n\nMany\nShort\nLines", 3, false},
	}

	for i, test := range tests {
		if commitMessageBodyIsValid(test.Message, test.MaxLen) != test.Expected {
			t.Errorf("Case %d failed", i)
		}
	}
}

func TestCommitMessageRegex(t *testing.T) {
	type testcase struct {
		Message string
		Regex string
		Expected bool
	}

	tests := []testcase{
		{"fea(abc) hello", "^(fea|fix|doc)\\([a-z0-9\\-]{2,30}\\)", true},
		{"hee(abc) hello", "^(fea|fix|doc)\\([a-z0-9\\-]{2,30}\\)", false},
	}

	for i, test := range tests {
		if commitMessageMatchesRegex(test.Message, test.Regex) != test.Expected {
			t.Errorf("Case %d failed", i)
		}
	}
}