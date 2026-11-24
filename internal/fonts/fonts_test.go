package fonts

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestRegisterFonts(t *testing.T) {
	Register()

	foundLatin := false
	foundCJK := false
	for _, fam := range ui.FontFamilies() {
		if fam.Name == LatinFamily && fam.Registered {
			foundLatin = true
		}
		if fam.Name == CJKFamily && fam.Registered {
			foundCJK = true
		}
	}
	if !foundLatin {
		t.Errorf("expected registered font family %q in ui.FontFamilies()", LatinFamily)
	}
	if !foundCJK {
		t.Errorf("expected registered font family %q in ui.FontFamilies()", CJKFamily)
	}
}
