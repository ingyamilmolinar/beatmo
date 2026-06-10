package audio

import (
	"errors"
	"fmt"
	"math"
)

// SynthRecipeProvider is the canonical plugin extension point. Any code
// (built-in registration, runtime plugin loader, test fixture) can satisfy
// this by implementing the Render method — the same shape the SynthRecipe
// interface advertises. Built-in recipes wrap C renderers; future
// out-of-process or .wasm-loaded plugins will wrap a sandboxed equivalent.
//
// Phase 13 of the live-instrument synthesis remediation plan introduces
// this seam so the registry stops being a builtin-only-via-CGo island. The
// existing RegisterRecipe is already pluggable (its New() closure accepts
// any SynthRecipe), but there was no convenience for the most common case
// (a pure-Go plugin with a single render function) and no test that the
// runtime registration path actually works end-to-end. RegisterPluginRecipe
// fills the convenience gap; TestPluginRecipeRoundtrip locks the contract.
type SynthRecipeProvider interface {
	Render(buf []float32, sampleRate, samples, variant int, p RecipeParams)
}

// PluginRecipeOptions describes a runtime-registered recipe. The Provider
// supplies the audio synthesis; ID/DisplayName/Category/ParamDefs become
// the registry metadata the UI uses to render the editor.
type PluginRecipeOptions struct {
	ID          string
	DisplayName string
	Category    string
	ParamDefs   []ParamDef
	Provider    SynthRecipeProvider
}

// ErrInvalidPluginRecipe is returned when a registration is missing one of
// the required fields. Sentinel-error so callers can distinguish "bad
// plugin definition" from "registry rejected the recipe".
var ErrInvalidPluginRecipe = errors.New("audio: invalid plugin recipe")

// RegisterPluginRecipe installs a runtime-defined SynthRecipe into the
// global registry. Once registered the recipe is reachable via NewRecipe(id)
// and BindInstrumentToRecipe(instrumentID, id) just like a builtin. The
// JSON shape (ParamDef) is the same v1 freezing surface that the JSON
// import/export round-trips through.
//
// Concurrency: safe to call from any goroutine. The underlying RegisterRecipe
// takes the registry write lock; subsequent reads see the new entry without
// further coordination. Existing per-instrument param maps and voice caches
// keyed on this recipe id are NOT invalidated — callers that re-register an
// existing id should also call globalVoiceCache.ClearInstrument for each
// instrument bound to it.
func RegisterPluginRecipe(opts PluginRecipeOptions) error {
	if opts.ID == "" {
		return fmt.Errorf("%w: ID is required", ErrInvalidPluginRecipe)
	}
	if opts.Provider == nil {
		return fmt.Errorf("%w: Provider is required", ErrInvalidPluginRecipe)
	}
	for _, d := range opts.ParamDefs {
		if err := validateParamDef(d); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidPluginRecipe, err)
		}
	}
	provider := opts.Provider
	display := opts.DisplayName
	if display == "" {
		display = opts.ID
	}
	category := opts.Category
	if category == "" {
		category = "plugin"
	}
	params := append([]ParamDef(nil), opts.ParamDefs...)
	RegisterRecipe(RecipeRegistration{
		ID:          opts.ID,
		DisplayName: display,
		Category:    category,
		Params:      params,
		New: func() SynthRecipe {
			return &pluginRecipe{
				id:          opts.ID,
				displayName: display,
				category:    category,
				params:      params,
				provider:    provider,
			}
		},
	})
	return nil
}

// RegisterPluginRecipeFromDoc is the canonical entry point for registering
// a RecipeDoc-shaped recipe at runtime. Used by Phase 3's override loader
// for user-saved recipes (read out of userprefs at startup) and by the
// Save-As flow that clones an existing recipe into a new user-scoped id.
//
// Builtin recipes do NOT go through this path — they register via
// registerBuiltinRecipes/init() so the platform-specific renderer map
// (builtinRecipeRenderers) is the source for their renderer. This function
// is for runtime/declarative recipes whose audio is produced by a Go-side
// SynthRecipeProvider (the BaseRecipe field on the doc is informational
// metadata for telemetry; the provider is what actually renders).
//
// The doc's ParamSeed (if any) is materialised into ParamDef defaults at
// registration time so the freshly-registered recipe ships with the saved
// tone. Subsequent param edits via SetInstrumentParam overlay on top.
func RegisterPluginRecipeFromDoc(doc RecipeDoc, provider SynthRecipeProvider) error {
	if doc.ID == "" {
		return fmt.Errorf("%w: ID is required", ErrInvalidPluginRecipe)
	}
	if provider == nil {
		return fmt.Errorf("%w: Provider is required", ErrInvalidPluginRecipe)
	}
	// Apply the seed onto the param defs so RecipeDefaultParams reflects it.
	params := make([]ParamDef, len(doc.ParamDefs))
	copy(params, doc.ParamDefs)
	if len(doc.ParamSeed) > 0 {
		for i := range params {
			if v, ok := doc.ParamSeed[params[i].Name]; ok {
				if v < params[i].Min {
					v = params[i].Min
				} else if v > params[i].Max {
					v = params[i].Max
				}
				params[i].Default = v
			}
		}
	}
	display := doc.DisplayName
	if display == "" {
		display = doc.ID
	}
	category := doc.Category
	if category == "" {
		category = "plugin"
	}
	return RegisterPluginRecipe(PluginRecipeOptions{
		ID:          doc.ID,
		DisplayName: display,
		Category:    category,
		ParamDefs:   params,
		Provider:    provider,
	})
}

// pluginRecipe adapts a SynthRecipeProvider to the SynthRecipe interface
// so it can flow through the existing voice-cache dispatch path
// (synth_recipe_dispatch.go) unchanged.
type pluginRecipe struct {
	id          string
	displayName string
	category    string
	params      []ParamDef
	provider    SynthRecipeProvider
}

func (r *pluginRecipe) ID() string              { return r.id }
func (r *pluginRecipe) DisplayName() string     { return r.displayName }
func (r *pluginRecipe) Category() string        { return r.category }
func (r *pluginRecipe) ParamSchema() []ParamDef { return r.params }

func (r *pluginRecipe) Render(buf []float32, sampleRate, samples, variant int, p RecipeParams) {
	if r.provider == nil || len(buf) == 0 || samples <= 0 {
		return
	}
	r.provider.Render(buf, sampleRate, samples, variant, p)
}

// SineProvider is a tiny pure-Go SynthRecipeProvider that emits a fixed
// 1-second windowed sine wave. Used as the canonical "does the plugin path
// actually work end-to-end?" fixture in TestPluginRecipeRoundtrip. Real
// plugins (whether Go or wasm-loaded) would do real synthesis.
type SineProvider struct {
	BaseFreqHz float64 // identity 220 when zero
}

// Render writes a sine of `p["freq"]` Hz (default = BaseFreqHz, default 220)
// with a gentle exponential decay. Honors the `decay` ParamDef when present.
func (s SineProvider) Render(buf []float32, sampleRate, samples, _ int, p RecipeParams) {
	freq := s.BaseFreqHz
	if freq == 0 {
		freq = 220
	}
	if v, ok := p["freq"]; ok && v > 0 {
		freq = v
	}
	decay := 4.0
	if v, ok := p["decay"]; ok && v > 0 {
		decay = v
	}
	n := samples
	if n > len(buf) {
		n = len(buf)
	}
	for i := 0; i < n; i++ {
		t := float64(i) / float64(sampleRate)
		buf[i] = float32(math.Sin(2*math.Pi*freq*t) * math.Exp(-decay*t))
	}
}
