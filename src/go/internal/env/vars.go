package env

// Every environment variable the game reads, declared once.

// ---- Runtime configuration -------------------------------------------------

var (
	// AssetsRoot overrides where instrument samples and templates load from.
	AssetsRoot = Str("BEATMO_ASSETS", "", ScopeRuntime,
		"Override the default assets root directory.")

	DemoConfig = Str("BEATMO_DEMO_CONFIG", "", ScopeRuntime,
		"Path to a JSON circuit loaded as the initial demo.")

	RecordingsDir = Str("BEATMO_RECORDINGS_DIR", "", ScopeRuntime,
		"Base directory for recording output; defaults to $CWD/recordings.")

	AudioSampleRate = Int("BEATMO_AUDIO_SAMPLE_RATE", 48000, ScopeRuntime,
		"Output sample rate in Hz (default = audio.EngineSampleRate, the rate the browser runs at too). Was AUDIO_SAMPLE_RATE.")
)

// ---- Logging and event stream ----------------------------------------------

var (
	InfoLog = Str("BEATMO_INFO_LOG", "", ScopeRuntime,
		"Set to 'off' to disable the hooks.Bus -> INFO narrative consumer (default: on).")

	EventLog = Str("BEATMO_EVENT_LOG", "", ScopeRuntime,
		"Write the full hooks.Bus event stream to this JSONL path.")

	EventLogVerbose = Bool("BEATMO_EVENT_LOG_VERBOSE", false, ScopeDebug,
		"Include verbose kinds (drag-progress / EventDragProgress) in both the INFO narrative and the JSONL stream.")

	TestLog = Bool("BEATMO_TEST_LOG", false, ScopeTest,
		"Enable in-process test logger output (default silent).")

	TestLogLevel = Str("BEATMO_TEST_LOG_LEVEL", "", ScopeTest,
		"Override log level when test logging is enabled: TRACE|DEBUG|INFO|WARN|ERROR|NONE.")
)

// ---- Profiling and runtime tuning ------------------------------------------

var (
	PProf = Bool("BEATMO_PPROF", false, ScopeDebug,
		"Start the pprof HTTP server on localhost:6060. Was PPROF.")

	PyroscopeURL = Str("PYROSCOPE_URL", "", ScopeDebug,
		"Pyroscope profiling backend URL. Pyroscope is disabled unless this is set.")

	PyroscopeApp = Str("PYROSCOPE_APP", "beatmo", ScopeDebug,
		"Pyroscope app identifier.")

	BlockProfile = Int("BEATMO_BLOCK_PROFILE", 0, ScopeDebug,
		"runtime.SetBlockProfileRate value. 0 = off, 1 = every event (heavy).")

	MutexProfile = Int("BEATMO_MUTEX_PROFILE", 0, ScopeDebug,
		"runtime.SetMutexProfileFraction value. 0 = off, 1 = every contention event.")

	MemoryLimitMB = Int("BEATMO_MEMORY_LIMIT_MB", 0, ScopeRuntime,
		"Soft memory limit via debug.SetMemoryLimit, in MB. 0 leaves the runtime default (unbounded).")

	GCPercent = Int("BEATMO_GC_PERCENT", 0, ScopeRuntime,
		"debug.SetGCPercent value. 0 leaves the Go default (100); lower triggers more frequent, shorter pauses.")

	GOMAXPROCS = Int("BEATMO_GOMAXPROCS", 0, ScopeRuntime,
		"runtime.GOMAXPROCS value. 0 leaves the default (NumCPU).")

	PerfLog = Bool("PERF_LOG", false, ScopeDebug,
		"Emit perf logs roughly every 2 seconds.")

	PerfBrowserUpdateMaxMS = Int("PERF_BROWSER_UPDATE_MAX_MS", 0, ScopeDebug,
		"Max Update() duration for browser perf optimization; presence alone (regardless of value) enables the fast perf path.")

	RecordBenchHeapSnapshots = Bool("BEATMO_RECORD_BENCH_HEAP_SNAPSHOTS", false, ScopeDebug,
		"Write heap snapshots during -record-bench.")

	RecordBenchSynthChurnHz = Int("BEATMO_RECORD_BENCH_SYNTH_CHURN_HZ", 0, ScopeDebug,
		"Synth-param mutation rate during -record-bench.")
)

