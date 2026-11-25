package token

import "testing"

func TestLookupIsCaseInsensitive(t *testing.T) {
	for _, s := range []string{"PRINT", "print", "Print"} {
		if got := Lookup(s); got != PRINT {
			t.Errorf("Lookup(%q) = %v, want PRINT", s, got)
		}
	}
	if got := Lookup("customerId"); got != Ident {
		t.Errorf("Lookup(customerId) = %v, want NAME", got)
	}
}

func TestEveryKeywordHasAName(t *testing.T) {
	for k := keywordStart + 1; k < keywordEnd; k++ {
		if _, ok := names[k]; !ok {
			t.Errorf("keyword Kind(%d) has no entry in names", int(k))
		}
	}
}
