package audio

import "testing"

func TestPrettyName(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"hi-hat", "Hi Hat"},
		{"open_hi_hat", "Open Hi Hat"},
		{"kick", "Kick"},
		{"kick-drum", "Kick Drum"},
		{"", ""},
		{"a-b-c", "A B C"},
		{"ALREADY Pretty", "Already Pretty"},
		{"  spaced  out  ", "Spaced Out"},
		{"mix_of-separators", "Mix Of Separators"},
		{"x", "X"},
	}
	for _, tc := range cases {
		got := PrettyName(tc.in)
		if got != tc.want {
			t.Errorf("PrettyName(%q) = %q; want %q", tc.in, got, tc.want)
		}
	}
}
