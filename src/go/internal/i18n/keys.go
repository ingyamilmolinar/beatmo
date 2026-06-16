package i18n

// Key identifies one user-facing message.
//
// ADD-A-KEY CHOKEPOINT: to add a message you must touch exactly four places —
// (1) a const here, (2) AllKeys(), (3) catalog_en.go, (4) catalog_es.go. The
// completeness test fails loudly if any of the four is missed.
type Key string

const (
	// Transport
	KeyTransportPlay  Key = "transport.play"
	KeyTransportPause Key = "transport.pause"
	KeyTransportRec   Key = "transport.rec"

	// Settings / shortcuts
	KeySettingsTitle       Key = "settings.title"
	KeySettingsLanguage    Key = "settings.language"
	KeySettingsShortcuts   Key = "settings.shortcuts"
	KeySettingsDesktopOnly Key = "settings.shortcuts.desktop_only"
	KeyLangEnglish         Key = "settings.lang.english"
	KeyLangSpanish         Key = "settings.lang.spanish"

	// Shortcut action descriptions (key glyphs stay literal)
	KeyShortcutPlayPause Key = "shortcut.play_pause"
	KeyShortcutCancel    Key = "shortcut.cancel"
	KeyShortcutPan       Key = "shortcut.pan"
	KeyShortcutZoom      Key = "shortcut.zoom"
	KeyShortcutResetZoom Key = "shortcut.reset_zoom"
	KeyShortcutBPM       Key = "shortcut.bpm"
	KeyShortcutTabs      Key = "shortcut.tabs"
	KeyShortcutHelp      Key = "shortcut.help"

	// Menus (context menu + overflow menu)
	KeyMenuRename       Key = "menu.rename"
	KeyMenuColor        Key = "menu.color"
	KeyMenuOrigin       Key = "menu.origin"
	KeyMenuDelete       Key = "menu.delete"
	KeyMenuFile         Key = "menu.file"
	KeyMenuUpload       Key = "menu.upload"
	KeyMenuImport       Key = "menu.import"
	KeyMenuExport       Key = "menu.export"
	KeyMenuLoadTemplate Key = "menu.load_template"
	KeyMenuTemplates    Key = "menu.templates"
	KeyMenuBack         Key = "menu.back"

	// Action button labels
	KeySave      Key = "btn.save"
	KeySaveAs    Key = "btn.save_as"
	KeyReset     Key = "btn.reset"
	KeyPreview   Key = "btn.preview"
	KeyCancel    Key = "btn.cancel"
	KeyOK        Key = "btn.ok"
	KeyAddEffect Key = "btn.add_effect"
	KeyLoadWAV   Key = "btn.load_wav"
	KeyNormalize Key = "btn.normalize"
	KeyReverse   Key = "btn.reverse"
	KeyFade      Key = "btn.fade"

	// Builtin instrument default display names
	KeyInstKick  Key = "inst.kick"
	KeyInstSnare Key = "inst.snare"
	KeyInstHiHat Key = "inst.hihat"
	KeyInstClap  Key = "inst.clap"
	KeyInstTom   Key = "inst.tom"
	KeyInstBass  Key = "inst.bass"

	// Audio-panel tab labels (desktop full + mobile-short), plus bottom-nav
	KeyTabWave     Key = "tab.wave"
	KeyTabSpectrum Key = "tab.spectrum"
	KeyTabLevels   Key = "tab.levels"
	KeyTabEQ       Key = "tab.eq"
	KeyTabChain    Key = "tab.chain"
	KeyTabSynth    Key = "tab.synth"
	KeyTabSampler  Key = "tab.sampler"

	KeyTabWaveShort     Key = "tab.short.wave"
	KeyTabSpectrumShort Key = "tab.short.spectrum"
	KeyTabLevelsShort   Key = "tab.short.levels"
	KeyTabEQShort       Key = "tab.short.eq"
	KeyTabChainShort    Key = "tab.short.chain"
	KeyTabSynthShort    Key = "tab.short.synth"
	KeyTabSamplerShort  Key = "tab.short.sampler"

	KeyNavPads Key = "nav.pads"

	// Audio-panel legend chips (one per PanelTab)
	KeyLegendWave     Key = "legend.wave"
	KeyLegendSpectrum Key = "legend.spectrum"
	KeyLegendMeters   Key = "legend.meters"
	KeyLegendEQ       Key = "legend.eq"
	KeyLegendScope    Key = "legend.scope"
	KeyLegendSynth    Key = "legend.synth"
	KeyLegendSampler  Key = "legend.sampler"

	// PlainEnglish knob/label glosses (kid-readable subtitles)
	KeyGlossVoice       Key = "gloss.voice"
	KeyGlossOsc         Key = "gloss.osc"
	KeyGlossPitch       Key = "gloss.pitch"
	KeyGlossWaveform    Key = "gloss.waveform"
	KeyGlossGenerator   Key = "gloss.generator"
	KeyGlossTune        Key = "gloss.tune"
	KeyGlossGlide       Key = "gloss.glide"
	KeyGlossBend        Key = "gloss.bend"
	KeyGlossDetune      Key = "gloss.detune"
	KeyGlossEnvelope    Key = "gloss.envelope"
	KeyGlossAttack      Key = "gloss.attack"
	KeyGlossDecay       Key = "gloss.decay"
	KeyGlossSustain     Key = "gloss.sustain"
	KeyGlossRelease     Key = "gloss.release"
	KeyGlossTone        Key = "gloss.tone"
	KeyGlossCutoff      Key = "gloss.cutoff"
	KeyGlossFilter      Key = "gloss.filter"
	KeyGlossReso        Key = "gloss.reso"
	KeyGlossResonance   Key = "gloss.resonance"
	KeyGlossBody        Key = "gloss.body"
	KeyGlossBasePitch   Key = "gloss.base_pitch"
	KeyGlossPitchSweep  Key = "gloss.pitch_sweep"
	KeyGlossSweepTime   Key = "gloss.sweep_time"
	KeyGlossOpRatio     Key = "gloss.op_ratio"
	KeyGlossOpFMDepth   Key = "gloss.op_fm_depth"
	KeyGlossOpDecay     Key = "gloss.op_decay"
	KeyGlossFundamental Key = "gloss.fundamental"
	KeyGlossSweepSpeed  Key = "gloss.sweep_speed"
	KeyGlossBoomDecay   Key = "gloss.boom_decay"
	KeyGlossBodyDecay   Key = "gloss.body_decay"
	KeyGloss2ndHarmonic Key = "gloss.2nd_harmonic"
	KeyGloss3rdHarmonic Key = "gloss.3rd_harmonic"
	KeyGloss4thHarmonic Key = "gloss.4th_harmonic"
	KeyGlossClick       Key = "gloss.click"
	KeyGlossThud        Key = "gloss.thud"
	KeyGlossRingDecay   Key = "gloss.ring_decay"
	KeyGlossOvertone1   Key = "gloss.overtone1"
	KeyGlossOvertone2   Key = "gloss.overtone2"
	KeyGlossStick       Key = "gloss.stick"
	KeyGlossRoom        Key = "gloss.room"
	KeyGlossTone2       Key = "gloss.tone2"
	KeyGlossNoiseTune   Key = "gloss.noise_tune"
	KeyGlossToneDecay   Key = "gloss.tone_decay"
	KeyGlossNoiseDecay  Key = "gloss.noise_decay"
	KeyGlossTailDecay   Key = "gloss.tail_decay"
	KeyGlossToneLevel   Key = "gloss.tone_level"
	KeyGlossNoiseLevel  Key = "gloss.noise_level"
	KeyGlossWires       Key = "gloss.wires"
	KeyGlossSnap        Key = "gloss.snap"
	KeyGlossMetalTune   Key = "gloss.metal_tune"
	KeyGlossAttackDecay Key = "gloss.attack_decay"
	KeyGlossMetalLevel  Key = "gloss.metal_level"
	KeyGlossSizzleLevel Key = "gloss.sizzle_level"
	KeyGlossSizzleDecay Key = "gloss.sizzle_decay"
	KeyGlossPluck       Key = "gloss.pluck"
	KeyGlossPick        Key = "gloss.pick"
	KeyGlossFade        Key = "gloss.fade"
	KeyGlossOvertone    Key = "gloss.overtone"
	KeyGlossPitchPunch  Key = "gloss.pitch_punch"
	KeyGlossPunch       Key = "gloss.punch"
	KeyGlossDrive       Key = "gloss.drive"
	KeyGlossGain        Key = "gloss.gain"
	KeyGlossSaturation  Key = "gloss.saturation"
	KeyGlossSat         Key = "gloss.sat"
	KeyGlossDistortion  Key = "gloss.distortion"
	KeyGlossSlope       Key = "gloss.slope"
	KeyGlossSlopes      Key = "gloss.slopes"
	KeyGlossPre         Key = "gloss.pre"
	KeyGlossPost        Key = "gloss.post"
	KeyGlossPeak        Key = "gloss.peak"
	KeyGlossRMS         Key = "gloss.rms"
	KeyGlossClip        Key = "gloss.clip"
	KeyGlossClips       Key = "gloss.clips"
	KeyGlossHeadroom    Key = "gloss.headroom"
	KeyGlossLUFS        Key = "gloss.lufs"
	KeyGlossK20         Key = "gloss.k20"
	KeyGlossOverlay     Key = "gloss.overlay"
	KeyGlossSplit       Key = "gloss.split"
	KeyGlossDiff        Key = "gloss.diff"
	KeyGlossOvr         Key = "gloss.ovr"
	KeyGlossSpl         Key = "gloss.spl"
	KeyGlossDif         Key = "gloss.dif"
	KeyGlossAG          Key = "gloss.ag"
	KeyGlossAutoGain    Key = "gloss.auto_gain"
	KeyGlossOut         Key = "gloss.out"
	KeyGlossDelay       Key = "gloss.delay"
	KeyGlossReverb      Key = "gloss.reverb"
	KeyGlossSend        Key = "gloss.send"
	KeyGlossPlay        Key = "gloss.play"
	KeyGlossStop        Key = "gloss.stop"
	KeyGlossPause       Key = "gloss.pause"

	// Caption labels (drawn each frame near controls / overlays)
	KeyCapMove         Key = "cap.move"
	KeyCapMoveNode     Key = "cap.move_node"
	KeyCapConnect      Key = "cap.connect"
	KeyCapYourSound    Key = "cap.your_sound"
	KeyCapVol          Key = "cap.vol"
	KeyCapPitch        Key = "cap.pitch"
	KeyCapPct          Key = "cap.pct"
	KeyCapDur          Key = "cap.dur"
	KeyCapSearch       Key = "cap.search"
	KeyCapHeadroom     Key = "cap.headroom"
	KeyCapLoudest      Key = "cap.loudest"
	KeyCapFrozen       Key = "cap.frozen"
	KeyCapBypassed     Key = "cap.bypassed"
	KeyCapSamplerTitle Key = "cap.sampler_title"

	// Final i18n cleanup pass — remaining user-facing chrome words.
	KeyMaster          Key = "mix.master"
	KeyNoNotifications Key = "notif.empty"
	KeyNodeLogic       Key = "node.logic"
	KeyNodeGroove      Key = "node.groove"
	KeyNoRowSelected   Key = "synth.no_row"
	KeyAuto            Key = "btn.auto"
	KeyPre             Key = "btn.pre"

	// Node sidebar (click-a-node menu) — titles, section headers, logic/groove
	// dropdown labels, and collapsed short/summary labels.
	KeyNodeTitle            Key = "node.title"
	KeyNodeSecVolume        Key = "node.sec.volume"
	KeyNodeSecDuration      Key = "node.sec.duration"
	KeyNodeSecLogic         Key = "node.sec.logic"
	KeyNodeSecGroove        Key = "node.sec.groove"
	KeyNodeSecAudible       Key = "node.sec.audible"
	KeyNodeSecMove          Key = "node.sec.move"
	KeyLogicNone            Key = "logic.none"
	KeyLogicEveryN          Key = "logic.every_n"
	KeyLogicSkipN           Key = "logic.skip_n"
	KeyLogicProbability     Key = "logic.probability"
	KeyLogicIfPrevSkipped   Key = "logic.if_prev_skipped"
	KeyLogicIfPrevTriggered Key = "logic.if_prev_triggered"
	KeyLogicShortEveryN     Key = "logic.short.every_n"
	KeyLogicShortSkipN      Key = "logic.short.skip_n"
	KeyLogicShortProb       Key = "logic.short.prob"
	KeyLogicShortPrevSkip   Key = "logic.short.prev_skip"
	KeyLogicShortPrevTrig   Key = "logic.short.prev_trig"
	KeyGrooveDelay          Key = "groove.delay"
	KeyGrooveRush           Key = "groove.rush"
	KeyAudSilent            Key = "aud.silent"
	KeyAudMuted             Key = "aud.muted"

	// Notifications / toasts (user-facing notifyInfo / notifyError messages)
	KeyNotifInvalidName            Key = "notif.invalid_name"
	KeyNotifRenamedInstrument      Key = "notif.renamed_instrument"
	KeyNotifErrLoadJSON            Key = "notif.err_load_json"
	KeyNotifImported               Key = "notif.imported"
	KeyNotifRecordingFailed        Key = "notif.recording_failed"
	KeyNotifSavingRecording        Key = "notif.saving_recording"
	KeyNotifCannotStartRecording   Key = "notif.cannot_start_recording"
	KeyNotifRecordingSaveFailed    Key = "notif.recording_save_failed"
	KeyNotifRecordingSaved         Key = "notif.recording_saved"
	KeyNotifImportCanceled         Key = "notif.import_canceled"
	KeyNotifErrLoadWAV             Key = "notif.err_load_wav"
	KeyNotifSelectedWAV            Key = "notif.selected_wav"
	KeyNotifCannotConnectInvisible Key = "notif.cannot_connect_invisible"
	KeyNotifOnlyPerpendicular      Key = "notif.only_perpendicular"
	KeyNotifInstNameEmpty          Key = "notif.inst_name_empty"
	KeyNotifUpdatedWAVInst         Key = "notif.updated_wav_inst"
	KeyNotifLoadedWAVInst          Key = "notif.loaded_wav_inst"
	KeyNotifInvalidBPM             Key = "notif.invalid_bpm"
)

