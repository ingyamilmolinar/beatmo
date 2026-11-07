package i18n

// Adding a message touches exactly four places (a const here, AllKeys(), catalog_en.go, catalog_es.go); the
// completeness test fails on a miss.
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

	// Theme editor (grid-pane palette button)
	KeyThemeTitle               Key = "theme.title"
	KeyThemeScopeMine           Key = "theme.scope.mine"
	KeyThemeScopeCircuit        Key = "theme.scope.circuit"
	KeyThemeOverride            Key = "theme.override"
	KeyThemeResetAll            Key = "theme.reset_all"
	KeyThemeReset               Key = "theme.reset"
	KeyThemeCatBackground       Key = "theme.cat.background"
	KeyThemeCatSurfaces         Key = "theme.cat.surfaces"
	KeyThemeCatText             Key = "theme.cat.text"
	KeyThemeCatAccent           Key = "theme.cat.accent"
	KeyThemeCatFocus            Key = "theme.cat.focus"
	KeyThemeCatGrid             Key = "theme.cat.grid"
	KeyThemeCatNodes            Key = "theme.cat.nodes"
	KeyThemeCatDividers         Key = "theme.cat.dividers"
	KeyThemeCatViz              Key = "theme.cat.viz"
	KeyThemeGlossBackground     Key = "theme.gloss.background"
	KeyThemeGlossSurfaces       Key = "theme.gloss.surfaces"
	KeyThemeGlossText           Key = "theme.gloss.text"
	KeyThemeGlossAccent         Key = "theme.gloss.accent"
	KeyThemeGlossFocus          Key = "theme.gloss.focus"
	KeyThemeGlossGrid           Key = "theme.gloss.grid"
	KeyThemeGlossNodes          Key = "theme.gloss.nodes"
	KeyThemeGlossDividers       Key = "theme.gloss.dividers"
	KeyThemeGlossViz            Key = "theme.gloss.viz"
	KeyThemeAdjust              Key = "theme.adjust"
	KeyThemeCustomized          Key = "theme.customized"
	KeyThemePresetViceCity      Key = "theme.preset.vice-city"
	KeyThemePresetViceCityGloss Key = "theme.preset.vice-city.gloss"
	KeyThemePresetOcean         Key = "theme.preset.ocean"
	KeyThemePresetOceanGloss    Key = "theme.preset.ocean.gloss"
	KeyThemePresetMidnight      Key = "theme.preset.midnight"
	KeyThemePresetMidnightGloss Key = "theme.preset.midnight.gloss"
	KeyThemePresetEmber         Key = "theme.preset.ember"
	KeyThemePresetEmberGloss    Key = "theme.preset.ember.gloss"
	KeyThemePresetNeonMint      Key = "theme.preset.neon-mint"
	KeyThemePresetNeonMintGloss Key = "theme.preset.neon-mint.gloss"
	KeyThemePresetOrchid        Key = "theme.preset.orchid"
	KeyThemePresetOrchidGloss   Key = "theme.preset.orchid.gloss"

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

	// Row rack: the add-row ghost row's label.
	KeyRackAddInstrument Key = "rack.add_instrument"

	// Action button labels
	KeySave   Key = "btn.save"
	KeySaveAs Key = "btn.save_as"
	KeyReset  Key = "btn.reset"
	// Save As (the only save): the confirm button reads what will happen; a
	// name that belongs to another instrument is refused; a save that could not
	// be stored is reported.
	KeySaveOverFmt  Key = "btn.save_over"
	KeySaveAsNew    Key = "btn.save_as_new"
	KeyNameTakenFmt Key = "msg.name_taken"
	KeySaveFailed   Key = "msg.save_failed"
	// The question when choosing an instrument (or applying a kit) would
	// replace a row's settings with the instrument's own.
	KeyKeepRowSettings          Key = "pick.keep_row_settings"
	KeyAlsoChangesRowFmt        Key = "pick.also_changes_row"
	KeyUseInstrumentSettingsFmt Key = "pick.use_instrument_settings"
	KeyKeepEachRowSettings      Key = "pick.keep_each_row_settings"
	KeyUseKitSettings           Key = "pick.use_kit_settings"
	KeyKeepRowNSettingsFmt      Key = "pick.keep_row_n_settings"
	KeyPreview                  Key = "btn.preview"
	KeyCancel                   Key = "btn.cancel"
	KeyOK                       Key = "btn.ok"
	KeyAddEffect                Key = "btn.add_effect"
	KeyLoadWAV                  Key = "btn.load_wav"
	KeyNormalize                Key = "btn.normalize"
	KeyReverse                  Key = "btn.reverse"
	KeyFade                     Key = "btn.fade"

	// Builtin instrument default display names
	KeyInstKick  Key = "inst.kick"
	KeyInstSnare Key = "inst.snare"
	KeyInstHiHat Key = "inst.hihat"
	KeyInstClap  Key = "inst.clap"
	KeyInstTom   Key = "inst.tom"

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

	// Spectrum findings — the headline sentence on the Spectrum tab. Each states
	// a CHECKABLE fact about the current sound; none expresses taste.
	KeySpectrumFindClipping  Key = "spectrum.find.clipping"
	KeySpectrumFindCollision Key = "spectrum.find.collision"
	KeySpectrumFindPileUp    Key = "spectrum.find.pileup"
	KeySpectrumFindDeadHigh  Key = "spectrum.find.dead.high"
	KeySpectrumFindDeadLow   Key = "spectrum.find.dead.low"
	KeySpectrumFindOK        Key = "spectrum.find.ok"
	// KeyAnd is the conjunction used when listing instruments in a sentence
	// ("Kick and Organ"). Separate from the sentences themselves because the
	// word order around it differs by language.
	KeyAnd Key = "word.and"

	// Chain tab legibility: the next-tap hint, the A/B legend and the
	// plain-English gloss for each display mode / toggle pill.
	KeyChainNextTapA       Key = "chain.nexttap.a"
	KeyChainNextTapB       Key = "chain.nexttap.b"
	KeyChainNextTapReplace Key = "chain.nexttap.replace"
	KeyChainLegendUnset    Key = "chain.legend.unset"
	KeyChainGlossOverlay   Key = "chain.gloss.overlay"
	KeyChainGlossSplit     Key = "chain.gloss.split"
	KeyChainGlossDiff      Key = "chain.gloss.diff"
	KeyChainGlossFreeze    Key = "chain.gloss.freeze"
	KeyChainFreeze         Key = "chain.freeze"
	KeyChainLive           Key = "chain.live"
	KeyLegendMeters        Key = "legend.meters"
	KeyLegendEQ            Key = "legend.eq"
	KeyLegendScope         Key = "legend.scope"
	KeyLegendSynth         Key = "legend.synth"
	KeyLegendSampler       Key = "legend.sampler"

	// PlainEnglish knob/label glosses (kid-readable subtitles)
	KeyGlossVoice     Key = "gloss.voice"
	KeyGlossOsc       Key = "gloss.osc"
	KeyGlossPitch     Key = "gloss.pitch"
	KeyGlossWaveform  Key = "gloss.waveform"
	KeyGlossGenerator Key = "gloss.generator"
	KeyGlossTune      Key = "gloss.tune"
	KeyGlossGlide     Key = "gloss.glide"
	KeyGlossBend      Key = "gloss.bend"
	KeyGlossDetune    Key = "gloss.detune"
	KeyGlossEnvelope  Key = "gloss.envelope"
	KeyGlossAttack    Key = "gloss.attack"
	KeyGlossDecay     Key = "gloss.decay"
	KeyGlossSustain   Key = "gloss.sustain"
	KeyGlossRelease   Key = "gloss.release"
	KeyGlossTone      Key = "gloss.tone"
	KeyGlossCutoff    Key = "gloss.cutoff"
	KeyGlossFilter    Key = "gloss.filter"
	KeyGlossReso      Key = "gloss.reso"
	KeyGlossResonance Key = "gloss.resonance"
	KeyGlossBody      Key = "gloss.body"
	// FINISH stage glosses.
	KeyGlossResample     Key = "gloss.resample"
	KeyGlossFadeSpeed    Key = "gloss.fade-speed"
	KeyGlossLowBoost     Key = "gloss.low-boost"
	KeyGlossDarken       Key = "gloss.darken"
	KeyGlossCrunch       Key = "gloss.crunch"
	KeyGlossLimit        Key = "gloss.limit"
	KeyGlossCrisp        Key = "gloss.crisp"
	KeyGlossBasePitch    Key = "gloss.base_pitch"
	KeyGlossPitchSweep   Key = "gloss.pitch_sweep"
	KeyGlossSweepTime    Key = "gloss.sweep_time"
	KeyGlossOpRatio      Key = "gloss.op_ratio"
	KeyGlossOpFMDepth    Key = "gloss.op_fm_depth"
	KeyGlossOpDecay      Key = "gloss.op_decay"
	KeyGlossFundamental  Key = "gloss.fundamental"
	KeyGlossSweepSpeed   Key = "gloss.sweep_speed"
	KeyGlossBoomDecay    Key = "gloss.boom_decay"
	KeyGlossBodyDecay    Key = "gloss.body_decay"
	KeyGloss2ndHarmonic  Key = "gloss.2nd_harmonic"
	KeyGloss3rdHarmonic  Key = "gloss.3rd_harmonic"
	KeyGloss4thHarmonic  Key = "gloss.4th_harmonic"
	KeyGlossClick        Key = "gloss.click"
	KeyGlossThud         Key = "gloss.thud"
	KeyGlossRingDecay    Key = "gloss.ring_decay"
	KeyGlossOvertone1    Key = "gloss.overtone1"
	KeyGlossOvertone2    Key = "gloss.overtone2"
	KeyGlossStick        Key = "gloss.stick"
	KeyGlossRoom         Key = "gloss.room"
	KeyGlossTone2        Key = "gloss.tone2"
	KeyGlossNoiseTune    Key = "gloss.noise_tune"
	KeyGlossToneDecay    Key = "gloss.tone_decay"
	KeyGlossNoiseDecay   Key = "gloss.noise_decay"
	KeyGlossTailDecay    Key = "gloss.tail_decay"
	KeyGlossToneLevel    Key = "gloss.tone_level"
	KeyGlossNoiseLevel   Key = "gloss.noise_level"
	KeyGlossWires        Key = "gloss.wires"
	KeyGlossSnap         Key = "gloss.snap"
	KeyGlossMetalTune    Key = "gloss.metal_tune"
	KeyGlossAttackDecay  Key = "gloss.attack_decay"
	KeyGlossMetalLevel   Key = "gloss.metal_level"
	KeyGlossSizzleLevel  Key = "gloss.sizzle_level"
	KeyGlossSizzleDecay  Key = "gloss.sizzle_decay"
	KeyGlossPlateModes   Key = "gloss.plate_modes"
	KeyGlossLowestMode   Key = "gloss.lowest_mode"
	KeyGlossHighestMode  Key = "gloss.highest_mode"
	KeyGlossHighsFade    Key = "gloss.highs_fade"
	KeyGlossTickCorner   Key = "gloss.tick_corner"
	KeyGlossShimmer      Key = "gloss.shimmer"
	KeyGlossStrikeTone   Key = "gloss.strike_tone"
	KeyGlossStrikeWidth  Key = "gloss.strike_width"
	KeyGlossBodyTone     Key = "gloss.body_tone"
	KeyGlossBodyWidth    Key = "gloss.body_width"
	KeyGlossSparkle      Key = "gloss.sparkle"
	KeyGlossSparkleTone  Key = "gloss.sparkle_tone"
	KeyGlossSparkleWidth Key = "gloss.sparkle_width"
	KeyGlossWash         Key = "gloss.wash"
	KeyGlossSoftness     Key = "gloss.softness"
	KeyGlossRattle       Key = "gloss.rattle"
	KeyGlossRattleSpeed  Key = "gloss.rattle_speed"
	KeyGlossRattleLength Key = "gloss.rattle_length"
	KeyGlossWireDelay    Key = "gloss.wire_delay"
	KeyGlossBuzzHold     Key = "gloss.buzz_hold"
	KeyGlossPluck        Key = "gloss.pluck"
	KeyGlossPick         Key = "gloss.pick"
	KeyGlossFade         Key = "gloss.fade"
	KeyGlossLength       Key = "gloss.length"
	KeyGlossOvertone     Key = "gloss.overtone"
	KeyGlossPitchPunch   Key = "gloss.pitch_punch"
	KeyGlossPunch        Key = "gloss.punch"
	KeyGlossDrive        Key = "gloss.drive"
	KeyGlossGain         Key = "gloss.gain"
	KeyGlossSaturation   Key = "gloss.saturation"
	KeyGlossSat          Key = "gloss.sat"
	KeyGlossDistortion   Key = "gloss.distortion"
	KeyGlossSlope        Key = "gloss.slope"
	KeyGlossSlopes       Key = "gloss.slopes"
	KeyGlossPre          Key = "gloss.pre"
	KeyGlossPost         Key = "gloss.post"
	KeyGlossPeak         Key = "gloss.peak"
	KeyGlossRMS          Key = "gloss.rms"
	KeyGlossClip         Key = "gloss.clip"
	KeyGlossClips        Key = "gloss.clips"
	KeyGlossHeadroom     Key = "gloss.headroom"
	KeyGlossLUFS         Key = "gloss.lufs"
	KeyGlossK20          Key = "gloss.k20"
	KeyGlossOverlay      Key = "gloss.overlay"
	KeyGlossSplit        Key = "gloss.split"
	KeyGlossDiff         Key = "gloss.diff"
	KeyGlossOvr          Key = "gloss.ovr"
	KeyGlossSpl          Key = "gloss.spl"
	KeyGlossDif          Key = "gloss.dif"
	KeyGlossAG           Key = "gloss.ag"
	KeyGlossAutoGain     Key = "gloss.auto_gain"
	KeyGlossOut          Key = "gloss.out"
	KeyGlossDelay        Key = "gloss.delay"
	KeyGlossReverb       Key = "gloss.reverb"
	KeyGlossSend         Key = "gloss.send"
	KeyGlossPlay         Key = "gloss.play"
	KeyGlossStop         Key = "gloss.stop"
	KeyGlossPause        Key = "gloss.pause"
	KeySynthAdvanced     Key = "synth.advanced"
	// Synth-tab CATEGORY cards (beginner overview): label + one-liner gloss.
	KeySynthCatSound           Key = "synth.cat.sound"
	KeySynthCatSoundGloss      Key = "synth.cat.sound.gloss"
	KeySynthCatBrightness      Key = "synth.cat.brightness"
	KeySynthCatBrightnessGloss Key = "synth.cat.brightness.gloss"
	KeySynthCatAttack          Key = "synth.cat.attack"
	KeySynthCatAttackGloss     Key = "synth.cat.attack.gloss"
	KeySynthCatLength          Key = "synth.cat.length"
	KeySynthCatLengthGloss     Key = "synth.cat.length.gloss"
	KeySynthCatWidth           Key = "synth.cat.width"
	KeySynthCatWidthGloss      Key = "synth.cat.width.gloss"
	KeySynthCatHumanFeel       Key = "synth.cat.humanfeel"
	KeySynthCatHumanFeelGloss  Key = "synth.cat.humanfeel.gloss"
	KeySynthCatMovement        Key = "synth.cat.movement"
	KeySynthCatMovementGloss   Key = "synth.cat.movement.gloss"
	KeySynthCatBody            Key = "synth.cat.body"
	KeySynthCatBodyGloss       Key = "synth.cat.body.gloss"
	KeySynthCatWarmth          Key = "synth.cat.warmth"
	KeySynthCatWarmthGloss     Key = "synth.cat.warmth.gloss"
	KeySynthCatPitchDrop       Key = "synth.cat.pitchdrop"
	KeySynthCatPitchDropGloss  Key = "synth.cat.pitchdrop.gloss"
	KeySynthCatVoice           Key = "synth.cat.voice"
	KeySynthCatVoiceGloss      Key = "synth.cat.voice.gloss"
	KeySynthCatModes           Key = "synth.cat.modes"
	KeySynthCatModesGloss      Key = "synth.cat.modes.gloss"
	KeySynthCatMallet          Key = "synth.cat.mallet"
	KeySynthCatMalletGloss     Key = "synth.cat.mallet.gloss"
	KeySynthCatFlam            Key = "synth.cat.flam"
	KeySynthCatFlamGloss       Key = "synth.cat.flam.gloss"
	KeySynthCatPunch           Key = "synth.cat.punch"
	KeySynthCatPunchGloss      Key = "synth.cat.punch.gloss"
	KeySynthCatBoom            Key = "synth.cat.boom"
	KeySynthCatBoomGloss       Key = "synth.cat.boom.gloss"
	KeySynthCatTone            Key = "synth.cat.tone"
	KeySynthCatToneGloss       Key = "synth.cat.tone.gloss"
	KeySynthCatSource          Key = "synth.cat.source"
	KeySynthCatSourceGloss     Key = "synth.cat.source.gloss"
	KeySynthCatGrit            Key = "synth.cat.grit"
	KeySynthCatGritGloss       Key = "synth.cat.grit.gloss"
	KeySynthCatLengthPercGloss Key = "synth.cat.length.perc.gloss"
	// Synth-tab MACRO DIALS (internal/assets/synth_dials.json): label + gloss.
	KeySynthDialRichness           Key = "synth.dial.richness"
	KeySynthDialRichnessGloss      Key = "synth.dial.richness.gloss"
	KeySynthDialBite               Key = "synth.dial.bite"
	KeySynthDialBiteGloss          Key = "synth.dial.bite.gloss"
	KeySynthDialBright             Key = "synth.dial.bright"
	KeySynthDialBrightGloss        Key = "synth.dial.bright.gloss"
	KeySynthDialSizzle             Key = "synth.dial.sizzle"
	KeySynthDialSizzleGloss        Key = "synth.dial.sizzle.gloss"
	KeySynthDialEdge               Key = "synth.dial.edge"
	KeySynthDialEdgeGloss          Key = "synth.dial.edge.gloss"
	KeySynthDialSweep              Key = "synth.dial.sweep"
	KeySynthDialSweepGloss         Key = "synth.dial.sweep.gloss"
	KeySynthDialFadein             Key = "synth.dial.fadein"
	KeySynthDialFadeinGloss        Key = "synth.dial.fadein.gloss"
	KeySynthDialSnap               Key = "synth.dial.snap"
	KeySynthDialSnapGloss          Key = "synth.dial.snap.gloss"
	KeySynthDialLength             Key = "synth.dial.length"
	KeySynthDialLengthGloss        Key = "synth.dial.length.gloss"
	KeySynthDialRoom               Key = "synth.dial.room"
	KeySynthDialRoomGloss          Key = "synth.dial.room.gloss"
	KeySynthDialHold               Key = "synth.dial.hold"
	KeySynthDialHoldGloss          Key = "synth.dial.hold.gloss"
	KeySynthDialTail               Key = "synth.dial.tail"
	KeySynthDialTailGloss          Key = "synth.dial.tail.gloss"
	KeySynthDialSpread             Key = "synth.dial.spread"
	KeySynthDialSpreadGloss        Key = "synth.dial.spread.gloss"
	KeySynthDialBlend              Key = "synth.dial.blend"
	KeySynthDialBlendGloss         Key = "synth.dial.blend.gloss"
	KeySynthDialDrift              Key = "synth.dial.drift"
	KeySynthDialDriftGloss         Key = "synth.dial.drift.gloss"
	KeySynthDialVibrato            Key = "synth.dial.vibrato"
	KeySynthDialVibratoGloss       Key = "synth.dial.vibrato.gloss"
	KeySynthDialLooseness          Key = "synth.dial.looseness"
	KeySynthDialLoosenessGloss     Key = "synth.dial.looseness.gloss"
	KeySynthDialDepth              Key = "synth.dial.depth"
	KeySynthDialDepthGloss         Key = "synth.dial.depth.gloss"
	KeySynthDialSpeedLfo           Key = "synth.dial.speed-lfo"
	KeySynthDialSpeedLfoGloss      Key = "synth.dial.speed-lfo.gloss"
	KeySynthDialDelay              Key = "synth.dial.delay"
	KeySynthDialDelayGloss         Key = "synth.dial.delay.gloss"
	KeySynthDialBody               Key = "synth.dial.body"
	KeySynthDialBodyGloss          Key = "synth.dial.body.gloss"
	KeySynthDialExpression         Key = "synth.dial.expression"
	KeySynthDialExpressionGloss    Key = "synth.dial.expression.gloss"
	KeySynthDialWeight             Key = "synth.dial.weight"
	KeySynthDialWeightGloss        Key = "synth.dial.weight.gloss"
	KeySynthDialRing               Key = "synth.dial.ring"
	KeySynthDialRingGloss          Key = "synth.dial.ring.gloss"
	KeySynthDialDrive              Key = "synth.dial.drive"
	KeySynthDialDriveGloss         Key = "synth.dial.drive.gloss"
	KeySynthDialDrop               Key = "synth.dial.drop"
	KeySynthDialDropGloss          Key = "synth.dial.drop.gloss"
	KeySynthDialSpeedDrop          Key = "synth.dial.speed-drop"
	KeySynthDialSpeedDropGloss     Key = "synth.dial.speed-drop.gloss"
	KeySynthDialCharacter          Key = "synth.dial.character"
	KeySynthDialCharacterGloss     Key = "synth.dial.character.gloss"
	KeySynthDialBreath             Key = "synth.dial.breath"
	KeySynthDialBreathGloss        Key = "synth.dial.breath.gloss"
	KeySynthDialShift              Key = "synth.dial.shift"
	KeySynthDialShiftGloss         Key = "synth.dial.shift.gloss"
	KeySynthDialSpreadFlam         Key = "synth.dial.spread-flam"
	KeySynthDialSpreadFlamGloss    Key = "synth.dial.spread-flam.gloss"
	KeySynthDialStrength           Key = "synth.dial.strength"
	KeySynthDialStrengthGloss      Key = "synth.dial.strength.gloss"
	KeySynthDialSharp              Key = "synth.dial.sharp"
	KeySynthDialSharpGloss         Key = "synth.dial.sharp.gloss"
	KeySynthDialGlassy             Key = "synth.dial.glassy"
	KeySynthDialGlassyGloss        Key = "synth.dial.glassy.gloss"
	KeySynthDialShimmer            Key = "synth.dial.shimmer"
	KeySynthDialShimmerGloss       Key = "synth.dial.shimmer.gloss"
	KeySynthDialBloom              Key = "synth.dial.bloom"
	KeySynthDialBloomGloss         Key = "synth.dial.bloom.gloss"
	KeySynthDialClick              Key = "synth.dial.click"
	KeySynthDialClickGloss         Key = "synth.dial.click.gloss"
	KeySynthDialAir                Key = "synth.dial.air"
	KeySynthDialAirGloss           Key = "synth.dial.air.gloss"
	KeySynthDialShine              Key = "synth.dial.shine"
	KeySynthDialShineGloss         Key = "synth.dial.shine.gloss"
	KeySynthDialTalk               Key = "synth.dial.talk"
	KeySynthDialTalkGloss          Key = "synth.dial.talk.gloss"
	KeySynthDialRichnessLo         Key = "synth.dial.richness.lo"
	KeySynthDialRichnessHi         Key = "synth.dial.richness.hi"
	KeySynthDialBiteLo             Key = "synth.dial.bite.lo"
	KeySynthDialBiteHi             Key = "synth.dial.bite.hi"
	KeySynthDialGlassyLo           Key = "synth.dial.glassy.lo"
	KeySynthDialGlassyHi           Key = "synth.dial.glassy.hi"
	KeySynthDialBrightLo           Key = "synth.dial.bright.lo"
	KeySynthDialBrightHi           Key = "synth.dial.bright.hi"
	KeySynthDialSizzleLo           Key = "synth.dial.sizzle.lo"
	KeySynthDialSizzleHi           Key = "synth.dial.sizzle.hi"
	KeySynthDialShimmerLo          Key = "synth.dial.shimmer.lo"
	KeySynthDialShimmerHi          Key = "synth.dial.shimmer.hi"
	KeySynthDialEdgeLo             Key = "synth.dial.edge.lo"
	KeySynthDialEdgeHi             Key = "synth.dial.edge.hi"
	KeySynthDialSweepLo            Key = "synth.dial.sweep.lo"
	KeySynthDialSweepHi            Key = "synth.dial.sweep.hi"
	KeySynthDialBloomLo            Key = "synth.dial.bloom.lo"
	KeySynthDialBloomHi            Key = "synth.dial.bloom.hi"
	KeySynthDialFadeinLo           Key = "synth.dial.fadein.lo"
	KeySynthDialFadeinHi           Key = "synth.dial.fadein.hi"
	KeySynthDialClickLo            Key = "synth.dial.click.lo"
	KeySynthDialClickHi            Key = "synth.dial.click.hi"
	KeySynthDialSnapLo             Key = "synth.dial.snap.lo"
	KeySynthDialSnapHi             Key = "synth.dial.snap.hi"
	KeySynthDialLengthLo           Key = "synth.dial.length.lo"
	KeySynthDialLengthHi           Key = "synth.dial.length.hi"
	KeySynthDialRoomLo             Key = "synth.dial.room.lo"
	KeySynthDialRoomHi             Key = "synth.dial.room.hi"
	KeySynthDialTailLo             Key = "synth.dial.tail.lo"
	KeySynthDialTailHi             Key = "synth.dial.tail.hi"
	KeySynthDialHoldLo             Key = "synth.dial.hold.lo"
	KeySynthDialHoldHi             Key = "synth.dial.hold.hi"
	KeySynthDialSpreadLo           Key = "synth.dial.spread.lo"
	KeySynthDialSpreadHi           Key = "synth.dial.spread.hi"
	KeySynthDialBlendLo            Key = "synth.dial.blend.lo"
	KeySynthDialBlendHi            Key = "synth.dial.blend.hi"
	KeySynthDialDriftLo            Key = "synth.dial.drift.lo"
	KeySynthDialDriftHi            Key = "synth.dial.drift.hi"
	KeySynthDialVibratoLo          Key = "synth.dial.vibrato.lo"
	KeySynthDialVibratoHi          Key = "synth.dial.vibrato.hi"
	KeySynthDialLoosenessLo        Key = "synth.dial.looseness.lo"
	KeySynthDialLoosenessHi        Key = "synth.dial.looseness.hi"
	KeySynthDialDepthLo            Key = "synth.dial.depth.lo"
	KeySynthDialDepthHi            Key = "synth.dial.depth.hi"
	KeySynthDialSpeedLfoLo         Key = "synth.dial.speed-lfo.lo"
	KeySynthDialSpeedLfoHi         Key = "synth.dial.speed-lfo.hi"
	KeySynthDialDelayLo            Key = "synth.dial.delay.lo"
	KeySynthDialDelayHi            Key = "synth.dial.delay.hi"
	KeySynthDialBodyLo             Key = "synth.dial.body.lo"
	KeySynthDialBodyHi             Key = "synth.dial.body.hi"
	KeySynthDialExpressionLo       Key = "synth.dial.expression.lo"
	KeySynthDialExpressionHi       Key = "synth.dial.expression.hi"
	KeySynthDialWeightLo           Key = "synth.dial.weight.lo"
	KeySynthDialWeightHi           Key = "synth.dial.weight.hi"
	KeySynthDialRingLo             Key = "synth.dial.ring.lo"
	KeySynthDialRingHi             Key = "synth.dial.ring.hi"
	KeySynthDialAirLo              Key = "synth.dial.air.lo"
	KeySynthDialAirHi              Key = "synth.dial.air.hi"
	KeySynthDialDriveLo            Key = "synth.dial.drive.lo"
	KeySynthDialDriveHi            Key = "synth.dial.drive.hi"
	KeySynthDialDropLo             Key = "synth.dial.drop.lo"
	KeySynthDialDropHi             Key = "synth.dial.drop.hi"
	KeySynthDialSpeedDropLo        Key = "synth.dial.speed-drop.lo"
	KeySynthDialSpeedDropHi        Key = "synth.dial.speed-drop.hi"
	KeySynthDialCharacterLo        Key = "synth.dial.character.lo"
	KeySynthDialCharacterHi        Key = "synth.dial.character.hi"
	KeySynthDialBreathLo           Key = "synth.dial.breath.lo"
	KeySynthDialBreathHi           Key = "synth.dial.breath.hi"
	KeySynthDialShiftLo            Key = "synth.dial.shift.lo"
	KeySynthDialShiftHi            Key = "synth.dial.shift.hi"
	KeySynthDialShineLo            Key = "synth.dial.shine.lo"
	KeySynthDialShineHi            Key = "synth.dial.shine.hi"
	KeySynthDialTalkLo             Key = "synth.dial.talk.lo"
	KeySynthDialTalkHi             Key = "synth.dial.talk.hi"
	KeySynthDialSpreadFlamLo       Key = "synth.dial.spread-flam.lo"
	KeySynthDialSpreadFlamHi       Key = "synth.dial.spread-flam.hi"
	KeySynthDialStrengthLo         Key = "synth.dial.strength.lo"
	KeySynthDialStrengthHi         Key = "synth.dial.strength.hi"
	KeySynthDialSharpLo            Key = "synth.dial.sharp.lo"
	KeySynthDialSharpHi            Key = "synth.dial.sharp.hi"
	KeyGraphViewOverview           Key = "synth.graph.view.overview"
	KeyGraphViewLoudness           Key = "synth.graph.view.loudness"
	KeyGraphViewAttack             Key = "synth.graph.view.attack"
	KeyGraphViewPitch              Key = "synth.graph.view.pitch"
	KeyGraphViewColour             Key = "synth.graph.view.colour"
	KeyGraphViewHarmonics          Key = "synth.graph.view.harmonics"
	KeyGraphViewColourTime         Key = "synth.graph.view.colour_time"
	KeyGraphViewNoise              Key = "synth.graph.view.noise"
	KeyGraphViewWave               Key = "synth.graph.view.wave"
	KeyGraphSoundsFor              Key = "synth.graph.sounds_for"
	KeyGraphFullAfter              Key = "synth.graph.full_after"
	KeyGraphPitchGlide             Key = "synth.graph.pitch_glide"
	KeyGraphPitchWobble            Key = "synth.graph.pitch_wobble"
	KeyGraphPitchHolds             Key = "synth.graph.pitch_holds"
	KeyGraphNoPitch                Key = "synth.graph.no_pitch"
	KeyGraphBrightCentre           Key = "synth.graph.bright_centre"
	KeyGraphHarmonicsCount         Key = "synth.graph.harmonics_count"
	KeyGraphCentreMoves            Key = "synth.graph.centre_moves"
	KeyGraphNoiseShare             Key = "synth.graph.noise_share"
	KeyGraphWaveAt                 Key = "synth.graph.wave_at"
	KeyGraphWas                    Key = "synth.graph.was"
	KeyGraphNoteEnds               Key = "synth.graph.note_ends"
	KeyGraphBefore                 Key = "synth.graph.before"
	KeyGraphUpdating               Key = "synth.graph.updating"
	KeyGraphNoPicture              Key = "synth.graph.no_picture"
	KeyGraphSamplerEdit            Key = "synth.graph.sampler_edit"
	KeyGraphLow                    Key = "synth.graph.low"
	KeyGraphHigh                   Key = "synth.graph.high"
	KeyGraphQuiet                  Key = "synth.graph.quiet"
	KeyGraphLoud                   Key = "synth.graph.loud"
	KeyGraphTone                   Key = "synth.graph.tone"
	KeyGraphNoise                  Key = "synth.graph.noise"
	KeyGraphSummary                Key = "synth.graph.summary"
	KeyGraphViewPick               Key = "synth.graph.view_pick"
	KeyGraphAxisDark               Key = "synth.graph.axis_dark"
	KeyGraphAxisBright             Key = "synth.graph.axis_bright"
	KeyGraphDetails                Key = "synth.graph.details"
	KeyGraphFullAfterClick         Key = "synth.graph.full_after_click"
	KeyGraphWaveCrest              Key = "synth.graph.wave_crest"
	KeyGraphWordsLenHit            Key = "synth.graph.words.len_hit"
	KeyGraphWordsLenShort          Key = "synth.graph.words.len_short"
	KeyGraphWordsLenMedium         Key = "synth.graph.words.len_medium"
	KeyGraphWordsLenLong           Key = "synth.graph.words.len_long"
	KeyGraphWordsLenVeryLong       Key = "synth.graph.words.len_very_long"
	KeyGraphWordsToTheEnd          Key = "synth.graph.words.to_the_end"
	KeyGraphWordsAtkInstant        Key = "synth.graph.words.atk_instant"
	KeyGraphWordsAtkQuick          Key = "synth.graph.words.atk_quick"
	KeyGraphWordsAtkGentle         Key = "synth.graph.words.atk_gentle"
	KeyGraphWordsAtkSlow           Key = "synth.graph.words.atk_slow"
	KeyGraphWordsWithClick         Key = "synth.graph.words.with_click"
	KeyGraphWordsPitchDropFast     Key = "synth.graph.words.pitch_drop_fast"
	KeyGraphWordsPitchDrop         Key = "synth.graph.words.pitch_drop"
	KeyGraphWordsPitchDropSlow     Key = "synth.graph.words.pitch_drop_slow"
	KeyGraphWordsPitchRise         Key = "synth.graph.words.pitch_rise"
	KeyGraphWordsPitchWobbleGentle Key = "synth.graph.words.pitch_wobble_gentle"
	KeyGraphWordsPitchWobbleLot    Key = "synth.graph.words.pitch_wobble_lot"
	KeyGraphWordsPitchSteady       Key = "synth.graph.words.pitch_steady"
	KeyGraphWordsDeep              Key = "synth.graph.words.deep"
	KeyGraphWordsWarm              Key = "synth.graph.words.warm"
	KeyGraphWordsBalanced          Key = "synth.graph.words.balanced"
	KeyGraphWordsBright            Key = "synth.graph.words.bright"
	KeyGraphWordsVeryBright        Key = "synth.graph.words.very_bright"
	KeyGraphWordsPure              Key = "synth.graph.words.pure"
	KeyGraphWordsSomeOvertones     Key = "synth.graph.words.some_overtones"
	KeyGraphWordsRich              Key = "synth.graph.words.rich"
	KeyGraphWordsGetsDarker        Key = "synth.graph.words.gets_darker"
	KeyGraphWordsGetsBrighter      Key = "synth.graph.words.gets_brighter"
	KeyGraphWordsKeepsColour       Key = "synth.graph.words.keeps_colour"
	KeyGraphWordsClean             Key = "synth.graph.words.clean"
	KeyGraphWordsSomeHiss          Key = "synth.graph.words.some_hiss"
	KeyGraphWordsHalfNoise         Key = "synth.graph.words.half_noise"
	KeyGraphWordsMostlyNoise       Key = "synth.graph.words.mostly_noise"
	KeyGraphWordsSmooth            Key = "synth.graph.words.smooth"
	KeyGraphWordsSlightlySquared   Key = "synth.graph.words.slightly_squared"
	KeyGraphWordsSquared           Key = "synth.graph.words.squared"
	KeyGraphWordsLonger            Key = "synth.graph.words.longer"
	KeyGraphWordsShorter           Key = "synth.graph.words.shorter"
	KeyGraphWordsSnappier          Key = "synth.graph.words.snappier"
	KeyGraphWordsSofter            Key = "synth.graph.words.softer"
	KeyGraphWordsBiggerDrop        Key = "synth.graph.words.bigger_drop"
	KeyGraphWordsSmallerDrop       Key = "synth.graph.words.smaller_drop"
	KeyGraphWordsMoreWobble        Key = "synth.graph.words.more_wobble"
	KeyGraphWordsLessWobble        Key = "synth.graph.words.less_wobble"
	KeyGraphWordsBrighter          Key = "synth.graph.words.brighter"
	KeyGraphWordsDarker            Key = "synth.graph.words.darker"
	KeyGraphWordsRicher            Key = "synth.graph.words.richer"
	KeyGraphWordsPlainer           Key = "synth.graph.words.plainer"
	KeyGraphWordsNoisier           Key = "synth.graph.words.noisier"
	KeyGraphWordsCleaner           Key = "synth.graph.words.cleaner"
	KeyGraphWordsGrittier          Key = "synth.graph.words.grittier"
	KeyGraphWordsSmoother          Key = "synth.graph.words.smoother"
	KeySynthMore                   Key = "synth.more"
	KeySynthAddStage               Key = "synth.add_stage"
	KeyCapSynthTitle               Key = "cap.synth_title"
	KeyGlossSaxBlow                Key = "gloss.sax_blow"
	KeyGlossSaxReed                Key = "gloss.sax_reed"
	KeyGlossSaxReedStiff           Key = "gloss.sax_reed_stiff"
	KeyGlossSaxBell                Key = "gloss.sax_bell"
	KeyGlossSaxBreath              Key = "gloss.sax_breath"
	KeyGlossSaxBright              Key = "gloss.sax_bright"
	KeyGlossBowPos                 Key = "gloss.bow_pos"
	KeyGlossBowPress               Key = "gloss.bow_press"
	KeyGlossBowSpeed               Key = "gloss.bow_speed"
	KeyGlossBowBright              Key = "gloss.bow_bright"
	KeyGlossOscillator             Key = "gloss.oscillator"
	KeyGlossOctave                 Key = "gloss.octave"
	KeyGlossAlgorithm              Key = "gloss.algorithm"
	KeyGlossOp1Ratio               Key = "gloss.op1_ratio"
	KeyGlossOp2Ratio               Key = "gloss.op2_ratio"
	KeyGlossOp3Ratio               Key = "gloss.op3_ratio"
	KeyGlossOp4Ratio               Key = "gloss.op4_ratio"
	KeyGlossOp1Depth               Key = "gloss.op1_depth"
	KeyGlossOp2Depth               Key = "gloss.op2_depth"
	KeyGlossOp3Depth               Key = "gloss.op3_depth"
	KeyGlossOp4Depth               Key = "gloss.op4_depth"
	KeyGlossOp1Level               Key = "gloss.op1_level"
	KeyGlossOp2Level               Key = "gloss.op2_level"
	KeyGlossOp3Level               Key = "gloss.op3_level"
	KeyGlossOp4Level               Key = "gloss.op4_level"
	KeyGlossCurve                  Key = "gloss.curve"
	KeyGlossFilterEnv              Key = "gloss.filter_env"
	KeyGlossEnvAmount              Key = "gloss.env_amount"
	KeyGlossEnvRise                Key = "gloss.env_rise"
	KeyGlossEnvDecay               Key = "gloss.env_decay"
	KeyGlossLfo                    Key = "gloss.lfo"
	KeyGlossRate                   Key = "gloss.rate"
	KeyGlossDepth                  Key = "gloss.depth"
	KeyGlossLfoTarget              Key = "gloss.lfo_target"
	KeyGlossVibratoDelay           Key = "gloss.vibrato_delay"
	KeyGlossPitchEnv               Key = "gloss.pitch_env"
	KeyGlossSweep                  Key = "gloss.sweep"
	KeyGlossBurst                  Key = "gloss.burst"
	KeyGlossSharpness              Key = "gloss.sharpness"
	KeyGlossHit1Time               Key = "gloss.hit1_time"
	KeyGlossHit2Time               Key = "gloss.hit2_time"
	KeyGlossHit3Time               Key = "gloss.hit3_time"
	KeyGlossHit4Time               Key = "gloss.hit4_time"
	KeyGlossHit1Level              Key = "gloss.hit1_level"
	KeyGlossHit2Level              Key = "gloss.hit2_level"
	KeyGlossHit3Level              Key = "gloss.hit3_level"
	KeyGlossHit4Level              Key = "gloss.hit4_level"
	KeyGlossVoices                 Key = "gloss.voices"
	KeyGlossMix                    Key = "gloss.mix"
	KeyGlossVoiceDrift             Key = "gloss.voice_drift"
	KeyGlossDriftDepth             Key = "gloss.drift_depth"
	KeyGlossKick                   Key = "gloss.kick"
	KeyGlossSnare                  Key = "gloss.snare"
	KeyGlossVariant                Key = "gloss.variant"
	KeyGlossHarmonicDecay          Key = "gloss.harmonic_decay"
	KeyGlossDropSpeed              Key = "gloss.drop_speed"
	KeyGlossPitchDrop              Key = "gloss.pitch_drop"
	KeyGlossModeSplit              Key = "gloss.mode_split"
	KeyGlossModeBody               Key = "gloss.mode_body"
	KeyGlossModeRing               Key = "gloss.mode_ring"
	KeyGlossBodyHold               Key = "gloss.body_hold"
	KeyGlossAir                    Key = "gloss.air"
	KeyGlossHarmonics              Key = "gloss.harmonics"
	KeyGlossPunchSpeed             Key = "gloss.punch_speed"
	KeyGlossLevel                  Key = "gloss.level"
	KeyGlossTail                   Key = "gloss.tail"
	KeyGlossFm                     Key = "gloss.fm"
	KeyGlossEnv                    Key = "gloss.env"
	KeyGlossDull                   Key = "gloss.dull"
	KeyGlossBright                 Key = "gloss.bright"
	// Phase-15 voice/choir knob glosses (Task 8): FORMANT + ENSEMBLE + RESONATOR.
	KeyGlossFormant      Key = "gloss.formant"
	KeyGlossModal        Key = "gloss.modal"
	KeyGlossStrike       Key = "gloss.strike"
	KeyGlossNoiseband    Key = "gloss.noiseband"
	KeyGlossStretch      Key = "gloss.stretch"
	KeyGlossDamping      Key = "gloss.damping"
	KeyGlossModalBright  Key = "gloss.modalbright"
	KeyGlossPartials     Key = "gloss.partials"
	KeyGlossDecayRate    Key = "gloss.decayrate"
	KeyGlossDecayTilt    Key = "gloss.decaytilt"
	KeyGlossTopDamping   Key = "gloss.topdamping"
	KeyGlossContact      Key = "gloss.contact"
	KeyGlossHardness     Key = "gloss.hardness"
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
	// Voice-realism knob glosses: ens_jitter (Roughness), formant_dry (Dry Blend).
	KeyGlossRoughness Key = "gloss.roughness"
	KeyGlossDryBlend  Key = "gloss.dry_blend"

	// Caption labels (drawn each frame near controls / overlays)
	KeyCapMove         Key = "cap.move"
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
	KeyNodeSecChord         Key = "node.sec.chord"
	KeyAudChord             Key = "aud.chord"
	KeyChordInversion       Key = "chord.inversion"
	KeyChordSpread          Key = "chord.spread"
	KeyChordSingleNote      Key = "chord.single_note"
	KeyChordInvRoot         Key = "chord.inv.root"
	KeyChordInv1            Key = "chord.inv.1"
	KeyChordInv2            Key = "chord.inv.2"
	KeyChordInv3            Key = "chord.inv.3"
	KeyChordInv4            Key = "chord.inv.4"
	KeyChordSpreadClose     Key = "chord.spread.close"
	KeyChordSpreadDrop2     Key = "chord.spread.drop2"
	KeyChordSpreadWide      Key = "chord.spread.wide"
	KeyChordMatrixTitle     Key = "chord.matrix.title"
	KeyChordPageTriads      Key = "chord.page.triads"
	KeyChordPageSevenths    Key = "chord.page.sevenths"
	KeyChordPageExt         Key = "chord.page.ext"
	KeyChordPageSus         Key = "chord.page.sus"

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
	KeyNotifCircuitLoading         Key = "notif.circuit_loading"
	KeyNotifCircuitReady           Key = "notif.circuit_ready"
	KeyNotifCircuitWarmupTimedOut  Key = "notif.circuit_warmup_timed_out"

	// KeyNotifImportedNamed names the loaded project/template + node/row counts;
	// KeyNotifInvalidBPMDetail names the rejected value and the valid range.
	KeyNotifImportedNamed    Key = "notif.imported_named"
	KeyNotifInvalidBPMDetail Key = "notif.invalid_bpm_detail"

	// One per undoable hooks.Kind (a drift test checks every undoable action has one).
	KeyActionChangeBPM          Key = "action.change_bpm"
	KeyActionChangeSubdivision  Key = "action.change_subdivision"
	KeyActionAddNode            Key = "action.add_node"
	KeyActionDeleteNode         Key = "action.delete_node"
	KeyActionPasteNode          Key = "action.paste_node"
	KeyActionReplaceNode        Key = "action.replace_node"
	KeyActionMoveNode           Key = "action.move_node"
	KeyActionChangeNodeType     Key = "action.change_node_type"
	KeyActionEditNode           Key = "action.edit_node"
	KeyActionSetStartNode       Key = "action.set_start_node"
	KeyActionAddEdge            Key = "action.add_edge"
	KeyActionDeleteEdge         Key = "action.delete_edge"
	KeyActionAddRow             Key = "action.add_row"
	KeyActionDeleteRow          Key = "action.delete_row"
	KeyActionChangeInstrument   Key = "action.change_instrument"
	KeyActionSetMasterVolume    Key = "action.set_master_volume"
	KeyActionAdjustEQ           Key = "action.adjust_eq"
	KeyActionAddEffect          Key = "action.add_effect"
	KeyActionRemoveEffect       Key = "action.remove_effect"
	KeyActionAdjustEffect       Key = "action.adjust_effect"
	KeyActionSetRowVolume       Key = "action.set_row_volume"
	KeyActionReorderEffect      Key = "action.reorder_effect"
	KeyActionToggleEffect       Key = "action.toggle_effect"
	KeyActionEditSynth          Key = "action.edit_synth"
	KeyActionResetSynth         Key = "action.reset_synth"
	KeyActionRecolorRow         Key = "action.recolor_row"
	KeyActionChangeCircuitTheme Key = "action.change_circuit_theme"
	KeyActionToggleEQFilter     Key = "action.toggle_eq_filter"
	KeyActionRenameInstrument   Key = "action.rename_instrument"
	KeyActionEditSample         Key = "action.edit_sample"
	KeyActionCreateGroup        Key = "action.create_group"
	KeyActionEditGroup          Key = "action.edit_group"
	KeyActionDeleteGroup        Key = "action.delete_group"

	// Localised DISPLAY labels only; the canonical English label behind hit-area tags and selectSynthSection stays in
	// internal/ui.
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
	KeySynthStageModal     Key = "synth.stage.modal"
	KeySynthStageStrike    Key = "synth.stage.strike"
	KeySynthStageNoise     Key = "synth.stage.noise"
	KeySynthStageFinish    Key = "synth.stage.finish"
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
	KeySynthStageModalSub     Key = "synth.stage.modal.sub"
	KeySynthStageStrikeSub    Key = "synth.stage.strike.sub"
	KeySynthStageNoiseSub     Key = "synth.stage.noise.sub"
	KeySynthStageFinishSub    Key = "synth.stage.finish.sub"

	// One key per distinct caption (params share, e.g. every "Wave"); the canonical English caption and the param ABI stay in
	// internal/ui.
	KeySynthControlTune      Key = "synth.knob.tune"
	KeySynthControlDecayMult Key = "synth.knob.decay-mult"
	KeySynthControlTone      Key = "synth.knob.tone"
	KeySynthControlDrive     Key = "synth.knob.drive"
	KeySynthControlBody      Key = "synth.knob.body"
	KeySynthControlBright    Key = "synth.knob.bright"
	KeySynthControlGain      Key = "synth.knob.gain"
	KeySynthControlWave      Key = "synth.knob.wave"
	KeySynthControlDetune    Key = "synth.knob.detune"
	KeySynthControlOctave    Key = "synth.knob.octave"
	KeySynthControlAttack    Key = "synth.knob.attack"
	KeySynthControlDecay     Key = "synth.knob.decay"
	KeySynthControlSustain   Key = "synth.knob.sustain"
	KeySynthControlRelease   Key = "synth.knob.release"
	KeySynthControlCurve     Key = "synth.knob.curve"
	KeySynthControlType      Key = "synth.knob.type"
	KeySynthControlCutoff    Key = "synth.knob.cutoff"
	KeySynthControlResonance Key = "synth.knob.resonance"
	KeySynthControlAlgorithm Key = "synth.knob.algorithm"
	KeySynthControlBaseFreq  Key = "synth.knob.base-freq"
	// FINISH stage knobs (the post ops + output limit).
	KeySynthControlResample      Key = "synth.knob.resample"
	KeySynthControlLength        Key = "synth.knob.length"
	KeySynthControlFadeSpeed     Key = "synth.knob.fade-speed"
	KeySynthControlLowBoost      Key = "synth.knob.low-boost"
	KeySynthControlCrisp         Key = "synth.knob.crisp"
	KeySynthControlDarken        Key = "synth.knob.darken"
	KeySynthControlCrunch        Key = "synth.knob.crunch"
	KeySynthControlLimit         Key = "synth.knob.limit"
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

	// The "OpN" / "Hit N" prefix stays English; only the noun is keyed. Depth/Decay reuse the knob keys.
	KeySynthNounRatio Key = "synth.noun.ratio"
	KeySynthNounLevel Key = "synth.noun.level"
	KeySynthNounTime  Key = "synth.noun.time"
	KeySynthNounHit   Key = "synth.noun.hit"

	// The canonical strings stay the storage form; these localise display and prefill. "FM" and "2-op" are universal and
	// keyless.
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

	// DIF is identical in both locales but routed for consistency; stage names keep their DAW English form.
	KeyChainOverlay      Key = "chain.overlay"
	KeyChainSplit        Key = "chain.split"
	KeyChainDiff         Key = "chain.diff"
	KeyChainStageSynth   Key = "chain.stage.synth"
	KeyChainStageAntiPop Key = "chain.stage.antipop"
	KeyChainStageFX      Key = "chain.stage.fx"
	KeyChainStageEQ      Key = "chain.stage.eq"

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
	// The half-row pill is too narrow for "Every %d rounds".
	KeyGroupEveryNShort   Key = "group.menu.everyn_short"
	KeyGroupDelete        Key = "group.menu.delete"
	KeyGroupSave          Key = "group.menu.save"
	KeyGroupParamPitch    Key = "group.menu.param.pitch"
	KeyGroupParamVolume   Key = "group.menu.param.volume"
	KeyGroupParamDuration Key = "group.menu.param.duration"
	// Opens a dropdown of the groups the selected node is NOT in.
	KeyGroupAddTo Key = "group.menu.add_to"
	// The always-present row that makes the selected node a new PROVISIONAL single-member group and opens its menu.
	KeyGroupCreateNew Key = "group.menu.create_new"
	// Opens a dropdown of the groups the selected node IS in; picking one removes that membership.
	KeyGroupRemoveFrom Key = "group.menu.remove_from"

	// Shown while a copied node is armed and awaiting placement.
	KeyBannerPasting Key = "banner.pasting"
	// Armed-mode banners take the subject's (i,j) as two %d verbs; KeyBannerEscHint is the desktop suffix (leading space)
	// when Profile().ShowEscHint.
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

	// Shown when an armed move would disconnect the circuit unhealably.
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
		KeyThemeTitle, KeyThemeScopeMine, KeyThemeScopeCircuit, KeyThemeOverride, KeyThemeResetAll, KeyThemeReset,
		KeyThemeCatBackground, KeyThemeCatSurfaces, KeyThemeCatText, KeyThemeCatAccent, KeyThemeCatFocus,
		KeyThemeCatGrid, KeyThemeCatNodes, KeyThemeCatDividers, KeyThemeCatViz,
		KeyThemeGlossBackground, KeyThemeGlossSurfaces, KeyThemeGlossText, KeyThemeGlossAccent, KeyThemeGlossFocus,
		KeyThemeGlossGrid, KeyThemeGlossNodes, KeyThemeGlossDividers, KeyThemeGlossViz,
		KeyThemeAdjust, KeyThemeCustomized,
		KeyThemePresetViceCity, KeyThemePresetViceCityGloss, KeyThemePresetOcean, KeyThemePresetOceanGloss,
		KeyThemePresetMidnight, KeyThemePresetMidnightGloss, KeyThemePresetEmber, KeyThemePresetEmberGloss,
		KeyThemePresetNeonMint, KeyThemePresetNeonMintGloss, KeyThemePresetOrchid, KeyThemePresetOrchidGloss,
		KeyShortcutPlayPause, KeyShortcutCancel, KeyShortcutPan, KeyShortcutZoom,
		KeyShortcutResetZoom, KeyShortcutBPM, KeyShortcutTabs, KeyShortcutHelp,
		KeyMenuRename, KeyMenuColor, KeyMenuOrigin, KeyMenuDelete,
		KeyMenuFile, KeyMenuUpload, KeyMenuImport, KeyMenuExport,
		KeyMenuLoadTemplate, KeyMenuTemplates, KeyMenuBack,
		KeyMenuCategories, KeyMenuFavorites, KeyRackAddInstrument,
		KeySave, KeySaveAs, KeyReset, KeySaveOverFmt, KeySaveAsNew, KeyNameTakenFmt, KeySaveFailed, KeyPreview, KeyCancel, KeyOK,
		KeyKeepRowSettings, KeyAlsoChangesRowFmt, KeyUseInstrumentSettingsFmt, KeyKeepEachRowSettings, KeyUseKitSettings, KeyKeepRowNSettingsFmt,
		KeyAddEffect, KeyLoadWAV, KeyNormalize, KeyReverse, KeyFade,
		KeyInstKick, KeyInstSnare, KeyInstHiHat, KeyInstClap, KeyInstTom,
		KeyTabWave, KeyTabSpectrum, KeyTabLevels, KeyTabEQ, KeyTabChain, KeyTabSynth, KeyTabSampler,
		KeyTabWaveShort, KeyTabSpectrumShort, KeyTabLevelsShort, KeyTabEQShort,
		KeyTabChainShort, KeyTabSynthShort, KeyTabSamplerShort,
		KeyNavPads,
		KeyLegendWave, KeyLegendSpectrum, KeyLegendMeters, KeyLegendEQ,
		KeySpectrumFindClipping, KeySpectrumFindCollision, KeySpectrumFindPileUp,
		KeySpectrumFindDeadHigh, KeySpectrumFindDeadLow, KeySpectrumFindOK, KeyAnd,
		KeyChainNextTapA, KeyChainNextTapB, KeyChainNextTapReplace, KeyChainLegendUnset,
		KeyChainGlossOverlay, KeyChainGlossSplit, KeyChainGlossDiff,
		KeyChainGlossFreeze,
		KeyChainFreeze, KeyChainLive,
		KeyLegendScope, KeyLegendSynth, KeyLegendSampler,
		KeyGlossVoice, KeyGlossOsc, KeyGlossPitch, KeyGlossWaveform, KeyGlossGenerator,
		KeyGlossTune, KeyGlossGlide, KeyGlossBend, KeyGlossDetune,
		KeyGlossEnvelope, KeyGlossAttack, KeyGlossDecay, KeyGlossSustain, KeyGlossRelease,
		KeyGlossTone, KeyGlossCutoff, KeyGlossFilter, KeyGlossReso, KeyGlossResonance, KeyGlossBody,
		KeyGlossResample, KeyGlossFadeSpeed, KeyGlossLowBoost, KeyGlossDarken, KeyGlossCrunch, KeyGlossLimit, KeyGlossCrisp,
		KeyGlossBasePitch, KeyGlossPitchSweep, KeyGlossSweepTime, KeyGlossOpRatio,
		KeyGlossOpFMDepth, KeyGlossOpDecay,
		KeyGlossFundamental, KeyGlossSweepSpeed, KeyGlossBoomDecay, KeyGlossBodyDecay,
		KeyGloss2ndHarmonic, KeyGloss3rdHarmonic, KeyGloss4thHarmonic, KeyGlossClick, KeyGlossThud,
		KeyGlossRingDecay, KeyGlossOvertone1, KeyGlossOvertone2, KeyGlossStick, KeyGlossRoom,
		KeyGlossTone2, KeyGlossNoiseTune, KeyGlossToneDecay, KeyGlossNoiseDecay, KeyGlossTailDecay,
		KeyGlossToneLevel, KeyGlossNoiseLevel, KeyGlossWires, KeyGlossSnap,
		KeyGlossMetalTune, KeyGlossAttackDecay, KeyGlossMetalLevel, KeyGlossSizzleLevel, KeyGlossSizzleDecay,
		KeyGlossPlateModes, KeyGlossLowestMode, KeyGlossHighestMode, KeyGlossHighsFade, KeyGlossTickCorner, KeyGlossShimmer, KeyGlossStrikeTone, KeyGlossStrikeWidth, KeyGlossBodyTone, KeyGlossBodyWidth, KeyGlossSparkle, KeyGlossSparkleTone, KeyGlossSparkleWidth, KeyGlossWash, KeyGlossSoftness,
		KeyGlossRattle, KeyGlossRattleSpeed, KeyGlossRattleLength, KeyGlossWireDelay, KeyGlossBuzzHold,
		KeyGlossPluck, KeyGlossPick, KeyGlossFade, KeyGlossLength, KeyGlossOvertone, KeyGlossPitchPunch, KeyGlossPunch,
		KeyGlossDrive, KeyGlossGain, KeyGlossSaturation, KeyGlossSat, KeyGlossDistortion,
		KeyGlossSlope, KeyGlossSlopes, KeyGlossPre, KeyGlossPost, KeyGlossPeak, KeyGlossRMS,
		KeyGlossClip, KeyGlossClips, KeyGlossHeadroom, KeyGlossLUFS, KeyGlossK20,
		KeyGlossOverlay, KeyGlossSplit, KeyGlossDiff, KeyGlossOvr, KeyGlossSpl, KeyGlossDif,
		KeyGlossAG, KeyGlossAutoGain,
		KeyGlossOut, KeyGlossDelay, KeyGlossReverb, KeyGlossSend,
		KeyGlossPlay, KeyGlossStop, KeyGlossPause,
		KeySynthAdvanced,
		KeySynthCatSound, KeySynthCatSoundGloss, KeySynthCatBrightness, KeySynthCatBrightnessGloss, KeySynthCatAttack, KeySynthCatAttackGloss, KeySynthCatLength, KeySynthCatLengthGloss, KeySynthCatWidth, KeySynthCatWidthGloss, KeySynthCatHumanFeel, KeySynthCatHumanFeelGloss, KeySynthCatMovement, KeySynthCatMovementGloss, KeySynthCatBody, KeySynthCatBodyGloss, KeySynthCatWarmth, KeySynthCatWarmthGloss, KeySynthCatPitchDrop, KeySynthCatPitchDropGloss, KeySynthCatVoice, KeySynthCatVoiceGloss, KeySynthCatModes, KeySynthCatModesGloss, KeySynthCatMallet, KeySynthCatMalletGloss, KeySynthCatFlam, KeySynthCatFlamGloss, KeySynthCatPunch, KeySynthCatPunchGloss, KeySynthCatBoom, KeySynthCatBoomGloss, KeySynthCatTone, KeySynthCatToneGloss, KeySynthCatSource, KeySynthCatSourceGloss, KeySynthCatGrit, KeySynthCatGritGloss, KeySynthCatLengthPercGloss, KeySynthMore, KeySynthAddStage, KeyCapSynthTitle,
		KeySynthDialRichness, KeySynthDialRichnessGloss, KeySynthDialBite, KeySynthDialBiteGloss, KeySynthDialBright, KeySynthDialBrightGloss, KeySynthDialSizzle, KeySynthDialSizzleGloss, KeySynthDialEdge, KeySynthDialEdgeGloss, KeySynthDialSweep, KeySynthDialSweepGloss, KeySynthDialFadein, KeySynthDialFadeinGloss, KeySynthDialSnap, KeySynthDialSnapGloss, KeySynthDialLength, KeySynthDialLengthGloss, KeySynthDialRoom, KeySynthDialRoomGloss, KeySynthDialHold, KeySynthDialHoldGloss, KeySynthDialTail, KeySynthDialTailGloss, KeySynthDialSpread, KeySynthDialSpreadGloss, KeySynthDialBlend, KeySynthDialBlendGloss, KeySynthDialDrift, KeySynthDialDriftGloss, KeySynthDialVibrato, KeySynthDialVibratoGloss, KeySynthDialLooseness, KeySynthDialLoosenessGloss, KeySynthDialDepth, KeySynthDialDepthGloss, KeySynthDialSpeedLfo, KeySynthDialSpeedLfoGloss, KeySynthDialDelay, KeySynthDialDelayGloss, KeySynthDialBody, KeySynthDialBodyGloss, KeySynthDialExpression, KeySynthDialExpressionGloss, KeySynthDialWeight, KeySynthDialWeightGloss, KeySynthDialRing, KeySynthDialRingGloss, KeySynthDialDrive, KeySynthDialDriveGloss, KeySynthDialDrop, KeySynthDialDropGloss, KeySynthDialSpeedDrop, KeySynthDialSpeedDropGloss, KeySynthDialCharacter, KeySynthDialCharacterGloss, KeySynthDialBreath, KeySynthDialBreathGloss, KeySynthDialShift, KeySynthDialShiftGloss, KeySynthDialSpreadFlam, KeySynthDialSpreadFlamGloss, KeySynthDialStrength, KeySynthDialStrengthGloss, KeySynthDialSharp, KeySynthDialSharpGloss, KeySynthDialGlassy, KeySynthDialGlassyGloss, KeySynthDialShimmer, KeySynthDialShimmerGloss, KeySynthDialBloom, KeySynthDialBloomGloss, KeySynthDialClick, KeySynthDialClickGloss, KeySynthDialAir, KeySynthDialAirGloss, KeySynthDialShine, KeySynthDialShineGloss, KeySynthDialTalk, KeySynthDialTalkGloss,
		KeySynthDialRichnessLo, KeySynthDialRichnessHi, KeySynthDialBiteLo, KeySynthDialBiteHi, KeySynthDialGlassyLo, KeySynthDialGlassyHi, KeySynthDialBrightLo, KeySynthDialBrightHi, KeySynthDialSizzleLo, KeySynthDialSizzleHi, KeySynthDialShimmerLo, KeySynthDialShimmerHi, KeySynthDialEdgeLo, KeySynthDialEdgeHi, KeySynthDialSweepLo, KeySynthDialSweepHi, KeySynthDialBloomLo, KeySynthDialBloomHi, KeySynthDialFadeinLo, KeySynthDialFadeinHi, KeySynthDialClickLo, KeySynthDialClickHi, KeySynthDialSnapLo, KeySynthDialSnapHi, KeySynthDialLengthLo, KeySynthDialLengthHi, KeySynthDialRoomLo, KeySynthDialRoomHi, KeySynthDialTailLo, KeySynthDialTailHi, KeySynthDialHoldLo, KeySynthDialHoldHi, KeySynthDialSpreadLo, KeySynthDialSpreadHi, KeySynthDialBlendLo, KeySynthDialBlendHi, KeySynthDialDriftLo, KeySynthDialDriftHi, KeySynthDialVibratoLo, KeySynthDialVibratoHi, KeySynthDialLoosenessLo, KeySynthDialLoosenessHi, KeySynthDialDepthLo, KeySynthDialDepthHi, KeySynthDialSpeedLfoLo, KeySynthDialSpeedLfoHi, KeySynthDialDelayLo, KeySynthDialDelayHi, KeySynthDialBodyLo, KeySynthDialBodyHi, KeySynthDialExpressionLo, KeySynthDialExpressionHi, KeySynthDialWeightLo, KeySynthDialWeightHi, KeySynthDialRingLo, KeySynthDialRingHi, KeySynthDialAirLo, KeySynthDialAirHi, KeySynthDialDriveLo, KeySynthDialDriveHi, KeySynthDialDropLo, KeySynthDialDropHi, KeySynthDialSpeedDropLo, KeySynthDialSpeedDropHi, KeySynthDialCharacterLo, KeySynthDialCharacterHi, KeySynthDialBreathLo, KeySynthDialBreathHi, KeySynthDialShiftLo, KeySynthDialShiftHi, KeySynthDialShineLo, KeySynthDialShineHi, KeySynthDialTalkLo, KeySynthDialTalkHi, KeySynthDialSpreadFlamLo, KeySynthDialSpreadFlamHi, KeySynthDialStrengthLo, KeySynthDialStrengthHi, KeySynthDialSharpLo, KeySynthDialSharpHi, KeyGraphViewOverview, KeyGraphViewLoudness, KeyGraphViewAttack, KeyGraphViewPitch, KeyGraphViewColour, KeyGraphViewHarmonics, KeyGraphViewColourTime, KeyGraphViewNoise, KeyGraphViewWave, KeyGraphSoundsFor, KeyGraphFullAfter, KeyGraphPitchGlide, KeyGraphPitchWobble, KeyGraphPitchHolds, KeyGraphNoPitch, KeyGraphBrightCentre, KeyGraphHarmonicsCount, KeyGraphCentreMoves, KeyGraphNoiseShare, KeyGraphWaveAt, KeyGraphWas, KeyGraphNoteEnds, KeyGraphBefore, KeyGraphUpdating, KeyGraphNoPicture, KeyGraphSamplerEdit, KeyGraphLow, KeyGraphHigh, KeyGraphQuiet, KeyGraphLoud, KeyGraphTone, KeyGraphNoise, KeyGraphSummary, KeyGraphViewPick, KeyGraphAxisDark, KeyGraphAxisBright, KeyGraphDetails, KeyGraphFullAfterClick, KeyGraphWaveCrest, KeyGraphWordsLenHit, KeyGraphWordsLenShort, KeyGraphWordsLenMedium, KeyGraphWordsLenLong, KeyGraphWordsLenVeryLong, KeyGraphWordsToTheEnd, KeyGraphWordsAtkInstant, KeyGraphWordsAtkQuick, KeyGraphWordsAtkGentle, KeyGraphWordsAtkSlow, KeyGraphWordsWithClick, KeyGraphWordsPitchDropFast, KeyGraphWordsPitchDrop, KeyGraphWordsPitchDropSlow, KeyGraphWordsPitchRise, KeyGraphWordsPitchWobbleGentle, KeyGraphWordsPitchWobbleLot, KeyGraphWordsPitchSteady, KeyGraphWordsDeep, KeyGraphWordsWarm, KeyGraphWordsBalanced, KeyGraphWordsBright, KeyGraphWordsVeryBright, KeyGraphWordsPure, KeyGraphWordsSomeOvertones, KeyGraphWordsRich, KeyGraphWordsGetsDarker, KeyGraphWordsGetsBrighter, KeyGraphWordsKeepsColour, KeyGraphWordsClean, KeyGraphWordsSomeHiss, KeyGraphWordsHalfNoise, KeyGraphWordsMostlyNoise, KeyGraphWordsSmooth, KeyGraphWordsSlightlySquared, KeyGraphWordsSquared, KeyGraphWordsLonger, KeyGraphWordsShorter, KeyGraphWordsSnappier, KeyGraphWordsSofter, KeyGraphWordsBiggerDrop, KeyGraphWordsSmallerDrop, KeyGraphWordsMoreWobble, KeyGraphWordsLessWobble, KeyGraphWordsBrighter, KeyGraphWordsDarker, KeyGraphWordsRicher, KeyGraphWordsPlainer, KeyGraphWordsNoisier, KeyGraphWordsCleaner, KeyGraphWordsGrittier, KeyGraphWordsSmoother,
		KeyGlossSaxBlow, KeyGlossSaxReed, KeyGlossSaxReedStiff, KeyGlossSaxBell, KeyGlossSaxBreath, KeyGlossSaxBright, KeyGlossBowPos, KeyGlossBowPress, KeyGlossBowSpeed, KeyGlossBowBright,
		KeyGlossOscillator, KeyGlossOctave, KeyGlossAlgorithm, KeyGlossOp1Ratio, KeyGlossOp2Ratio, KeyGlossOp3Ratio, KeyGlossOp4Ratio, KeyGlossOp1Depth, KeyGlossOp2Depth, KeyGlossOp3Depth, KeyGlossOp4Depth, KeyGlossOp1Level, KeyGlossOp2Level, KeyGlossOp3Level, KeyGlossOp4Level, KeyGlossCurve, KeyGlossFilterEnv, KeyGlossEnvAmount, KeyGlossEnvRise, KeyGlossEnvDecay, KeyGlossLfo, KeyGlossRate, KeyGlossDepth, KeyGlossLfoTarget, KeyGlossVibratoDelay, KeyGlossPitchEnv, KeyGlossSweep, KeyGlossBurst, KeyGlossSharpness, KeyGlossHit1Time, KeyGlossHit2Time, KeyGlossHit3Time, KeyGlossHit4Time, KeyGlossHit1Level, KeyGlossHit2Level, KeyGlossHit3Level, KeyGlossHit4Level, KeyGlossVoices, KeyGlossMix, KeyGlossVoiceDrift, KeyGlossDriftDepth, KeyGlossKick, KeyGlossSnare, KeyGlossVariant, KeyGlossHarmonicDecay, KeyGlossDropSpeed, KeyGlossPitchDrop, KeyGlossModeSplit, KeyGlossModeBody, KeyGlossModeRing, KeyGlossBodyHold, KeyGlossAir, KeyGlossHarmonics, KeyGlossPunchSpeed, KeyGlossLevel, KeyGlossTail, KeyGlossFm, KeyGlossEnv,
		KeyGlossDull, KeyGlossBright,
		KeyGlossFormant,
		KeyGlossModal, KeyGlossStrike, KeyGlossNoiseband,
		KeyGlossStretch, KeyGlossDamping, KeyGlossModalBright, KeyGlossPartials, KeyGlossDecayRate, KeyGlossDecayTilt, KeyGlossTopDamping, KeyGlossContact, KeyGlossHardness,
		KeyGlossVowel, KeyGlossVoiceType, KeyGlossBreath, KeyGlossShine, KeyGlossHeadSize,
		KeyGlossMorphSpeed, KeyGlossMorphTo, KeyGlossScatter, KeyGlossVibratoRate,
		KeyGlossVibratoDepth, KeyGlossHumanize, KeyGlossBodyModel, KeyGlossBodyMix, KeyGlossBowDynamics,
		KeyGlossRoughness, KeyGlossDryBlend,
		KeyCapMove, KeyCapCopy, KeyCapConnect, KeyCapYourSound, KeyCapUpClose,
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
		KeyNodeSecChord, KeyAudChord, KeyChordInversion, KeyChordSpread, KeyChordSingleNote, KeyChordInvRoot, KeyChordInv1, KeyChordInv2, KeyChordInv3, KeyChordInv4, KeyChordSpreadClose, KeyChordSpreadDrop2, KeyChordSpreadWide, KeyChordMatrixTitle, KeyChordPageTriads, KeyChordPageSevenths, KeyChordPageExt, KeyChordPageSus,
		KeyNotifInvalidName, KeyNotifRenamedInstrument, KeyNotifErrLoadJSON, KeyNotifImported,
		KeyNotifRecordingFailed, KeyNotifSavingRecording, KeyNotifCannotStartRecording,
		KeyNotifRecordingSaveFailed, KeyNotifRecordingSaved, KeyNotifImportCanceled,
		KeyNotifErrLoadWAV, KeyNotifSelectedWAV, KeyNotifCannotConnectInvisible,
		KeyNotifOnlyPerpendicular, KeyNotifInstNameEmpty, KeyNotifUpdatedWAVInst,
		KeyNotifLoadedWAVInst, KeyNotifInvalidBPM,
		KeyNotifUndo, KeyNotifRedo,
		KeyNotifUndoDetail, KeyNotifRedoDetail,
		KeyNotifCircuitLoading, KeyNotifCircuitReady, KeyNotifCircuitWarmupTimedOut,
		KeyNotifImportedNamed, KeyNotifInvalidBPMDetail,
		KeyActionChangeBPM, KeyActionChangeSubdivision, KeyActionAddNode, KeyActionDeleteNode,
		KeyActionPasteNode, KeyActionReplaceNode,
		KeyActionMoveNode, KeyActionChangeNodeType, KeyActionEditNode, KeyActionSetStartNode,
		KeyActionAddEdge, KeyActionDeleteEdge, KeyActionAddRow, KeyActionDeleteRow,
		KeyActionChangeInstrument, KeyActionSetMasterVolume, KeyActionAdjustEQ, KeyActionAddEffect,
		KeyActionRemoveEffect, KeyActionAdjustEffect, KeyActionSetRowVolume, KeyActionReorderEffect,
		KeyActionToggleEffect, KeyActionEditSynth, KeyActionResetSynth, KeyActionRecolorRow, KeyActionChangeCircuitTheme,
		KeyActionToggleEQFilter, KeyActionRenameInstrument, KeyActionEditSample,
		KeyActionCreateGroup, KeyActionEditGroup, KeyActionDeleteGroup,
		KeySynthStageVoice, KeySynthStageOsc, KeySynthStageFM, KeySynthStagePitch,
		KeySynthStageLFO, KeySynthStageBurst, KeySynthStageEnvelope, KeySynthStageFilter,
		KeySynthStageFilterEnv, KeySynthStagePost, KeySynthStageTone,
		KeySynthStageEnsemble, KeySynthStageFormant, KeySynthStageResonator, KeySynthStageFinish,
		KeySynthStageModal, KeySynthStageStrike, KeySynthStageNoise,
		KeySynthStageEnvelopeShort, KeySynthStageFilterEnvShort,
		KeySynthStageEnsembleShort, KeySynthStageFormantShort, KeySynthStageResonatorShort,
		KeySynthStageVoiceSub, KeySynthStageOscSub, KeySynthStageFMSub, KeySynthStagePitchSub,
		KeySynthStageLFOSub, KeySynthStageBurstSub, KeySynthStageEnvelopeSub, KeySynthStageFilterSub,
		KeySynthStageFilterEnvSub, KeySynthStagePostSub, KeySynthStageToneSub,
		KeySynthStageEnsembleSub, KeySynthStageFormantSub, KeySynthStageResonatorSub, KeySynthStageFinishSub,
		KeySynthStageModalSub, KeySynthStageStrikeSub, KeySynthStageNoiseSub,
		KeySynthControlResample, KeySynthControlLength, KeySynthControlFadeSpeed, KeySynthControlLowBoost,
		KeySynthControlCrisp, KeySynthControlDarken, KeySynthControlCrunch, KeySynthControlLimit,
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
		KeyChainOverlay, KeyChainSplit, KeyChainDiff,
		KeyChainStageSynth, KeyChainStageAntiPop, KeyChainStageFX, KeyChainStageEQ,
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
