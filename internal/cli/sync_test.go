package cli

import "testing"

// normalizeStorage strips server-side noise (random macro-id, version-at-save,
// whitespace re-serialization) so that a freshly rendered body can be compared
// for equality with what Confluence returns. These tests pin the behavior so
// future regressions don't silently turn every sync into a churn-update.
func TestNormalizeStorageMacroID(t *testing.T) {
	a := `<ac:structured-macro ac:name="code" ac:schema-version="1" ac:macro-id="111"><x/></ac:structured-macro>`
	b := `<ac:structured-macro ac:name="code" ac:schema-version="1" ac:macro-id="222"><x/></ac:structured-macro>`
	if normalizeStorage(a) != normalizeStorage(b) {
		t.Errorf("macro-id should be normalized away")
	}
}

func TestNormalizeStorageVersionAtSave(t *testing.T) {
	a := `<ri:attachment ri:filename="x.png" ri:version-at-save="3"/>`
	b := `<ri:attachment ri:filename="x.png"/>`
	if normalizeStorage(a) != normalizeStorage(b) {
		t.Errorf("ri:version-at-save should be normalized away")
	}
}

func TestNormalizeStorageWhitespaceBetweenTags(t *testing.T) {
	a := "<p>hi</p>\n<p>bye</p>"
	b := "<p>hi</p><p>bye</p>"
	if normalizeStorage(a) != normalizeStorage(b) {
		t.Errorf("whitespace between tags should be normalized away")
	}
}

func TestNormalizeStorageSelfCloseSpacing(t *testing.T) {
	a := `<hr />`
	b := `<hr/>`
	if normalizeStorage(a) != normalizeStorage(b) {
		t.Errorf("self-close spacing should be normalized away")
	}
}

func TestNormalizeStorageDoesNotCollapseTextWhitespace(t *testing.T) {
	// Whitespace inside text content (between two words, not between tags)
	// must NOT be collapsed - that would let two genuinely different bodies
	// hash to the same normalized form.
	got := normalizeStorage("<p>hello world</p>")
	want := "<p>hello world</p>"
	if got != want {
		t.Errorf("text whitespace got collapsed: %q", got)
	}
}