// AllKeys lists every declared Key. The completeness test asserts this set is
// exactly the key set of every locale catalog.
func AllKeys() []Key {
	return []Key{
		KeyTransportPlay, KeyTransportPause, KeyTransportRec,
		KeySettingsTitle, KeySettingsLanguage, KeySettingsShortcuts, KeySettingsDesktopOnly,
		KeyLangEnglish, KeyLangSpanish,
		KeyShortcutPlayPause, KeyShortcutCancel, KeyShortcutPan, KeyShortcutZoom,
		KeyShortcutResetZoom, KeyShortcutBPM, KeyShortcutTabs, KeyShortcutHelp,
		KeyMenuRename, KeyMenuColor, KeyMenuOrigin, KeyMenuDelete,
		KeyMenuFile, KeyMenuUpload, KeyMenuImport, KeyMenuExport,
		KeyMenuLoadTemplate, KeyMenuTemplates, KeyMenuBack,
		KeySave, KeySaveAs, KeyReset, KeyPreview, KeyCancel, KeyOK,
		KeyAddEffect, KeyLoadWAV, KeyNormalize, KeyReverse, KeyFade,
		KeyInstKick, KeyInstSnare, KeyInstHiHat, KeyInstClap, KeyInstTom, KeyInstBass,
		KeyTabWave, KeyTabSpectrum, KeyTabLevels, KeyTabEQ, KeyTabChain, KeyTabSynth, KeyTabSampler,
		KeyTabWaveShort, KeyTabSpectrumShort, KeyTabLevelsShort, KeyTabEQShort,
		KeyTabChainShort, KeyTabSynthShort, KeyTabSamplerShort,
		KeyNavPads,
		KeyLegendWave, KeyLegendSpectrum, KeyLegendMeters, KeyLegendEQ,
		KeyLegendScope, KeyLegendSynth, KeyLegendSampler,
		KeyGlossVoice, KeyGlossOsc, KeyGlossPitch, KeyGlossWaveform, KeyGlossGenerator,
		KeyGlossTune, KeyGlossGlide, KeyGlossBend, KeyGlossDetune,
		KeyGlossEnvelope, KeyGlossAttack, KeyGlossDecay, KeyGlossSustain, KeyGlossRelease,
		KeyGlossTone, KeyGlossCutoff, KeyGlossFilter, KeyGlossReso, KeyGlossResonance, KeyGlossBody,
		KeyGlossBasePitch, KeyGlossPitchSweep, KeyGlossSweepTime, KeyGlossOpRatio,
		KeyGlossOpFMDepth, KeyGlossOpDecay,
		KeyGlossFundamental, KeyGlossSweepSpeed, KeyGlossBoomDecay, KeyGlossBodyDecay,
		KeyGloss2ndHarmonic, KeyGloss3rdHarmonic, KeyGloss4thHarmonic, KeyGlossClick, KeyGlossThud,
		KeyGlossRingDecay, KeyGlossOvertone1, KeyGlossOvertone2, KeyGlossStick, KeyGlossRoom,
		KeyGlossTone2, KeyGlossNoiseTune, KeyGlossToneDecay, KeyGlossNoiseDecay, KeyGlossTailDecay,
		KeyGlossToneLevel, KeyGlossNoiseLevel, KeyGlossWires, KeyGlossSnap,
		KeyGlossMetalTune, KeyGlossAttackDecay, KeyGlossMetalLevel, KeyGlossSizzleLevel, KeyGlossSizzleDecay,
		KeyGlossPluck, KeyGlossPick, KeyGlossFade, KeyGlossOvertone, KeyGlossPitchPunch, KeyGlossPunch,
		KeyGlossDrive, KeyGlossGain, KeyGlossSaturation, KeyGlossSat, KeyGlossDistortion,
		KeyGlossSlope, KeyGlossSlopes, KeyGlossPre, KeyGlossPost, KeyGlossPeak, KeyGlossRMS,
		KeyGlossClip, KeyGlossClips, KeyGlossHeadroom, KeyGlossLUFS, KeyGlossK20,
		KeyGlossOverlay, KeyGlossSplit, KeyGlossDiff, KeyGlossOvr, KeyGlossSpl, KeyGlossDif,
		KeyGlossAG, KeyGlossAutoGain,
		KeyGlossOut, KeyGlossDelay, KeyGlossReverb, KeyGlossSend,
		KeyGlossPlay, KeyGlossStop, KeyGlossPause,
		KeyCapMove, KeyCapMoveNode, KeyCapConnect, KeyCapYourSound,
		KeyCapVol, KeyCapPitch, KeyCapPct, KeyCapDur, KeyCapSearch,
		KeyCapHeadroom, KeyCapLoudest, KeyCapFrozen, KeyCapBypassed, KeyCapSamplerTitle,
		KeyMaster, KeyNoNotifications, KeyNodeLogic, KeyNodeGroove,
		KeyNoRowSelected, KeyAuto, KeyPre,
		KeyNodeTitle, KeyNodeSecVolume, KeyNodeSecDuration, KeyNodeSecLogic,
		KeyNodeSecGroove, KeyNodeSecAudible, KeyNodeSecMove,
		KeyLogicNone, KeyLogicEveryN, KeyLogicSkipN, KeyLogicProbability,
		KeyLogicIfPrevSkipped, KeyLogicIfPrevTriggered,
		KeyLogicShortEveryN, KeyLogicShortSkipN, KeyLogicShortProb,
		KeyLogicShortPrevSkip, KeyLogicShortPrevTrig,
		KeyGrooveDelay, KeyGrooveRush, KeyAudSilent, KeyAudMuted,
		KeyNotifInvalidName, KeyNotifRenamedInstrument, KeyNotifErrLoadJSON, KeyNotifImported,
		KeyNotifRecordingFailed, KeyNotifSavingRecording, KeyNotifCannotStartRecording,
		KeyNotifRecordingSaveFailed, KeyNotifRecordingSaved, KeyNotifImportCanceled,
		KeyNotifErrLoadWAV, KeyNotifSelectedWAV, KeyNotifCannotConnectInvisible,
		KeyNotifOnlyPerpendicular, KeyNotifInstNameEmpty, KeyNotifUpdatedWAVInst,
		KeyNotifLoadedWAVInst, KeyNotifInvalidBPM,
	}
}