// ---- Heap diagnostics -------------------------------------------------------

// One three-state level: probe implies watch.
var HeapDiag = Str("BEATMO_HEAP_DIAG", "", ScopeDebug,
	"Heap diagnostics level: 'watch' enables the 30 s heap-growth rollup WARN; "+
		"'probe' additionally enables the per-second allocation-rate probe with "+
		"top contributors by tag. Replaces BEATMO_HEAP_PROBE and BEATMO_HEAP_WATCH.")

// HeapWatchEnabled reports whether the 30 s heap-growth rollup is on.
func HeapWatchEnabled() bool {
	switch HeapDiag.Get() {
	case "watch", "probe":
		return true
	}
	return false
}

// Probe implies watch.
func HeapProbeEnabled() bool { return HeapDiag.Get() == "probe" }

// Bench-sweep overrides of RuntimeProfile fields (ui/runtime_profile_env.go), applied only when IsSet, so they never
// clobber the platform default.

// The declared default is a not-set sentinel: the real default comes from the platform's profile builder. Not
// duplicated here, since those bench-tuned numbers change independently and a copy would rot.
const notSetSentinel = " Unset leaves the platform RuntimeProfile's built-in value; " +
	"the default shown here is a not-set sentinel, not the effective default " +
	"(see internal/ui/runtime_profile.go)."

var (
	EdgeCachePad = Int("BEATMO_EDGE_CACHE_PAD", 0, ScopeDebug,
		"Override RuntimeProfile.EdgeCachePad for bench sweeps without recompiling."+notSetSentinel)

	GridCachePad = Int("BEATMO_GRID_CACHE_PAD", 0, ScopeDebug,
		"Override RuntimeProfile.GridCachePad for bench sweeps without recompiling."+notSetSentinel)

	DisableNodeGlow = Bool("BEATMO_DISABLE_NODE_GLOW", false, ScopeDebug,
		"Override RuntimeProfile.DisableNodeGlow for bench sweeps without recompiling."+notSetSentinel)

	DisableEdgeArrows = Bool("BEATMO_DISABLE_EDGE_ARROWS", false, ScopeDebug,
		"Override RuntimeProfile.DisableEdgeArrows for bench sweeps without recompiling."+notSetSentinel)

	SimpleDrawDefault = Bool("BEATMO_SIMPLE_DRAW_DEFAULT", false, ScopeDebug,
		"Override RuntimeProfile.SimpleDrawDefault for bench sweeps without recompiling."+notSetSentinel)

	TimelineInfoThrottleMS = Int("BEATMO_TIMELINE_INFO_THROTTLE_MS", 0, ScopeDebug,
		"Override RuntimeProfile.TimelineInfoThrottleMS for bench sweeps without recompiling."+notSetSentinel)

	AudioLookaheadSec = Float("BEATMO_AUDIO_LOOKAHEAD_SEC", 0, ScopeDebug,
		"Override RuntimeProfile.AudioLookaheadSec for bench sweeps without recompiling."+notSetSentinel)

	// A/B of the audio lane against the legacy grid-time dispatch (scripts/audio_lane_ab.mjs sets them per arm).
	AudioLane = Bool("BEATMO_AUDIO_LANE", false, ScopeDebug,
		"Override RuntimeProfile.AudioLaneEnabled (audio lane vs legacy grid-time dispatch)."+notSetSentinel)
	AudioHorizonSec = Float("BEATMO_AUDIO_HORIZON_SEC", 0, ScopeDebug,
		"Override RuntimeProfile.AudioHorizonSec (audio lane commit window)."+notSetSentinel)

	// Like the rest, inert unless set; unlike them its effective built-in value differs between browser and desktop, which
	// the purpose string says.
	MinDispatchLeadSec = Float("BEATMO_MIN_DISPATCH_LEAD_SEC", 0, ScopeDebug,
		"Minimum lead time in seconds before a scheduled audio event is dispatched. "+
			"Bench-sweep knob; was reachable only through window.__beatmoProfileOverride "+
			"before env vars worked on WASM."+notSetSentinel+
			" The effective built-in value differs between browser and desktop "+
			"(see internal/ui/runtime_profile.go) — check both before assuming a sweep result generalizes.")

	AudioBatchMax = Int("BEATMO_AUDIO_BATCH_MAX", 0, ScopeDebug,
		"Override RuntimeProfile.AudioBatchMax for bench sweeps without recompiling."+notSetSentinel)

	// The sequencer loop reads only RuntimeProf().SequencerTickMS; there is no second, bare-named variable.
	SequencerTickMS = Int("BEATMO_SEQUENCER_TICK_MS", 0, ScopeDebug,
		"Override RuntimeProfile.SequencerTickMS for bench sweeps without recompiling. Also replaces the removed SEQ_TICK_MS."+notSetSentinel)

	AdaptivePanPad = Bool("BEATMO_ADAPTIVE_PAN_PAD", false, ScopeDebug,
		"Override RuntimeProfile.AdaptivePanPad for bench sweeps without recompiling."+notSetSentinel)

	FastPanDetect = Bool("BEATMO_FAST_PAN_DETECT", false, ScopeDebug,
		"Override RuntimeProfile.FastPanDetect for bench sweeps without recompiling."+notSetSentinel)

	PredictorBackoff = Bool("BEATMO_PREDICTOR_BACKOFF", false, ScopeDebug,
		"Override RuntimeProfile.PredictorBackoffOnSlowDraw for bench sweeps without recompiling."+notSetSentinel)

	// ForceGCInterval's environment value is milliseconds; 0 disables the
	// cooperative forced GC. Desktop default is 0 (off).
	ForceGCInterval = Dur("BEATMO_FORCE_GC_INTERVAL_MS", 0, ScopeDebug,
		"Override RuntimeProfile.ForceGCInterval (milliseconds) for bench sweeps without recompiling."+notSetSentinel)
)

