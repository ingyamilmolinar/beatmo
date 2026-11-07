//go:build test

package ui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// Everything else opened through the portal must carry the shared frame.
var menuCardExempt = map[string]string{
	"compPortalOverlay":             "adapter — the wrapped component is checked by name below",
	"dvOverlayPortal":               "adapter — the wrapped DrumView menu is checked by name below",
	"TooltipOverlay":                "tooltip (display-only, Passive)",
	"sliderPopupPortalOverlay":      "value popup: a single slider, no title band by design",
	"mobileWheelPopupPortalOverlay": "value-wheel popup (must-answer editor, no title band)",
	"RenameComponent":               "inline text-commit dialog (rename / naming); the field IS the surface",
	"SubdivMenuComponent":           "dropdown option list anchored to its control (a select, not a menu)",
	"actionMenuComponent":           "dropdown option list anchored to its control (a select, not a menu)",
	"InstrumentMenuComponent":       "breadcrumb strip is its title band (same band/×/hairline primitives); a plain title would lose category navigation",
}

var dvMenuCards = map[string]string{
	"overflow-menu": "overflowCard",
	"context-menu":  "contextMenuCard",
	"fx-panel":      "fxPanelCard",
}

// Every overlay type opened through portal.Open holds a *MenuCard or is exempt with a reason.
func TestEveryPortalMenuUsesMenuCard(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	fset := token.NewFileSet()
	types := map[string]*ast.StructType{}
	var opened []string // overlay type names found at PortalEntry sites
	var dvIDs []string
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		file, err := parser.ParseFile(fset, f, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			switch v := n.(type) {
			case *ast.TypeSpec:
				if st, ok := v.Type.(*ast.StructType); ok {
					types[v.Name.Name] = st
				}
			case *ast.CompositeLit:
				id, ok := v.Type.(*ast.Ident)
				if !ok || id.Name != "PortalEntry" {
					return true
				}
				for _, el := range v.Elts {
					kv, ok := el.(*ast.KeyValueExpr)
					if !ok {
						continue
					}
					k, _ := kv.Key.(*ast.Ident)
					if k == nil {
						continue
					}
					if k.Name == "Overlay" {
						opened = append(opened, overlayTypeName(kv.Value))
					}
				}
			case *ast.CallExpr:
				// menuCardEntry(id, overlay, …) sites.
				if fn, ok := v.Fun.(*ast.Ident); ok && fn.Name == "menuCardEntry" && len(v.Args) >= 2 {
					opened = append(opened, overlayTypeName(v.Args[1]))
				}
			}
			return true
		})
		// dvOverlayPortal{id: "…"} literals name DrumView menus by id.
		for _, m := range strings.Split(string(src), "dvOverlayPortal{")[1:] {
			if i := strings.Index(m, "id:"); i >= 0 && i < 80 {
				rest := m[i+3:]
				if q := strings.Index(rest, "\""); q >= 0 {
					end := strings.Index(rest[q+1:], "\"")
					dvIDs = append(dvIDs, rest[q+1:q+1+end])
				}
			}
		}
	}
	hasCard := func(typ string) bool {
		st, ok := types[typ]
		if !ok {
			return false
		}
		for _, fld := range st.Fields.List {
			if ft, ok := fld.Type.(*ast.StarExpr); ok {
				if id, ok := ft.X.(*ast.Ident); ok && id.Name == "MenuCard" {
					return true
				}
			}
			if id, ok := fld.Type.(*ast.Ident); ok && id.Name == "MenuCard" {
				return true
			}
		}
		return false
	}
	seen := map[string]bool{}
	var bad []string
	for _, typ := range opened {
		if typ == "" || seen[typ] {
			continue
		}
		seen[typ] = true
		if _, ok := menuCardExempt[typ]; ok {
			continue
		}
		if !hasCard(typ) {
			bad = append(bad, typ)
		}
	}
	dv := types["DrumView"]
	for _, id := range dvIDs {
		field, ok := dvMenuCards[id]
		if !ok {
			continue // ids without a card field are checked by their component type above
		}
		found := false
		for _, fld := range dv.Fields.List {
			for _, n := range fld.Names {
				if n.Name == field {
					found = true
				}
			}
		}
		if !found {
			bad = append(bad, "DrumView."+field+" (menu id "+id+")")
		}
	}
	sort.Strings(bad)
	if len(bad) > 0 {
		t.Fatalf("portal menus without a MenuCard frame (embed *MenuCard or add to menuCardExempt with a reason):\n  %s", strings.Join(bad, "\n  "))
	}
	if len(seen) < 6 {
		t.Fatalf("scanner found only %d overlay types; parser regression?", len(seen))
	}
}

// &T{…}, T{…}, NewT(…), compPortalOverlay{comp: dv.X} → X's type; a variable gives "".
func overlayTypeName(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.UnaryExpr:
		return overlayTypeName(v.X)
	case *ast.CompositeLit:
		if id, ok := v.Type.(*ast.Ident); ok {
			if id.Name == "compPortalOverlay" {
				for _, el := range v.Elts {
					if kv, ok := el.(*ast.KeyValueExpr); ok {
						if k, _ := kv.Key.(*ast.Ident); k != nil && k.Name == "comp" {
							return compTypeName(kv.Value)
						}
					}
				}
			}
			return id.Name
		}
	case *ast.CallExpr:
		if fn, ok := v.Fun.(*ast.Ident); ok && strings.HasPrefix(fn.Name, "New") {
			return strings.TrimPrefix(fn.Name, "New")
		}
	case *ast.Ident:
		return ""
	}
	return ""
}

// The known DrumView component fields are mapped explicitly.
func compTypeName(e ast.Expr) string {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok {
		if id, ok := e.(*ast.Ident); ok {
			return compVarTypes[id.Name]
		}
		return ""
	}
	return compVarTypes[sel.Sel.Name]
}

// Explicit, so the scan stays simple.
var compVarTypes = map[string]string{
	"colorWheelComp": "ColorWheelComponent",
	"instMenuComp":   "InstrumentMenuComponent",
	"subdivMenuComp": "SubdivMenuComponent",
	"navMenuComp":    "NavigationMenuComponent",
	"renameComp":     "RenameComponent",
	"namingComp":     "RenameComponent",
	"c":              "LogsComponent",
	"m":              "actionMenuComponent",
	"noteMatrixComp": "NoteMatrixComponent",
	"jumpMenuComp":   "SubdivMenuComponent",
	"volumePopup":    "SliderPopup",
	"masterVolPopup": "SliderPopup",
	"wheelPopup":     "wheelPopupOverlay",
	"samplerWheel":   "wheelPopupOverlay",
	"eqWheel":        "wheelPopupOverlay",
	"datePicker":     "logsDatePicker",
	"dv.datePicker":  "logsDatePicker",
}
