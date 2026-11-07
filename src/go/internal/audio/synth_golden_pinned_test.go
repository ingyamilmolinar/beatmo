//go:build !test && !js

package audio

import (
	"slices"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// The byte-identity net: any change to a single output sample of any instrument fails here. Regenerate ONLY on an
// intentional sound change, with TestSynthGoldenIdentityCapture, and say why in the commit.
var synthGoldens = map[instrument.ID]struct {
	n    int
	bits uint64
}{
	roster.Accordion:          {48000, 0x8368832bf176ce5e},
	roster.Banjo:              {36000, 0x5c0010e9f1ae9e29},
	roster.Bass808:            {36000, 0x3d92af2d6f49a54c},
	roster.BassAcid:           {36000, 0xe7d4a8266f01038c},
	roster.BassFm:             {36000, 0xdf93e155683ccf96},
	roster.BassGuitar:         {36000, 0x268f584b9ceece30},
	roster.BassReese:          {36000, 0x4f5e7ad49dae0ec8},
	roster.BassWobble:         {36000, 0x6599d385fbb6f06e},
	roster.Bassoon:            {48000, 0x7edf4edba6928aff},
	roster.BrassStab:          {36000, 0x6ad3af8ffc740e4e},
	roster.Celesta:            {48000, 0x43aa3c708f4fb2e1},
	roster.Cello:              {48000, 0xe454b408cca141ff},
	roster.CelloWarm:          {48000, 0xc3dd294e1ec968af},
	roster.Clap:               {24000, 0xdbc5a3e61b996645},
	roster.ClapBright:         {6000, 0x70b9f09962a8c491},
	roster.ClapLofi:           {7200, 0x35921ee4c5ff01a1},
	roster.ClapTight:          {6000, 0x07a8a183b0c84f36},
	roster.Clarinet:           {48000, 0xc0f12eae58fffa24},
	roster.Clavinet:           {36000, 0xb647596ebe26d7ba},
	roster.Conga:              {12000, 0xce3ed2996d6705b6},
	roster.CongaOpen:          {12000, 0x48f7a7bca343abbd},
	roster.CongaOrganic:       {38400, 0x062ffa17befc589c},
	roster.CongaOrganicDry:    {38400, 0xacb17fc683b15759},
	roster.CongaTumba:         {12000, 0x7b6e083992e4f1d8},
	roster.Cowbell:            {19200, 0x0f57270c96b54f9c},
	roster.CowbellDrive:       {7200, 0x649f4b134ebfb82c},
	roster.CowbellLofi:        {9600, 0xeeb009acc6d64fbc},
	roster.Crash:              {72000, 0x12120f57b5a652ff},
	roster.DnbKick:            {24000, 0x990f9c6793ba4aeb},
	roster.DoubleBass:         {48000, 0xf6c78cd5e2a9faff},
	roster.EnsembleLead:       {48000, 0xcb0e4859d3628b48},
	roster.EnsembleLeadDark:   {48000, 0xfccaa540395c0a02},
	roster.Flute:              {48000, 0x4393e8c6c0ab19dc},
	roster.FluteBreathy:       {48000, 0xf24a4c2fefa9e408},
	roster.FmBass:             {36000, 0xf2263013bf2675b4},
	roster.FmBassDrive:        {24000, 0xe2343f45c4fc2e60},
	roster.FmBell:             {48000, 0x9754f16f0f183694},
	roster.FmBellSoft:         {36000, 0xdc5bb9592083d2f7},
	roster.FmEpiano:           {48000, 0x55005c382044c808},
	roster.FmEpianoDrive:      {36000, 0x220cf63508bd9730},
	roster.FmLead:             {24000, 0xb53fad81c177f20e},
	roster.FmLeadCrush:        {16800, 0x67381a76076dbb17},
	roster.FmPluck:            {12000, 0x195865bfed74f8f2},
	roster.FmPluckTight:       {7200, 0x5f737b992cf17e35},
	roster.FrenchHorn:         {48000, 0xf3ab6b6594cb2d01},
	roster.FrenchHornLoud:     {48000, 0xc00cc84a77102d5d},
	roster.Gamelan:            {48000, 0xe91f8106b8c6ed08},
	roster.GhostBass:          {48000, 0x105697ce13c22432},
	roster.GlassArmonica:      {60000, 0x0c5bdd25911541f1},
	roster.GlassWash:          {60000, 0xa59ed34eae8a1c21},
	roster.Glockenspiel:       {48000, 0xca63a1dce1f7ac47},
	roster.GuitarElectric:     {36000, 0x2d040e7bb45b0ed8},
	roster.GuitarElectricNeck: {36000, 0x4c26b085e59b2d75},
	roster.GuitarNylon:        {36000, 0x4b3c38e92e710122},
	roster.GuitarNylonBright:  {36000, 0x44d71a44485e3422},
	roster.GuitarSteel:        {36000, 0x023ba1d656ed6116},
	roster.GuitarSteelWarm:    {36000, 0x13448b1d2314aaf9},
	roster.Harmonium:          {48000, 0x10eccf8f5fd1f4a9},
	roster.Harp:               {48000, 0xde349788a37ece32},
	roster.Harpsichord:        {48000, 0x38c59b72db78275e},
	roster.HighTomOrganic:     {21600, 0x625952d074d3b120},
	roster.Hihat:              {12000, 0x27e1ab853ab144de},
	roster.HihatLofi:          {4800, 0xd2309782c366bf82},
	roster.HihatOpenBright:    {16800, 0x93d6c14fb3126f0f},
	roster.HihatPedal:         {3600, 0xe731aeaf4f715c63},
	roster.Kalimba:            {36000, 0xe50985121b6fb76e},
	roster.Kick:               {24000, 0x6e45e1f487456d5e},
	roster.Kick808:            {36000, 0x6bd253957fb58463},
	roster.KickAcoustic:       {31200, 0x96d998e5c01df7f9},
	roster.KickAcousticHeavy:  {31200, 0x3970fee68faf349d},
	roster.KickAiry:           {24000, 0x20022d0201e5251e},
	roster.KickBoomBap:        {24000, 0x9de07a6a93e38c24},
	roster.KickBrutal:         {24000, 0x612d75a76f34dae4},
	roster.KickDeep:           {38400, 0x19aa8d873aed2632},
	roster.KickElectro:        {19200, 0x88b62790a908db40},
	roster.KickFat:            {24000, 0x90894630bceaec07},
	roster.KickLofi:           {12000, 0x8fc6fb760319f161},
	roster.KickMuted:          {24000, 0xc4cd307008111d2e},
	roster.KickMutedRoom:      {24000, 0x02780284c45ae387},
	roster.KickOrganicTight:   {24000, 0x41b98b247e2f0e43},
	roster.KickPunchy:         {12000, 0x160688cf5947a7d2},
	roster.KickRock:           {24000, 0xa110016d823ac450},
	roster.KickSnap:           {12000, 0xd4882305d50f7047},
	roster.KickTight:          {7200, 0xd8e8ce6b52fce823},
	roster.KickWood:           {24000, 0x7f220cfde47dc7ef},
	roster.Koto:               {48000, 0x8493fdd08b490370},
	roster.LeadChip:           {36000, 0xb5136907314d9bf6},
	roster.LeadHoover:         {48000, 0xe6fc649c49ceb4ad},
	roster.LeadSupersaw:       {48000, 0xedea4156b2365a9b},
	roster.Mandolin:           {36000, 0xdbb04e2b0ee8a3c1},
	roster.Marimba:            {36000, 0x6618933cdc34885d},
	roster.Modular:            {24000, 0xe21a3a540f3e9779},
	roster.ModularPad:         {24000, 0x3123d25a65731a2a},
	roster.MoogBass:           {36000, 0x038f2d02c45dbde7},
	roster.MusicBox:           {48000, 0xb95836131d01beac},
	roster.MutedTrumpet:       {48000, 0x6cba4cc3570b4ce5},
	roster.Oboe:               {48000, 0xc4df4f2186afa4c1},
	roster.OboeFull:           {48000, 0x195356cead56eb47},
	roster.Organ:              {48000, 0x082a2d709cde5ccf},
	roster.OrganChurch:        {48000, 0x4c79461388ecd694},
	roster.PanFlute:           {48000, 0xf95f809cd98c47a7},
	roster.PianoFelt:          {48000, 0x9544a8e5e77de7eb},
	roster.PianoGrand:         {48000, 0x9e9c1e0fa5596656},
	roster.PianoRhodes:        {48000, 0xb970c026fabc3c8e},
	roster.Piccolo:            {48000, 0x1c292dd035b53b58},
	roster.PizzStrings:        {36000, 0x26ac51ea45595321},
	roster.PwmPad:             {60000, 0x8dcb0790528017d1},
	roster.RawKick:            {16800, 0xd5f5b990bc2b8d3d},
	roster.Ride:               {86400, 0xd6312cf1ee78b301},
	roster.Rimshot:            {14400, 0xe753508fb8782670},
	roster.Sax:                {48000, 0x0e24230aa6021d31},
	roster.ScifiLead:          {48000, 0x83054709823b8098},
	roster.Shaker:             {14400, 0x8a91b38a1bc5e421},
	roster.Sidestick:          {12000, 0x7dce50117dc6989a},
	roster.SingingBowl:        {72000, 0xeb022def20591abc},
	roster.Sitar:              {48000, 0x5125332c02ed5b8f},
	roster.Snare:              {48000, 0xb8c0a015938c6ff1},
	roster.SnareAcoustic:      {67200, 0xefc8434f4e6cdda2},
	roster.SnareCrack:         {12000, 0xda8851610f7bb9c4},
	roster.SnareDeep:          {67200, 0x3b2251186c50bf02},
	roster.SnareFat:           {48000, 0x5b8f006376e261b9},
	roster.SnareGhost:         {9600, 0xc77e6668b4ca9bec},
	roster.SnareLofi:          {14400, 0xc0f808b51f2be12d},
	roster.SnarePunch:         {43200, 0x65fc2aa27e1afd3f},
	roster.SteelDrum:          {48000, 0x426141a3119e5394},
	roster.SubBass:            {48000, 0x5bc704ef07a047f7},
	roster.SubBassDrive:       {36000, 0x86039ed873ebc410},
	roster.Theremin:           {48000, 0xe5c4d5a10973bd09},
	roster.Tom:                {24000, 0xbff346d1ae230201},
	roster.TomHighAcoustic:    {67200, 0xfd1f1ea3d701896b},
	roster.TomHighDrive:       {7200, 0x0af7e2331d434a81},
	roster.TomLofi:            {16800, 0x5cf3da4e95d02d20},
	roster.Trombone:           {48000, 0xd90da067d236f9f9},
	roster.Trumpet:            {48000, 0xe1b14e0253806cc1},
	roster.TrumpetMellow:      {48000, 0xeb58381edb27e123},
	roster.Tuba:               {48000, 0x3557c87eccc80a9b},
	roster.TubularBells:       {72000, 0xd2fb8a90a579779f},
	roster.Ukulele:            {36000, 0x2b782b6456bb4c06},
	roster.Vibraphone:         {60000, 0xb969c3fd6c5b27e6},
	roster.Viola:              {48000, 0xde767767c5e314f0},
	roster.ViolaPad:           {48000, 0x1cd2a30c50bd72c1},
	roster.Violin:             {48000, 0xe16eb6d397a4c34c},
	roster.ViolinEnsemble:     {48000, 0xb0992e5b6f3bc838},
	roster.VoiceAhh:           {60000, 0xae8da2253d806c1b},
	roster.VoiceAlien:         {60000, 0x6f4d7dc7721196ee},
	roster.VoiceOpera:         {60000, 0x3bca036e1842bf0e},
	roster.VoiceSoprano:       {48000, 0x4b21d62ef7f20c69},
	roster.VoiceWhisper:       {48000, 0xf6ed5645bd195e72},
	roster.Xylophone:          {24000, 0x367259232b4c81f4},
	roster.ZgumpKick:          {16800, 0x950b42adba52e0c8},
}

func TestSynthGoldenByteIdentity(t *testing.T) {
	Reset()
	ResetInstruments()
	ids := append([]instrument.ID(nil), roster.All...)
	slices.Sort(ids)
	seen := map[string]bool{}
	for _, id := range ids {
		seen[string(id)] = true
		want, ok := synthGoldens[id]
		if !ok {
			t.Errorf("%q has no pinned golden — add it (an intentional new instrument?)", id)
			continue
		}
		n, bits := synthRenderChecksum(string(id))
		if n != want.n || bits != want.bits {
			t.Errorf("%q render changed: got {%d, 0x%016x} want {%d, 0x%016x}", id, n, bits, want.n, want.bits)
		}
	}
	for id := range synthGoldens {
		if !seen[string(id)] {
			t.Errorf("pinned golden %q is no longer a built-in instrument (removed/renamed?)", id)
		}
	}
}
