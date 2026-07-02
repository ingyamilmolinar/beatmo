# Reference WAVs — University of Iowa Musical Instrument Samples

**Source**: University of Iowa Musical Instrument Samples (MIS)
**URL**: https://theremin.music.uiowa.edu/
**License**: "These samples may be downloaded and used for any projects, without restrictions." (public-domain-equivalent)

## Format

Original files are AIFF (24-bit stereo, 44100 Hz, ~3 s single-note recordings).
Each file was converted to **16-bit mono WAV, capped at 2 seconds** using
`scripts/aiff_to_wav.py` (Python stdlib only — no ffmpeg/sox).

Regenerate via:
```
bash scripts/fetch_synth_refs.sh
```

## File table

| file | instrument | note | source URL | sha256 |
|------|------------|------|------------|--------|
| violin_A4.wav | violin | A4 | https://theremin.music.uiowa.edu/sound%20files/MIS%20Pitches%20-%202014/Strings/Violin/Violin.arco.ff.sulA.A4.stereo.aif | fb768f6dbcec02ccff2abcb3bf3a3f8b5bb3ec60babde30b8f8448cfffb6a95c |
| flute_A4.wav | flute | A4 | https://theremin.music.uiowa.edu/sound%20files/MIS%20Pitches%20-%202014/Woodwinds/Flute/Flute.nonvib.ff.A4.stereo.aif | e181d3be967d3f198aad27504e3dbb0a3af38517cbf4710ed1d1b187b63a7941 |
| cello_A3.wav | cello | A3 | https://theremin.music.uiowa.edu/sound%20files/MIS%20Pitches%20-%202014/Strings/Cello/Cello.arco.ff.sulA.A3.stereo.aif | 799ea7385546e95843c8c65cabb805b90688be514641ca2a61d826def61ddc8e |
| oboe_A4.wav | oboe | A4 | https://theremin.music.uiowa.edu/sound%20files/MIS%20Pitches%20-%202014/Woodwinds/Oboe/Oboe.ff.A4.stereo.aif | 90744e1f567e841e7a9e6f9ca48c1097266d93f35eda974043f055dabc8836d2 |
| trumpet_A4.wav | trumpet | A4 | https://theremin.music.uiowa.edu/sound%20files/MIS%20Pitches%20-%202014/Brass/BbTrumpet/Trumpet.novib.ff.A4.stereo.aif | ce2e889179ebc6c508eaa4a70ffb9f63e02311a0af7b00cde1dac659a227d48b |
