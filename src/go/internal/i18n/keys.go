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

	// Orientation (mobile landscape is unsupported — rotate-to-portrait notice)
	KeyOrientationTitle Key = "orientation.title"
	KeyOrientationBody  Key = "orientation.body"

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

	// Menus (context menu + overflow menu + instrument-picker chrome)
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
	KeyMenuCategories   Key = "menu.categories"
	KeyMenuFavorites    Key = "menu.favorites"

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
	KeyGlossVoice         Key = "gloss.voice"
	KeyGlossOsc           Key = "gloss.osc"
	KeyGlossPitch         Key = "gloss.pitch"
	KeyGlossWaveform      Key = "gloss.waveform"
	KeyGlossGenerator     Key = "gloss.generator"
	KeyGlossTune          Key = "gloss.tune"
	KeyGlossGlide         Key = "gloss.glide"
	KeyGlossBend          Key = "gloss.bend"
	KeyGlossDetune        Key = "gloss.detune"
	KeyGlossEnvelope      Key = "gloss.envelope"
	KeyGlossAttack        Key = "gloss.attack"
	KeyGlossDecay         Key = "gloss.decay"
	KeyGlossSustain       Key = "gloss.sustain"
	KeyGlossRelease       Key = "gloss.release"
	KeyGlossTone          Key = "gloss.tone"
	KeyGlossCutoff        Key = "gloss.cutoff"
	KeyGlossFilter        Key = "gloss.filter"
	KeyGlossReso          Key = "gloss.reso"
	KeyGlossResonance     Key = "gloss.resonance"
	KeyGlossBody          Key = "gloss.body"
	KeyGlossBasePitch     Key = "gloss.base_pitch"
	KeyGlossPitchSweep    Key = "gloss.pitch_sweep"
	KeyGlossSweepTime     Key = "gloss.sweep_time"
	KeyGlossOpRatio       Key = "gloss.op_ratio"
	KeyGlossOpFMDepth     Key = "gloss.op_fm_depth"
	KeyGlossOpDecay       Key = "gloss.op_decay"
	KeyGlossFundamental   Key = "gloss.fundamental"
	KeyGlossSweepSpeed    Key = "gloss.sweep_speed"
	KeyGlossBoomDecay     Key = "gloss.boom_decay"
	KeyGlossBodyDecay     Key = "gloss.body_decay"
	KeyGloss2ndHarmonic   Key = "gloss.2nd_harmonic"
	KeyGloss3rdHarmonic   Key = "gloss.3rd_harmonic"
	KeyGloss4thHarmonic   Key = "gloss.4th_harmonic"
	KeyGlossClick         Key = "gloss.click"
	KeyGlossThud          Key = "gloss.thud"
	KeyGlossRingDecay     Key = "gloss.ring_decay"
	KeyGlossOvertone1     Key = "gloss.overtone1"
	KeyGlossOvertone2     Key = "gloss.overtone2"
	KeyGlossStick         Key = "gloss.stick"
	KeyGlossRoom          Key = "gloss.room"
	KeyGlossTone2         Key = "gloss.tone2"
	KeyGlossNoiseTune     Key = "gloss.noise_tune"
	KeyGlossToneDecay     Key = "gloss.tone_decay"
	KeyGlossNoiseDecay    Key = "gloss.noise_decay"
	KeyGlossTailDecay     Key = "gloss.tail_decay"
	KeyGlossToneLevel     Key = "gloss.tone_level"
	KeyGlossNoiseLevel    Key = "gloss.noise_level"
	KeyGlossWires         Key = "gloss.wires"
	KeyGlossSnap          Key = "gloss.snap"
	KeyGlossMetalTune     Key = "gloss.metal_tune"
	KeyGlossAttackDecay   Key = "gloss.attack_decay"
	KeyGlossMetalLevel    Key = "gloss.metal_level"
	KeyGlossSizzleLevel   Key = "gloss.sizzle_level"
	KeyGlossSizzleDecay   Key = "gloss.sizzle_decay"
	KeyGlossPluck         Key = "gloss.pluck"
	KeyGlossPick          Key = "gloss.pick"
	KeyGlossFade          Key = "gloss.fade"
	KeyGlossOvertone      Key = "gloss.overtone"
	KeyGlossPitchPunch    Key = "gloss.pitch_punch"
	KeyGlossPunch         Key = "gloss.punch"
	KeyGlossDrive         Key = "gloss.drive"
	KeyGlossGain          Key = "gloss.gain"
	KeyGlossSaturation    Key = "gloss.saturation"
	KeyGlossSat           Key = "gloss.sat"
	KeyGlossDistortion    Key = "gloss.distortion"
	KeyGlossSlope         Key = "gloss.slope"
	KeyGlossSlopes        Key = "gloss.slopes"
	KeyGlossPre           Key = "gloss.pre"
	KeyGlossPost          Key = "gloss.post"
	KeyGlossPeak          Key = "gloss.peak"
	KeyGlossRMS           Key = "gloss.rms"
	KeyGlossClip          Key = "gloss.clip"
	KeyGlossClips         Key = "gloss.clips"
	KeyGlossHeadroom      Key = "gloss.headroom"
	KeyGlossLUFS          Key = "gloss.lufs"
	KeyGlossK20           Key = "gloss.k20"
	KeyGlossOverlay       Key = "gloss.overlay"
	KeyGlossSplit         Key = "gloss.split"
	KeyGlossDiff          Key = "gloss.diff"
	KeyGlossOvr           Key = "gloss.ovr"
	KeyGlossSpl           Key = "gloss.spl"
	KeyGlossDif           Key = "gloss.dif"
	KeyGlossAG            Key = "gloss.ag"
	KeyGlossAutoGain      Key = "gloss.auto_gain"
	KeyGlossOut           Key = "gloss.out"
	KeyGlossDelay         Key = "gloss.delay"
	KeyGlossReverb        Key = "gloss.reverb"
	KeyGlossSend          Key = "gloss.send"
	KeyGlossPlay          Key = "gloss.play"
	KeyGlossStop          Key = "gloss.stop"
	KeyGlossPause         Key = "gloss.pause"
	KeySynthAdvanced      Key = "synth.advanced"
	KeyGlossSaxBlow       Key = "gloss.sax_blow"
	KeyGlossSaxReed       Key = "gloss.sax_reed"
	KeyGlossSaxReedStiff  Key = "gloss.sax_reed_stiff"
	KeyGlossSaxBell       Key = "gloss.sax_bell"
	KeyGlossSaxBreath     Key = "gloss.sax_breath"
	KeyGlossSaxBright     Key = "gloss.sax_bright"
	KeyGlossBowPos        Key = "gloss.bow_pos"
	KeyGlossBowPress      Key = "gloss.bow_press"
	KeyGlossBowSpeed      Key = "gloss.bow_speed"
	KeyGlossBowBright     Key = "gloss.bow_bright"
	KeyGlossOscillator    Key = "gloss.oscillator"
	KeyGlossOctave        Key = "gloss.octave"
	KeyGlossAlgorithm     Key = "gloss.algorithm"
	KeyGlossOp1Ratio      Key = "gloss.op1_ratio"
	KeyGlossOp2Ratio      Key = "gloss.op2_ratio"
	KeyGlossOp3Ratio      Key = "gloss.op3_ratio"
	KeyGlossOp4Ratio      Key = "gloss.op4_ratio"
	KeyGlossOp1Depth      Key = "gloss.op1_depth"
	KeyGlossOp2Depth      Key = "gloss.op2_depth"
	KeyGlossOp3Depth      Key = "gloss.op3_depth"
	KeyGlossOp4Depth      Key = "gloss.op4_depth"
	KeyGlossOp1Level      Key = "gloss.op1_level"
	KeyGlossOp2Level      Key = "gloss.op2_level"
	KeyGlossOp3Level      Key = "gloss.op3_level"
	KeyGlossOp4Level      Key = "gloss.op4_level"
	KeyGlossCurve         Key = "gloss.curve"
	KeyGlossFilterEnv     Key = "gloss.filter_env"
	KeyGlossEnvAmount     Key = "gloss.env_amount"
	KeyGlossEnvRise       Key = "gloss.env_rise"
	KeyGlossEnvDecay      Key = "gloss.env_decay"
	KeyGlossLfo           Key = "gloss.lfo"
	KeyGlossRate          Key = "gloss.rate"
	KeyGlossDepth         Key = "gloss.depth"
	KeyGlossLfoTarget     Key = "gloss.lfo_target"
	KeyGlossVibratoDelay  Key = "gloss.vibrato_delay"
	KeyGlossPitchEnv      Key = "gloss.pitch_env"
	KeyGlossSweep         Key = "gloss.sweep"
	KeyGlossBurst         Key = "gloss.burst"
	KeyGlossSharpness     Key = "gloss.sharpness"
	KeyGlossHit1Time      Key = "gloss.hit1_time"
	KeyGlossHit2Time      Key = "gloss.hit2_time"
	KeyGlossHit3Time      Key = "gloss.hit3_time"
	KeyGlossHit4Time      Key = "gloss.hit4_time"
	KeyGlossHit1Level     Key = "gloss.hit1_level"
	KeyGlossHit2Level     Key = "gloss.hit2_level"
	KeyGlossHit3Level     Key = "gloss.hit3_level"
	KeyGlossHit4Level     Key = "gloss.hit4_level"
	KeyGlossVoices        Key = "gloss.voices"
	KeyGlossMix           Key = "gloss.mix"
	KeyGlossVoiceDrift    Key = "gloss.voice_drift"
	KeyGlossDriftDepth    Key = "gloss.drift_depth"
	KeyGlossKick          Key = "gloss.kick"
	KeyGlossVariant       Key = "gloss.variant"
	KeyGlossHarmonicDecay Key = "gloss.harmonic_decay"
	KeyGlossDropSpeed     Key = "gloss.drop_speed"
	KeyGlossPitchDrop     Key = "gloss.pitch_drop"
	KeyGlossModeSplit     Key = "gloss.mode_split"
	KeyGlossModeBody      Key = "gloss.mode_body"
	KeyGlossModeRing      Key = "gloss.mode_ring"
	KeyGlossTail          Key = "gloss.tail"
	KeyGlossFm            Key = "gloss.fm"
	KeyGlossEnv           Key = "gloss.env"
	KeyGlossDull          Key = "gloss.dull"
	KeyGlossBright        Key = "gloss.bright"
	// Phase-15 voice/choir knob glosses (Task 8): FORMANT + ENSEMBLE + RESONATOR.
	KeyGlossFormant      Key = "gloss.formant"
	KeyGlossVowel        Key = "gloss.vowel"
	KeyGlossVoiceType    Key = "gloss.voice_type"
	KeyGlossBreath       Key = "gloss.breath"
	KeyGlossShine        Key = "gloss.shine"
	KeyGlossHeadSize     Key = "gloss.head_size"
	KeyGlossMorphSpeed   Key = "gloss.morph_speed"
	KeyGlossMorphTo      Key = "gloss.morph_to"
	KeyGlossScatter      Key = "gloss.scatter"
	KeyGlossVibratoRate  Key = "gloss.vibrato_rate"
	KeyGlossVibratoDepth Key = "gloss.vibrato_depth"
	KeyGlossHumanize     Key = "gloss.humanize"
	KeyGlossBodyModel    Key = "gloss.body_model"
	KeyGlossBodyMix      Key = "gloss.body_mix"
	KeyGlossBowDynamics  Key = "gloss.bow_dynamics"
	// Phase-16 voice-realism knob glosses: ens_jitter (Roughness) + formant_dry
	// (Dry Blend).
	KeyGlossRoughness Key = "gloss.roughness"
	KeyGlossDryBlend  Key = "gloss.dry_blend"

	// Caption labels (drawn each frame near controls / overlays)
	KeyCapMove         Key = "cap.move"
	KeyCapMoveNode     Key = "cap.move_node"
	KeyCapCopy         Key = "cap.copy"
	KeyCapConnect      Key = "cap.connect"
	KeyCapYourSound    Key = "cap.your_sound"
	KeyCapUpClose      Key = "cap.up_close"
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
	KeyCapStageOff     Key = "cap.stage_off"
	KeyCapStarts       Key = "cap.starts"
	KeyCapFades        Key = "cap.fades"
	KeyCapBeforeAfter  Key = "cap.before_after"
	// KeyCapBreak labels the destructive "Break" button in the break-circuit
	// confirmation dialog (grid_move_break_confirm.go).
	KeyCapBreak Key = "cap.break"

	// Menu empty states (shared list-menu anatomy — see DESIGN.md "Menu anatomy")
	KeyMenuNoMatches       Key = "menu.no_matches"
	KeyMenuNoMatchesHint   Key = "menu.no_matches_hint"
	KeyMenuNoFavorites     Key = "menu.no_favorites"
	KeyMenuNoFavoritesHint Key = "menu.no_favorites_hint"
	KeyFXEmptyHint         Key = "fx.empty_hint"

	// Final i18n cleanup pass — remaining user-facing chrome words.
	KeyMaster          Key = "mix.master"
	KeyNoNotifications Key = "notif.empty"
	KeyNodeLogic       Key = "node.logic"
	KeyNodeGroove      Key = "node.groove"
	KeyNoRowSelected   Key = "synth.no_row"
	KeyAuto            Key = "btn.auto"
	KeyPre             Key = "btn.pre"
	KeyAnalyzerIdle    Key = "analyzer.idle_hint"

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
	KeyNotifUndo                   Key = "notif.undo"
	KeyNotifRedo                   Key = "notif.redo"
	KeyNotifUndoDetail             Key = "notif.undo_detail"
	KeyNotifRedoDetail             Key = "notif.redo_detail"

	// Parameter labels used by undo/redo detail notifications (and reusable by
	// rendering) for instrument knobs that lack a dedicated label elsewhere.
	KeyParamPan        Key = "param.pan"
	KeyParamDelaySend  Key = "param.delay_send"
	KeyParamReverbSend Key = "param.reverb_send"

	// KeyNotifImportedNamed names the loaded project/template + node/row counts;
	// KeyNotifInvalidBPMDetail names the rejected value and the valid range.
	KeyNotifImportedNamed    Key = "notif.imported_named"
	KeyNotifInvalidBPMDetail Key = "notif.invalid_bpm_detail"

	// Undo/redo action labels — the %s substituted into notif.undo / notif.redo.
	// One per undoable hooks.Kind (mapped in internal/ui; a drift test asserts
	// every undoable action has a key here).
	KeyActionChangeBPM         Key = "action.change_bpm"
	KeyActionChangeSubdivision Key = "action.change_subdivision"
	KeyActionAddNode           Key = "action.add_node"
	KeyActionDeleteNode        Key = "action.delete_node"
	KeyActionPasteNode         Key = "action.paste_node"
	KeyActionReplaceNode       Key = "action.replace_node"
	KeyActionMoveNode          Key = "action.move_node"
	KeyActionChangeNodeType    Key = "action.change_node_type"
	KeyActionEditNode          Key = "action.edit_node"
	KeyActionSetStartNode      Key = "action.set_start_node"
	KeyActionAddEdge           Key = "action.add_edge"
	KeyActionDeleteEdge        Key = "action.delete_edge"
	KeyActionAddRow            Key = "action.add_row"
	KeyActionDeleteRow         Key = "action.delete_row"
	KeyActionChangeInstrument  Key = "action.change_instrument"
	KeyActionSetMasterVolume   Key = "action.set_master_volume"
	KeyActionAdjustEQ          Key = "action.adjust_eq"
	KeyActionAddEffect         Key = "action.add_effect"
	KeyActionRemoveEffect      Key = "action.remove_effect"
	KeyActionAdjustEffect      Key = "action.adjust_effect"
	KeyActionSetRowVolume      Key = "action.set_row_volume"
	KeyActionReorderEffect     Key = "action.reorder_effect"
	KeyActionToggleEffect      Key = "action.toggle_effect"
	KeyActionEditSynth         Key = "action.edit_synth"
	KeyActionResetSynth        Key = "action.reset_synth"
	KeyActionRecolorRow        Key = "action.recolor_row"
	KeyActionToggleEQFilter    Key = "action.toggle_eq_filter"
	KeyActionRenameInstrument  Key = "action.rename_instrument"
	KeyActionEditSample        Key = "action.edit_sample"
	KeyActionCreateGroup       Key = "action.create_group"
	KeyActionEditGroup         Key = "action.edit_group"
	KeyActionDeleteGroup       Key = "action.delete_group"

	// Synth-tab pipeline stage chip labels (VOICE · OSC · FM · PITCH · LFO ·
	// BURST · ENVELOPE · FILTER · POST, plus the TONE stage). These are the
	// localized DISPLAY labels; the canonical English label backing hit-area
	// tags + the selectSynthSection JS lookup stays in internal/ui.
	KeySynthStageVoice     Key = "synth.stage.voice"
	KeySynthStageOsc       Key = "synth.stage.osc"
	KeySynthStageFM        Key = "synth.stage.fm"
	KeySynthStagePitch     Key = "synth.stage.pitch"
	KeySynthStageLFO       Key = "synth.stage.lfo"
	KeySynthStageBurst     Key = "synth.stage.burst"
	KeySynthStageEnvelope  Key = "synth.stage.envelope"
	KeySynthStageFilter    Key = "synth.stage.filter"
	KeySynthStageFilterEnv Key = "synth.stage.filterenv"
	KeySynthStagePost      Key = "synth.stage.post"
	KeySynthStageTone      Key = "synth.stage.tone"
	// Phase-15 voice/choir stages (Task 8).
	KeySynthStageEnsemble  Key = "synth.stage.ensemble"
	KeySynthStageFormant   Key = "synth.stage.formant"
	KeySynthStageResonator Key = "synth.stage.resonator"
	// Designed short chip labels for the stages whose full names overflow a
	// narrow pipeline chip — a fitted fallback, never a mid-word ellipsis.
	KeySynthStageEnvelopeShort  Key = "synth.stage.envelope.short"
	KeySynthStageFilterEnvShort Key = "synth.stage.filterenv.short"
	KeySynthStageEnsembleShort  Key = "synth.stage.ensemble.short"
	KeySynthStageFormantShort   Key = "synth.stage.formant.short"
	KeySynthStageResonatorShort Key = "synth.stage.resonator.short"

	// Synth-tab stage subtitles — the kid-friendly "what makes this sound"
	// descriptions rendered under each stage's jargon title.
	KeySynthStageVoiceSub     Key = "synth.stage.voice.sub"
	KeySynthStageOscSub       Key = "synth.stage.osc.sub"
	KeySynthStageFMSub        Key = "synth.stage.fm.sub"
	KeySynthStagePitchSub     Key = "synth.stage.pitch.sub"
	KeySynthStageLFOSub       Key = "synth.stage.lfo.sub"
	KeySynthStageBurstSub     Key = "synth.stage.burst.sub"
	KeySynthStageEnvelopeSub  Key = "synth.stage.envelope.sub"
	KeySynthStageFilterSub    Key = "synth.stage.filter.sub"
	KeySynthStageFilterEnvSub Key = "synth.stage.filterenv.sub"
	KeySynthStagePostSub      Key = "synth.stage.post.sub"
	KeySynthStageToneSub      Key = "synth.stage.tone.sub"
	// Phase-15 voice/choir stage subtitles (Task 8).
	KeySynthStageEnsembleSub  Key = "synth.stage.ensemble.sub"
	KeySynthStageFormantSub   Key = "synth.stage.formant.sub"
	KeySynthStageResonatorSub Key = "synth.stage.resonator.sub"

	// Synth-tab KNOB labels — the curated knob captions (one key per distinct
	// caption value; several param ids share a key, e.g. every "Wave" knob). The
	// canonical English caption backing the editor round-trip + the param-id ABI
	// stays in internal/ui; these are the localized DISPLAY labels only.
	KeySynthControlTune          Key = "synth.knob.tune"
	KeySynthControlDecayMult     Key = "synth.knob.decay-mult"
	KeySynthControlTone          Key = "synth.knob.tone"
	KeySynthControlDrive         Key = "synth.knob.drive"
	KeySynthControlBody          Key = "synth.knob.body"
	KeySynthControlBright        Key = "synth.knob.bright"
	KeySynthControlGain          Key = "synth.knob.gain"
	KeySynthControlWave          Key = "synth.knob.wave"
	KeySynthControlDetune        Key = "synth.knob.detune"
	KeySynthControlOctave        Key = "synth.knob.octave"
	KeySynthControlAttack        Key = "synth.knob.attack"
	KeySynthControlDecay         Key = "synth.knob.decay"
	KeySynthControlSustain       Key = "synth.knob.sustain"
	KeySynthControlRelease       Key = "synth.knob.release"
	KeySynthControlCurve         Key = "synth.knob.curve"
	KeySynthControlType          Key = "synth.knob.type"
	KeySynthControlCutoff        Key = "synth.knob.cutoff"
	KeySynthControlResonance     Key = "synth.knob.resonance"
	KeySynthControlAlgorithm     Key = "synth.knob.algorithm"
	KeySynthControlBaseFreq      Key = "synth.knob.base-freq"
	KeySynthControlPitchEnv      Key = "synth.knob.pitch-env"
	KeySynthControlPitchEnvDecay Key = "synth.knob.pitch-env-decay"
	KeySynthControlAmount        Key = "synth.knob.amount"
	KeySynthControlRate          Key = "synth.knob.rate"
	KeySynthControlDepth         Key = "synth.knob.depth"
	KeySynthControlSharpness     Key = "synth.knob.sharpness"
	KeySynthControlDraws         Key = "synth.knob.draws"
	KeySynthControlPrelude       Key = "synth.knob.prelude"
	KeySynthControlSeed          Key = "synth.knob.seed"
	KeySynthControlPitch         Key = "synth.knob.pitch"
	KeySynthControlClick         Key = "synth.knob.click"
	KeySynthControlNoise         Key = "synth.knob.noise"

	// Synth-tab knob NOUNS — back the composed FM-operator and burst-hit
	// captions ("Op1 Ratio", "Hit 1 Level"). The universal "OpN"/"Hit N" prefix
	// stays English; only the noun is keyed. Depth/Decay reuse the knob keys.
	KeySynthNounRatio Key = "synth.noun.ratio"
	KeySynthNounLevel Key = "synth.noun.level"
	KeySynthNounTime  Key = "synth.noun.time"
	KeySynthNounHit   Key = "synth.noun.hit"

	// Synth-tab value ENUMS — the knob VALUE that reads "Triangle", "Low-pass",
	// "Exponential", … Canonical strings stay the storage form (audio.ParamDef
	// .Enum); these localize the display + editor prefill. "FM" and "2-op" are
	// universal and intentionally keyless (they pass through unchanged).
	KeySynthEnumSine        Key = "synth.enum.wave-sine"
	KeySynthEnumSaw         Key = "synth.enum.wave-saw"
	KeySynthEnumSquare      Key = "synth.enum.wave-square"
	KeySynthEnumTriangle    Key = "synth.enum.wave-triangle"
	KeySynthEnumNoiseWhite  Key = "synth.enum.wave-noise-white"
	KeySynthEnumNoisePink   Key = "synth.enum.wave-noise-pink"
	KeySynthEnumParallel    Key = "synth.enum.algo-parallel"
	KeySynthEnum3opChain    Key = "synth.enum.algo-3op-chain"
	KeySynthEnum4opStack    Key = "synth.enum.algo-4op-stack"
	KeySynthEnumLinear      Key = "synth.enum.curve-linear"
	KeySynthEnumExponential Key = "synth.enum.curve-exponential"
	KeySynthEnumLowPass     Key = "synth.enum.filter-lowpass"
	KeySynthEnumHighPass    Key = "synth.enum.filter-highpass"
	KeySynthEnumBandPass    Key = "synth.enum.filter-bandpass"
	KeySynthEnumOn          Key = "synth.enum.on"
	KeySynthEnumOff         Key = "synth.enum.off"

	// Spectrum tab — ISO band brackets (Bass/Mids/Treble). Frequency ticks
	// ("31", "1k", …) stay numeric and are NOT keyed.
	KeySpectrumBandBass   Key = "spectrum.band.bass"
	KeySpectrumBandMids   Key = "spectrum.band.mids"
	KeySpectrumBandTreble Key = "spectrum.band.treble"

	// Levels tab — translatable labels + the compact readout format string.
	// RMS / LUFS-S / K-20 are international standards and stay literal.
	KeyLevelsPeak       Key = "levels.peak"
	KeyLevelsClips      Key = "levels.clips"
	KeyLevelsClear      Key = "levels.clear"
	KeyLevelsReadoutFmt Key = "levels.readout_fmt"

	// Chain tab — display-mode pills + auto-gain/fit. DIF is identical in both
	// locales but routed for consistency. Stage names keep their common DAW
	// English form (Synth/AntiPop/FX/EQ/Bus/Master) but route through i18n.
	KeyChainOverlay      Key = "chain.overlay"
	KeyChainSplit        Key = "chain.split"
	KeyChainDiff         Key = "chain.diff"
	KeyChainAutoGain     Key = "chain.auto_gain"
	KeyChainFit          Key = "chain.fit"
	KeyChainStageSynth   Key = "chain.stage.synth"
	KeyChainStageAntiPop Key = "chain.stage.antipop"
	KeyChainStageFX      Key = "chain.stage.fx"
	KeyChainStageEQ      Key = "chain.stage.eq"
	KeyChainStageBus     Key = "chain.stage.bus"

	// Sampler tab — knob group labels, knob caption prefixes, knob glosses.
	// Units (%, st, cents, dB) stay literal.
	KeySamplerGroupTrim  Key = "sampler.group.trim"
	KeySamplerGroupTune  Key = "sampler.group.tune"
	KeySamplerGroupLevel Key = "sampler.group.level"
	KeySamplerKnobStart  Key = "sampler.knob.start"
	KeySamplerKnobEnd    Key = "sampler.knob.end"
	KeySamplerKnobPitch  Key = "sampler.knob.pitch"
	KeySamplerKnobFine   Key = "sampler.knob.fine"
	KeySamplerKnobGain   Key = "sampler.knob.gain"
	KeySamplerGlossStart Key = "sampler.gloss.start"
	KeySamplerGlossEnd   Key = "sampler.gloss.end"
	KeySamplerGlossPitch Key = "sampler.gloss.pitch"
	KeySamplerGlossFine  Key = "sampler.gloss.fine"
	KeySamplerGlossGain  Key = "sampler.gloss.gain"

	// Translatable value unit (cents). Symbol units (%, dB, Hz, ms, s, st, ×)
	// are international and stay literal; cents reads as "ct" (centésimas) in es.
	KeyUnitCents Key = "unit.cents"

	// Sampler length / sample-rate readout. The "of" joiner is English prose.
	KeySamplerMetaFmt Key = "sampler.meta_fmt"

	// Levels long-press tooltips (one per aggregate icon).
	KeyLevelsTipHeadroom     Key = "levels.tip.headroom"
	KeyLevelsTipHeadroomClip Key = "levels.tip.headroom_clip"
	KeyLevelsTipClips        Key = "levels.tip.clips"
	KeyLevelsTipLoudest      Key = "levels.tip.loudest"
	KeyLevelsTipLoudestNone  Key = "levels.tip.loudest_none"

	// GroupMenu (anchored panel editing a node group) — title, rule section
	// label, and the three rule-editable params.
	KeyGroupMenuTitle Key = "group.menu.title"
	KeyGroupMembers   Key = "group.menu.members"
	KeyGroupRule      Key = "group.menu.rule"
	KeyGroupRuleOn    Key = "group.menu.rule.on"
	KeyGroupRuleOff   Key = "group.menu.rule.off"
	KeyGroupEveryN    Key = "group.menu.everyn"
	// KeyGroupEveryNShort is a compact form of KeyGroupEveryN ("Every %d")
	// for the combined delta/everyN half-row pill, which is too narrow for
	// the full "Every %d rounds" sentence (see the Every-N row layout fix in
	// game_group_menu.go).
	KeyGroupEveryNShort   Key = "group.menu.everyn_short"
	KeyGroupDelete        Key = "group.menu.delete"
	KeyGroupSave          Key = "group.menu.save"
	KeyGroupParamPitch    Key = "group.menu.param.pitch"
	KeyGroupParamVolume   Key = "group.menu.param.volume"
	KeyGroupParamDuration Key = "group.menu.param.duration"
	// KeyGroupAddTo labels the node-sidebar row that opens a dropdown of
	// groups the selected node is NOT yet a member of (see
	// game_node_sidebar.go's "grpadd" row / groupAddDropdownOpen).
	KeyGroupAddTo Key = "group.menu.add_to"
	// KeyGroupCreateNew labels the node-sidebar's always-present "grpnew" row
	// (task-sidebar-create brief): turns the selected node into a brand-new
	// PROVISIONAL single-member group and opens its GroupMenu (see
	// NodeSidebar.createNewGroup).
	KeyGroupCreateNew Key = "group.menu.create_new"
	// KeyGroupRemoveFrom labels the node-sidebar row that opens a dropdown of
	// groups the selected node IS currently a member of (see
	// game_node_sidebar.go's "grpremove" row / groupRemoveDropdownOpen).
	// Picking one removes that membership (task-remove-selector brief).
	KeyGroupRemoveFrom Key = "group.menu.remove_from"

	// KeyBannerPasting is the copy-mode grid banner shown while a copied node
	// is armed and awaiting placement (see grid_pane_draw.go's
	// drawGridMoveMode, GridModeCopy case).
	KeyBannerPasting Key = "banner.pasting"
	// KeyBannerMoving / KeyBannerConnectFrom are the move / connect armed-mode
	// grid banners; both take the subject's (i,j) grid coordinate as two %d
	// verbs. KeyBannerConnecting is an armed-mode banner for connecting to a
	// target node; also takes (i,j) as two %d verbs. KeyBannerEscHint is the
	// desktop-only suffix appended to every armed-mode banner when
	// Profile().ShowEscHint (note its leading space). All render through
	// gridModeBannerText (grid_mode_banner.go).
	KeyBannerMoving      Key = "banner.moving"
	KeyBannerConnectFrom Key = "banner.connect_from"
	KeyBannerConnecting  Key = "banner.connecting"
	KeyBannerEscHint     Key = "banner.esc_hint"
	KeyEscToCancel       Key = "banner.esc_to_cancel"

	// Confirm dialog titles
	KeyConfirmReplaceNode Key = "confirm.replace_node"

	// Instrument-import collision dialog (Replace vs Keep both).
	KeyImportCollisionTitle Key = "import.collision.title"
	KeyCapReplace           Key = "cap.replace"
	KeyCapKeepBoth          Key = "cap.keep_both"

	// Caption labels (drawn each frame near controls / overlays) — continued below

	// KeyCapOverwrite is the caption shown when pasting over an occupied node.
	KeyCapOverwrite Key = "cap.overwrite"

	// KeyBreakCircuitTitle titles the "Break the circuit?" confirmation
	// dialog shown when an armed move would disconnect the circuit
	// unhealably (grid_move_break_confirm.go).
	KeyBreakCircuitTitle Key = "confirm.break_circuit"

	// Logs Navigation Panel (logs_component.go) — searchable/filterable/paginated
	// view of the event stream + notifications, opened from the grid Logs icon.
	KeyLogs              Key = "logs.title"
	KeyLogsSearchName    Key = "logs.search_name"
	KeyLogsTimeAt        Key = "logs.time_at"
	KeyLogsTimeRange     Key = "logs.time_range"
	KeyLogsFrom          Key = "logs.from"
	KeyLogsTo            Key = "logs.to"
	KeyLogsAllCategories Key = "logs.all_categories"
	KeyLogsPage          Key = "logs.page" // "Page %d / %d"
	KeyLogsGoToPage      Key = "logs.go_to_page"
	KeyLogsClearHistory  Key = "logs.clear_history"
	KeyNoLogs            Key = "logs.empty"

	// Logs date/time picker (logs_datetime_picker.go).
	KeyLogsPickTitle Key = "logs.pick_title"
	KeyLogsSet       Key = "logs.set"
	KeyLogsClear     Key = "logs.clear"
	KeyLogsAnyTime   Key = "logs.any_time"

	// Calendar month names (cal.month.N) + weekday abbreviations (cal.wd.*).
	KeyMonth1     Key = "cal.month.1"
	KeyMonth2     Key = "cal.month.2"
	KeyMonth3     Key = "cal.month.3"
	KeyMonth4     Key = "cal.month.4"
	KeyMonth5     Key = "cal.month.5"
	KeyMonth6     Key = "cal.month.6"
	KeyMonth7     Key = "cal.month.7"
	KeyMonth8     Key = "cal.month.8"
	KeyMonth9     Key = "cal.month.9"
	KeyMonth10    Key = "cal.month.10"
	KeyMonth11    Key = "cal.month.11"
	KeyMonth12    Key = "cal.month.12"
	KeyWeekdaySun Key = "cal.wd.sun"
	KeyWeekdayMon Key = "cal.wd.mon"
	KeyWeekdayTue Key = "cal.wd.tue"
	KeyWeekdayWed Key = "cal.wd.wed"
	KeyWeekdayThu Key = "cal.wd.thu"
	KeyWeekdayFri Key = "cal.wd.fri"
	KeyWeekdaySat Key = "cal.wd.sat"
)