// ---- Parity -----------------------------------------------------------------

var (
	ParityFatal = Str("PARITY_FATAL", "", ScopeDebug,
		"'0' or 'false' disables parity panics.")

	ParityWatch = Str("PARITY_WATCH", "", ScopeDebug,
		"Parity mode on desktop: 'log' or 'panic'.")

	ParityWasmFatal = Str("PARITY_WASM_FATAL", "", ScopeDebug,
		"Arm parity panics on WASM: '1', 'true', or 'panic'.")

	ParityTiming = Str("PARITY_TIMING", "", ScopeDebug,
		"Timing-gate watch mode.")

	ParityTimingFatal = Str("PARITY_TIMING_FATAL", "", ScopeDebug,
		"Arm timing-gate panics on browser.")

	ParityDumpStderr = Bool("PARITY_DUMP_STDERR", false, ScopeDebug,
		"Dump parity error messages to stderr.")

	ParityScanEvery = Int("PARITY_SCAN_EVERY", 4, ScopeDebug,
		"Frames between parity scan passes.")

	ParityScanStride = Int("PARITY_SCAN_STRIDE", 1, ScopeDebug,
		"Cells per parity scan pass.")
)

// ---- Audio debug ------------------------------------------------------------

var (
	TestTone = Bool("BEATMO_TEST_TONE", false, ScopeDebug,
		"Output a pure 440 Hz sine, bypassing all synth. Tests the Oto/driver path. Was TEST_TONE.")

	TestVoice = Bool("BEATMO_TEST_VOICE", false, ScopeDebug,
		"Replace the C synth with simple 220 Hz sine voices. Tests the voice/mixer path. Was TEST_VOICE.")

	TestRawVoice = Bool("BEATMO_TEST_RAW_VOICE", false, ScopeDebug,
		"Output the first voice's raw samples, bypassing all processing. Was TEST_RAW_VOICE.")

	TestRawVoiceFull = Bool("BEATMO_TEST_RAW_VOICE_FULL", false, ScopeDebug,
		"With TestRawVoice, use full amplitude (1.0x) instead of 0.2x. Was TEST_RAW_VOICE_FULL.")

	TestRawVoiceNoScale = Bool("BEATMO_TEST_RAW_VOICE_NOSCALE", false, ScopeDebug,
		"With TestRawVoice, skip all multiplication; write direct to int16. Was TEST_RAW_VOICE_NOSCALE.")

	SingleVoice = Bool("BEATMO_SINGLE_VOICE", false, ScopeDebug,
		"Limit the mixer to one voice at a time. Tests multi-voice accumulation. Was SINGLE_VOICE.")

	BypassChannelProc = Bool("BYPASS_CHANNEL_PROC", false, ScopeDebug,
		"Skip all channel processing (volume, EQ). Isolates mixer vs channel bugs.")

	BypassHeadroom = Bool("BYPASS_HEADROOM", false, ScopeDebug,
		"Skip per-voice headroom attenuation (0.25x).")

	BypassEQ = Bool("BYPASS_EQ", false, ScopeDebug,
		"Skip all EQ/processor processing.")

	BypassNormalize = Bool("BYPASS_NORMALIZE", false, ScopeDebug,
		"Skip peak normalization during export.")

	AudioClipDebug = Bool("AUDIO_CLIP_DEBUG", false, ScopeDebug,
		"Log clipping events (samples exceeding +/-1.0 before hard clamp).")

	DebugMixer = Bool("DEBUG_MIXER", false, ScopeDebug,
		"Log mixer workBuf min/max values and voice stats periodically.")

	DebugChannel = Bool("DEBUG_CHANNEL", false, ScopeDebug,
		"Log channel processing values.")
)

