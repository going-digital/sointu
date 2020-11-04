%define BPM 100

%include "sointu/header.inc"

BEGIN_PATTERNS
    PATTERN 64, HLD, HLD, HLD, HLD, HLD, HLD, HLD,  0, 0, 0, 0, 0, 0, 0, 0
END_PATTERNS

BEGIN_TRACKS
    TRACK VOICES(1),0
END_TRACKS

BEGIN_PATCH
    BEGIN_INSTRUMENT VOICES(1) ; Instrument0
        SU_ENVELOPE MONO,ATTAC(64),DECAY(64),SUSTAIN(64),RELEASE(80),GAIN(128)
        SU_HOLD     MONO,HOLDFREQ(3)
        SU_ENVELOPE MONO,ATTAC(64),DECAY(64),SUSTAIN(64),RELEASE(80),GAIN(128)
        SU_HOLD     MONO,HOLDFREQ(3)
        SU_OSCILLAT MONO,TRANSPOSE(70),DETUNE(64),PHASE(64),COLOR(128),SHAPE(64),GAIN(128),FLAGS(SINE+LFO)
        SU_SEND     MONO,AMOUNT(68),UNIT(1),PORT(0),FLAGS(NONE)
        SU_SEND     MONO,AMOUNT(68),UNIT(3),PORT(0),FLAGS(SEND_POP)
        SU_OUT      STEREO,GAIN(128)
    END_INSTRUMENT
END_PATCH

%include "sointu/footer.inc"
