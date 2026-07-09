//go:build test || js

package audio

// registerClonedVoice (test/js) makes newID available by aliasing the source.
// Under -tags test the stub engine renders no real audio; on WASM the actual
// render happens on the JS side (RENDER/RENDER_INFO) and the cloned config
// reaches it through the recipe-defaults push in CloneInstrument. Baking a CGo
// voice (bakedModularRender) is native-only, so it does not apply here.
func registerClonedVoice(newID string, seed RecipeParams, srcID string) {
	registerInstanceAlias(newID, srcID)
}