// ---- Render and UI debug ----------------------------------------------------

var (
	RenderSafe = Bool("RENDER_SAFE", false, ScopeDebug,
		"Force drawRect-based rendering instead of sprite cache, for pixel testing.")

	ScreenEdges = Bool("SCREEN_EDGES", false, ScopeDebug,
		"Draw screen-space edge arrows at viewport edges.")

	NoGridDraw = Bool("NO_GRID_DRAW", false, ScopeDebug,
		"Skip grid pane drawing entirely.")

	NoGridTileCache = Bool("NO_GRID_TILE_CACHE", false, ScopeDebug,
		"Disable grid tile caching (force redraw every frame).")

	NoPixelSnap = Bool("NO_PIXEL_SNAP", false, ScopeDebug,
		"Disable pixel snapping for geometry diagnostics.")

	NoEdgeCache = Bool("NO_EDGE_CACHE", false, ScopeDebug,
		"Disable edge cache (force redraw every frame).")

	NoSpriteNodes = Bool("NO_SPRITE_NODES", false, ScopeDebug,
		"Disable node sprite rendering.")

	DebugGeom = Bool("DEBUG_GEOM", false, ScopeDebug,
		"Verbose geometry logs.")

	DebugDrawNodes = Bool("DEBUG_DRAW_NODES", false, ScopeDebug,
		"Enable detailed node draw logs.")

	DebugLayout = Bool("BEATMO_DEBUG_LAYOUT", false, ScopeDebug,
		"Draw developer-only layout guide ticks between widget surfaces.")

	DebugInst = Bool("BEATMO_DEBUG_INST", false, ScopeDebug,
		"Debug logging for instrument menu operations.")
)

// ---- Timeline, sequencer, and synth debug -----------------------------------

