package matcher

import "testing"

func TestCompileAndMatch(t *testing.T) {
	cases := []struct {
		pattern string
		value   string
		want    bool
	}{
		{"", "anything", true},
		{"invoice", "Your INVOICE is ready", true},
		{"invoice", "your receipt", false},
		{"case:invoice", "Your INVOICE", false},
		{"case:invoice", "your invoice", true},
		{"*verify*code*", "Please verify your login code", true},
		{"*verify*code*", "Please verify your login", false},
		{"re:^\\d{6}$", "123456", true},
		{"re:^\\d{6}$", "12345a", false},
		{"re:^INVOICE-\\d+$", "invoice-42", true},
		{"*", "whatever", true},
	}
	for _, c := range cases {
		p, err := Compile(c.pattern)
		if err != nil {
			t.Fatalf("compile %q: %v", c.pattern, err)
		}
		if got := p.Match(c.value); got != c.want {
			t.Fatalf("pattern %q value %q: got %v want %v", c.pattern, c.value, got, c.want)
		}
	}
}

func TestCompileErrors(t *testing.T) {
	if _, err := Compile("re:("); err == nil {
		t.Fatal("expected error for invalid regexp")
	}
	if _, err := Compile("re:"); err == nil {
		t.Fatal("expected error for empty regexp")
	}
}

func TestRuleRequiresBoth(t *testing.T) {
	r, err := CompileRule("invoice", "@bank.com")
	if err != nil {
		t.Fatal(err)
	}
	if !r.Match("New invoice", "noreply@bank.com") {
		t.Fatal("expected match")
	}
	if r.Match("New invoice", "noreply@other.com") {
		t.Fatal("sender mismatch should not match")
	}
	if r.Match("Receipt", "noreply@bank.com") {
		t.Fatal("subject mismatch should not match")
	}
	empty, _ := CompileRule("", "")
	if !empty.Empty() || !empty.Match("x", "y") {
		t.Fatal("empty rule should match everything")
	}
}
