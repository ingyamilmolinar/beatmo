package theme

import "testing"

func TestDocIsEmptyAndClone(t *testing.T) {
	var d Doc
	if !d.IsEmpty() {
		t.Fatal("zero Doc must be empty")
	}
	d = Doc{Categories: map[string]string{"accent": "orchid-300"}}
	if d.IsEmpty() {
		t.Fatal("doc with a category is not empty")
	}
	c := d.Clone()
	c.Categories["accent"] = "mint-100"
	if d.Categories["accent"] != "orchid-300" {
		t.Fatal("Clone must deep-copy Categories")
	}
	if !d.Equal(d.Clone()) || d.Equal(c) {
		t.Fatal("Equal must compare map contents")
	}
	e := Doc{Categories: map[string]string{}, Tokens: map[string]string{}}
	if !e.IsEmpty() {
		t.Fatal("doc with only empty maps is empty")
	}
	if !e.Equal(Doc{}) {
		t.Fatal("empty maps equal nil maps")
	}
}
