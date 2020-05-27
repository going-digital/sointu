%define BPM 100

%include "../src/sointu.inc"

BEGIN_PATTERNS
    PATTERN 64, HLD, HLD, HLD, HLD, HLD, HLD, HLD,  0, 0, 0, 0, 0, 0, 0, 0
END_PATTERNS

BEGIN_TRACKS
    TRACK VOICES(1),0
END_TRACKS

BEGIN_PATCH
    BEGIN_INSTRUMENT VOICES(1) ; Instrument0
        SU_LOADVAL MONO,VALUE(0)
        SU_LOADVAL MONO,VALUE(64)
        SU_AUX     STEREO,GAIN(128),CHANNEL(0)
        SU_LOADVAL MONO,VALUE(128)
        SU_LOADVAL MONO,VALUE(128)
        SU_AUX     STEREO,GAIN(64),CHANNEL(2)
        SU_IN      STEREO,CHANNEL(0)
        SU_IN      STEREO,CHANNEL(2)
        SU_ADDP    STEREO
        SU_OUT     STEREO,GAIN(128)
    END_INSTRUMENT
END_PATCH

%include "../src/sointu.asm"