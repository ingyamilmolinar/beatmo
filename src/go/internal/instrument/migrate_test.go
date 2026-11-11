package instrument

import (
	"strings"
	"testing"
)

func TestMigrateAppliesStepsInOrder(t *testing.T) {
	decls := testDecls()
	decls[0].Version = 3
	decls[0].Steps = []Step{
		func(v Values) Values { v["core_g"] *= 10; return v },
		func(v Values) Values { v["core_g"]++; return v },
	}
	r, err := NewRegistry(decls, testFields, 2)
	if err != nil {
		t.Fatal(err)
	}
	d := minimalDoc() // core at v1, core_g = 1
	out, err := Migrate(d, r)
	if err != nil {
		t.Fatal(err)
	}
	if out.Stages["core"].V != 3 || out.Stages["core"].Values["core_g"] != 11 {
		t.Fatalf("migrated core = %+v", out.Stages["core"])
	}
	if d.Stages["core"].Values["core_g"] != 1 {
		t.Fatal("Migrate mutated its input")
	}
}

func TestMigrateRejectsNewerVersion(t *testing.T) {
	d := minimalDoc()
	s := d.Stages["core"]
	s.V = 5
	d.Stages["core"] = s
	_, err := Migrate(d, testRegistry(t))
	if err == nil || !strings.Contains(err.Error(), "newer than this build") {
		t.Fatalf("want a newer-version error, got %v", err)
	}
}
