//go:build !test && !js

package audio

// modularTableVoices adds each config-first table instrument's engine voice to
// m. Native-only because it bakes the seed through bakedModularRender (a CGo
// render path). Called from ResetInstruments so the instrument map is rebuilt
// from the table on every reset. On WASM the same instruments render through
// audio.js (RENDER/RENDER_INFO generated from the table); under -tags test the
// stub engine provides its own voices.
func modularTableVoices(m map[string]Instrument) {
	for _, d := range modularInstrumentDefs {
		m[d.ID] = CVariantInstrument{
			Name:   d.ID,
			Render: bakedModularRender(d.Seed),
			Beats:  d.Beats,
		}
	}
}