var (
	TimelineTrace = Bool("TIMELINE_TRACE", false, ScopeDebug,
		"Trace timeline masking logic.")

	TimelineTraceRow = Int("TIMELINE_TRACE_ROW", 0, ScopeDebug,
		"Which row to trace.")

	DebugHistorySeed = Bool("DEBUG_HISTORY_SEED", false, ScopeDebug,
		"Debug timeline history seeding logic.")

	RowSnapshots = Bool("BEATMO_ROW_SNAPSHOTS", false, ScopeDebug,
		"Enable row snapshot mode.")

	BpmTimingTest = Bool("BPM_TIMING_TEST", false, ScopeDebug,
		"Enable BPM timing test mode.")

	SynthDispatchDebug = Bool("BEATMO_SYNTH_DISPATCH_DEBUG", false, ScopeDebug,
		"Log which dispatch path each voice took (recipe vs legacy), the merged paramsHash, and the instrument id. Diagnoses \"the slider does nothing\" reports.")

	PrewarmDebug = Bool("BEATMO_PREWARM_DEBUG", false, ScopeDebug,
		"Log voice-cache HIT/RENDER for the pitched path with the full cache key, exposing warm-vs-dispatch key mismatches.")
)

// ---- Scope export -----------------------------------------------------------

var (
	ScopeExport = Bool("SCOPE_EXPORT", false, ScopeDebug,
		"Enable the scope export flight recorder (JSONL).")

	ScopeExportPath = Str("SCOPE_EXPORT_PATH", "scope_export.jsonl", ScopeDebug,
		"Output file for the scope export flight recorder.")

	ScopeExportInterval = Float("SCOPE_EXPORT_INTERVAL", 2.0, ScopeDebug,
		"Snapshot interval, in seconds. Fractional values are supported (e.g. 0.5).")
)

// ---- Test-only ---------------------------------------------------------------

var (
	WavRefs = Str("BEATMO_WAV_REFS", "", ScopeTest,
		"Directory of local, uncommitted reference WAVs; tests skip cleanly when unset.")

	DiagWav = Str("BEATMO_DIAG_WAV", "", ScopeTest,
		"Dump diagnostic WAVs from the template audio report to this path.")

	RtfMin = Float("BEATMO_RTF_MIN", 20.0, ScopeTest,
		"Minimum acceptable render real-time factor for the P5 render performance gate.")

	UpdateOracle = Bool("BEATMO_UPDATE_ORACLE", false, ScopeTest,
		"Regenerate the legacy oracle golden.")

	UpdateGolden = Bool("BEATMO_UPDATE_GOLDEN", false, ScopeTest,
		"Regenerate the eventlogger session golden. Was UPDATE_GOLDEN.")

	IfxGoldenUpdate = Bool("IFX_GOLDEN_UPDATE", false, ScopeTest,
		"Regenerate the insert-FX cross-platform golden.")

	RunSoakReal = Bool("BEATMO_RUN_SOAK_REAL", false, ScopeTest,
		"Opt into the long real-Ebiten heap soak test. Was RUN_SOAK_REAL.")

	FontcacheSubprocess = Bool("FONTCACHE_SUBPROCESS", false, ScopeTest,
		"Internal marker identifying the font-cache test subprocess.")

	KickSweep = Bool("BEATMO_KICK_SWEEP", false, ScopeTest,
		"Run the kick parameter sweep. Was SWEEP.")

	WriteWav = Bool("BEATMO_WRITE_WAV", false, ScopeTest,
		"Write tuning output WAVs to disk. Was WRITE_WAV.")

	VerifyDeployURL = Str("BEATMO_VERIFY_DEPLOY_URL", "", ScopeTest,
		"Live deploy target for the post-deploy header probe.")

	WasmRawMaxBytes = Int("BEATMO_WASM_RAW_MAX_BYTES", 29360128, ScopeTest,
		"Raw main.wasm size budget, in bytes.")

	WasmGzipMaxBytes = Int("BEATMO_WASM_GZIP_MAX_BYTES", 6291456, ScopeTest,
		"Gzipped main.wasm size budget, in bytes.")

	AssetsGzipMaxBytes = Int("BEATMO_ASSETS_GZIP_MAX_BYTES", 6553600, ScopeTest,
		"Total gzipped asset budget, in bytes.")
)