// AllKeys lists every declared Key. The completeness test asserts this set is
// exactly the key set of every locale catalog.
func AllKeys() []Key {
	return []Key{
		KeyTransportPlay, KeyTransportPause, KeyTransportRec,
		KeyOrientationTitle, KeyOrientationBody,
		KeySettingsTitle, KeySettingsLanguage, KeySettingsShortcuts, KeySettingsDesktopOnly,
		KeyLangEnglish, KeyLangSpanish,
		KeyShortcutPlayPause, KeyShortcutCancel, KeyShortcutPan, KeyShortcutZoom,
		KeyShortcutResetZoom, KeyShortcutBPM, KeyShortcutTabs, KeyShortcutHelp,
		KeyMenuRename, KeyMenuColor, KeyMenuOrigin, KeyMenuDelete,
		KeyMenuFile, KeyMenuUpload, KeyMenuImport, KeyMenuExport,
		KeyMenuLoadTemplate, KeyMenuTemplates, KeyMenuBack,
		KeyMenuCategories, KeyMenuFavorites,
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
		KeySynthAdvanced,
		KeyGlossSaxBlow, KeyGlossSaxReed, KeyGlossSaxReedStiff, KeyGlossSaxBell, KeyGlossSaxBreath, KeyGlossSaxBright, KeyGlossBowPos, KeyGlossBowPress, KeyGlossBowSpeed, KeyGlossBowBright,
		KeyGlossOscillator, KeyGlossOctave, KeyGlossAlgorithm, KeyGlossOp1Ratio, KeyGlossOp2Ratio, KeyGlossOp3Ratio, KeyGlossOp4Ratio, KeyGlossOp1Depth, KeyGlossOp2Depth, KeyGlossOp3Depth, KeyGlossOp4Depth, KeyGlossOp1Level, KeyGlossOp2Level, KeyGlossOp3Level, KeyGlossOp4Level, KeyGlossCurve, KeyGlossFilterEnv, KeyGlossEnvAmount, KeyGlossEnvRise, KeyGlossEnvDecay, KeyGlossLfo, KeyGlossRate, KeyGlossDepth, KeyGlossLfoTarget, KeyGlossVibratoDelay, KeyGlossPitchEnv, KeyGlossSweep, KeyGlossBurst, KeyGlossSharpness, KeyGlossHit1Time, KeyGlossHit2Time, KeyGlossHit3Time, KeyGlossHit4Time, KeyGlossHit1Level, KeyGlossHit2Level, KeyGlossHit3Level, KeyGlossHit4Level, KeyGlossVoices, KeyGlossMix, KeyGlossVoiceDrift, KeyGlossDriftDepth, KeyGlossKick, KeyGlossVariant, KeyGlossHarmonicDecay, KeyGlossDropSpeed, KeyGlossPitchDrop, KeyGlossModeSplit, KeyGlossModeBody, KeyGlossModeRing, KeyGlossTail, KeyGlossFm, KeyGlossEnv,
		KeyGlossDull, KeyGlossBright,
		KeyGlossFormant,
		KeyGlossVowel, KeyGlossVoiceType, KeyGlossBreath, KeyGlossShine, KeyGlossHeadSize,
		KeyGlossMorphSpeed, KeyGlossMorphTo, KeyGlossScatter, KeyGlossVibratoRate,
		KeyGlossVibratoDepth, KeyGlossHumanize, KeyGlossBodyModel, KeyGlossBodyMix, KeyGlossBowDynamics,
		KeyGlossRoughness, KeyGlossDryBlend,
		KeyCapMove, KeyCapMoveNode, KeyCapCopy, KeyCapConnect, KeyCapYourSound, KeyCapUpClose,
		KeyCapVol, KeyCapPitch, KeyCapPct, KeyCapDur, KeyCapSearch,
		KeyCapHeadroom, KeyCapLoudest, KeyCapFrozen, KeyCapBypassed, KeyCapSamplerTitle,
		KeyCapStageOff, KeyCapStarts, KeyCapFades, KeyCapBeforeAfter, KeyCapBreak,
		KeyMenuNoMatches, KeyMenuNoMatchesHint, KeyMenuNoFavorites, KeyMenuNoFavoritesHint,
		KeyFXEmptyHint,
		KeyMaster, KeyNoNotifications, KeyNodeLogic, KeyNodeGroove,
		KeyNoRowSelected, KeyAuto, KeyPre, KeyAnalyzerIdle,
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
		KeyNotifUndo, KeyNotifRedo,
		KeyNotifUndoDetail, KeyNotifRedoDetail,
		KeyParamPan, KeyParamDelaySend, KeyParamReverbSend,
		KeyNotifImportedNamed, KeyNotifInvalidBPMDetail,
		KeyActionChangeBPM, KeyActionChangeSubdivision, KeyActionAddNode, KeyActionDeleteNode,
		KeyActionPasteNode, KeyActionReplaceNode,
		KeyActionMoveNode, KeyActionChangeNodeType, KeyActionEditNode, KeyActionSetStartNode,
		KeyActionAddEdge, KeyActionDeleteEdge, KeyActionAddRow, KeyActionDeleteRow,
		KeyActionChangeInstrument, KeyActionSetMasterVolume, KeyActionAdjustEQ, KeyActionAddEffect,
		KeyActionRemoveEffect, KeyActionAdjustEffect, KeyActionSetRowVolume, KeyActionReorderEffect,
		KeyActionToggleEffect, KeyActionEditSynth, KeyActionResetSynth, KeyActionRecolorRow,
		KeyActionToggleEQFilter, KeyActionRenameInstrument, KeyActionEditSample,
		KeyActionCreateGroup, KeyActionEditGroup, KeyActionDeleteGroup,
		KeySynthStageVoice, KeySynthStageOsc, KeySynthStageFM, KeySynthStagePitch,
		KeySynthStageLFO, KeySynthStageBurst, KeySynthStageEnvelope, KeySynthStageFilter,
		KeySynthStageFilterEnv, KeySynthStagePost, KeySynthStageTone,
		KeySynthStageEnsemble, KeySynthStageFormant, KeySynthStageResonator,
		KeySynthStageEnvelopeShort, KeySynthStageFilterEnvShort,
		KeySynthStageEnsembleShort, KeySynthStageFormantShort, KeySynthStageResonatorShort,
		KeySynthStageVoiceSub, KeySynthStageOscSub, KeySynthStageFMSub, KeySynthStagePitchSub,
		KeySynthStageLFOSub, KeySynthStageBurstSub, KeySynthStageEnvelopeSub, KeySynthStageFilterSub,
		KeySynthStageFilterEnvSub, KeySynthStagePostSub, KeySynthStageToneSub,
		KeySynthStageEnsembleSub, KeySynthStageFormantSub, KeySynthStageResonatorSub,
		KeySynthControlTune, KeySynthControlDecayMult, KeySynthControlTone, KeySynthControlDrive,
		KeySynthControlBody, KeySynthControlBright, KeySynthControlGain, KeySynthControlWave,
		KeySynthControlDetune, KeySynthControlOctave, KeySynthControlAttack, KeySynthControlDecay,
		KeySynthControlSustain, KeySynthControlRelease, KeySynthControlCurve, KeySynthControlType,
		KeySynthControlCutoff, KeySynthControlResonance, KeySynthControlAlgorithm, KeySynthControlBaseFreq,
		KeySynthControlPitchEnv, KeySynthControlPitchEnvDecay, KeySynthControlAmount, KeySynthControlRate,
		KeySynthControlDepth, KeySynthControlSharpness, KeySynthControlDraws, KeySynthControlPrelude,
		KeySynthControlSeed, KeySynthControlPitch, KeySynthControlClick, KeySynthControlNoise,
		KeySynthNounRatio, KeySynthNounLevel, KeySynthNounTime, KeySynthNounHit,
		KeySynthEnumSine, KeySynthEnumSaw, KeySynthEnumSquare, KeySynthEnumTriangle,
		KeySynthEnumNoiseWhite, KeySynthEnumNoisePink, KeySynthEnumParallel, KeySynthEnum3opChain,
		KeySynthEnum4opStack, KeySynthEnumLinear, KeySynthEnumExponential, KeySynthEnumLowPass,
		KeySynthEnumHighPass, KeySynthEnumBandPass, KeySynthEnumOn, KeySynthEnumOff,
		KeySpectrumBandBass, KeySpectrumBandMids, KeySpectrumBandTreble,
		KeyLevelsPeak, KeyLevelsClips, KeyLevelsClear, KeyLevelsReadoutFmt,
		KeyChainOverlay, KeyChainSplit, KeyChainDiff, KeyChainAutoGain, KeyChainFit,
		KeyChainStageSynth, KeyChainStageAntiPop, KeyChainStageFX, KeyChainStageEQ, KeyChainStageBus,
		KeySamplerGroupTrim, KeySamplerGroupTune, KeySamplerGroupLevel,
		KeySamplerKnobStart, KeySamplerKnobEnd, KeySamplerKnobPitch, KeySamplerKnobFine, KeySamplerKnobGain,
		KeySamplerGlossStart, KeySamplerGlossEnd, KeySamplerGlossPitch, KeySamplerGlossFine, KeySamplerGlossGain,
		KeyUnitCents, KeySamplerMetaFmt,
		KeyLevelsTipHeadroom, KeyLevelsTipHeadroomClip, KeyLevelsTipClips,
		KeyLevelsTipLoudest, KeyLevelsTipLoudestNone,
		KeyGroupMenuTitle, KeyGroupMembers, KeyGroupRule,
		KeyGroupRuleOn, KeyGroupRuleOff, KeyGroupEveryN, KeyGroupEveryNShort, KeyGroupDelete, KeyGroupSave,
		KeyGroupParamPitch, KeyGroupParamVolume, KeyGroupParamDuration,
		KeyGroupAddTo, KeyGroupCreateNew, KeyGroupRemoveFrom,
		KeyBannerPasting, KeyBannerMoving, KeyBannerConnectFrom, KeyBannerConnecting, KeyBannerEscHint, KeyEscToCancel,
		KeyConfirmReplaceNode, KeyCapOverwrite,
		KeyImportCollisionTitle, KeyCapReplace, KeyCapKeepBoth,
		KeyBreakCircuitTitle,
		KeyLogs, KeyLogsSearchName, KeyLogsTimeAt, KeyLogsTimeRange,
		KeyLogsFrom, KeyLogsTo, KeyLogsAllCategories, KeyLogsPage,
		KeyLogsGoToPage, KeyLogsClearHistory, KeyNoLogs,
		KeyLogsPickTitle, KeyLogsSet, KeyLogsClear, KeyLogsAnyTime,
		KeyMonth1, KeyMonth2, KeyMonth3, KeyMonth4, KeyMonth5, KeyMonth6,
		KeyMonth7, KeyMonth8, KeyMonth9, KeyMonth10, KeyMonth11, KeyMonth12,
		KeyWeekdaySun, KeyWeekdayMon, KeyWeekdayTue, KeyWeekdayWed,
		KeyWeekdayThu, KeyWeekdayFri, KeyWeekdaySat,
	}
}
