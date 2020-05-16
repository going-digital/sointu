%define	BPM	100
%define USE_SECTIONS

%include "../src/sointu.inc"

SU_BEGIN_PATTERNS
	PATTERN 64, 0, 68, 0, 32, 0, 0, 0,	75, 0, 78, 0,	0, 0, 0, 0,		
SU_END_PATTERNS

SU_BEGIN_TRACKS
	TRACK VOICES(1),0
SU_END_TRACKS

SU_BEGIN_PATCH
	SU_BEGIN_INSTRUMENT VOICES(1) ; Instrument0
		SU_ENVELOPE	MONO,ATTAC(80),DECAY(80),SUSTAIN(64),RELEASE(80),GAIN(128)			
		SU_OSCILLAT	MONO,TRANSPOSE(64),DETUNE(64),PHASE(0),COLOR(128),SHAPE(64),GAIN(128),FLAGS(SINE)		
		SU_MULP		MONO
		SU_DELAY    MONO,PREGAIN(40),DRY(128),FEEDBACK(125),DAMP(64),DELAY(1),COUNT(1)
		SU_PAN		MONO,PANNING(64)
		SU_OUT		STEREO,GAIN(128)
		SU_OSCILLAT	MONO,TRANSPOSE(70),DETUNE(64),PHASE(64),COLOR(128),SHAPE(64),GAIN(128),FLAGS(SINE+LFO)		
		SU_SEND		MONO,AMOUNT(32),PORT(3,delay,damp) + SEND_POP
	SU_END_INSTRUMENT
SU_END_PATCH

SU_BEGIN_DELTIMES
	DELTIME	11025
SU_END_DELTIMES

%include "../src/sointu.asm"