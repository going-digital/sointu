package vm

import (
	"errors"
	"fmt"
	"math"

	"github.com/vsariola/sointu"
)

//go:generate go run generate/generate.go

// Interpreter is a pure-Go bytecode interpreter for the Sointu VM bytecode. It
// can only simulate bytecode compiled for AllFeatures, as the opcodes hard
// coded in it for speed. If you are interested exactly how opcodes / units
// work, studying Interpreter.Render is a good place to start.
//
// Internally, it uses software stack with practically no limitations in the
// number of signals, so be warned that if you compose patches for it, they
// might not work with the x87 implementation, as it has only 8-level stack.
type Interpreter struct {
	bytePatch  BytePatch
	stack      []float32
	synth      synth
	delaylines []delayline
}

type SynthService struct {
}

const MAX_VOICES = 32
const MAX_UNITS = 63

type unit struct {
	state [8]float32
	ports [8]float32
}

type voice struct {
	note    byte
	release bool
	units   [MAX_UNITS]unit
}

type synth struct {
	outputs    [8]float32
	randSeed   uint32
	globalTime uint32
	voices     [MAX_VOICES]voice
}

type delayline struct {
	buffer      [65536]float32
	dampState   float32
	dcIn        float32
	dcFiltState float32
}

const (
	envStateAttack = iota
	envStateDecay
	envStateSustain
	envStateRelease
)

func Synth(patch sointu.Patch) (sointu.Synth, error) {
	bytePatch, err := Encode(patch, AllFeatures{})
	if err != nil {
		return nil, fmt.Errorf("error compiling %v", err)
	}
	ret := &Interpreter{bytePatch: *bytePatch, stack: make([]float32, 0, 4), delaylines: make([]delayline, patch.NumDelayLines())}
	ret.synth.randSeed = 1
	return ret, nil
}

func (s SynthService) Compile(patch sointu.Patch) (sointu.Synth, error) {
	synth, err := Synth(patch)
	return synth, err
}

func (s *Interpreter) Trigger(voiceIndex int, note byte) {
	s.synth.voices[voiceIndex] = voice{}
	s.synth.voices[voiceIndex].note = note
}

func (s *Interpreter) Release(voiceIndex int) {
	s.synth.voices[voiceIndex].release = true
}

func (s *Interpreter) Update(patch sointu.Patch) error {
	bytePatch, err := Encode(patch, AllFeatures{})
	if err != nil {
		return fmt.Errorf("error compiling %v", err)
	}
	needsRefresh := len(bytePatch.Commands) != len(s.bytePatch.Commands)
	if !needsRefresh {
		for i, c := range bytePatch.Commands {
			if s.bytePatch.Commands[i] != c {
				needsRefresh = true
				break
			}
		}
	}
	s.bytePatch = *bytePatch
	for len(s.delaylines) < patch.NumDelayLines() {
		s.delaylines = append(s.delaylines, delayline{})
	}
	if needsRefresh {
		for i := range s.synth.voices {
			for j := range s.synth.voices[i].units {
				s.synth.voices[i].units[j] = unit{}
			}
		}
	}
	return nil
}

func (s *Interpreter) Render(buffer []float32, syncBuf []float32, maxtime int) (samples int, syncs int, time int, renderError error) {
	defer func() {
		if err := recover(); err != nil {
			renderError = fmt.Errorf("render panicced: %v", err)
		}
	}()
	var params [8]float32
	stack := s.stack[:]
	stack = append(stack, []float32{0, 0, 0, 0}...)
	synth := &s.synth
	for time < maxtime && len(buffer) > 1 {
		commandInstr := s.bytePatch.Commands
		valuesInstr := s.bytePatch.Values
		commands, values := commandInstr, valuesInstr
		delaylines := s.delaylines
		voicesRemaining := s.bytePatch.NumVoices
		voices := s.synth.voices[:]
		units := voices[0].units[:]
		if byte(s.synth.globalTime) == 0 { // every 256 samples
			syncBuf[0], syncBuf = float32(time), syncBuf[1:]
			syncs++
		}
		for voicesRemaining > 0 {
			op := commands[0]
			commands = commands[1:]
			channels := int((op & 1) + 1)
			stereo := channels == 2
			opNoStereo := (op & 0xFE) >> 1
			if opNoStereo == 0 {
				voices = voices[1:]
				units = voices[0].units[:]
				voicesRemaining--
				if mask := uint32(1) << uint32(voicesRemaining); s.bytePatch.PolyphonyBitmask&mask == mask {
					commands, values = commandInstr, valuesInstr
				} else {
					commandInstr, valuesInstr = commands, values
				}
				continue
			}
			tcount := transformCounts[opNoStereo-1]
			if len(values) < tcount {
				return samples, syncs, time, errors.New("value stream ended prematurely")
			}
			voice := &voices[0]
			unit := &units[0]
			valuesAtTransform := values
			for i := 0; i < tcount; i++ {
				params[i] = float32(values[0])/128.0 + unit.ports[i]
				unit.ports[i] = 0
				values = values[1:]
			}
			l := len(stack)
			switch opNoStereo {
			case opAdd:
				if stereo {
					stack[l-1] += stack[l-3]
					stack[l-2] += stack[l-4]
				} else {
					stack[l-1] += stack[l-2]
				}
			case opAddp:
				if stereo {
					stack[l-3] += stack[l-1]
					stack[l-4] += stack[l-2]
					stack = stack[:l-2]
				} else {
					stack[l-2] += stack[l-1]
					stack = stack[:l-1]
				}
			case opMul:
				if stereo {
					stack[l-1] *= stack[l-3]
					stack[l-2] *= stack[l-4]
				} else {
					stack[l-1] *= stack[l-2]
				}
			case opMulp:
				if stereo {
					stack[l-3] *= stack[l-1]
					stack[l-4] *= stack[l-2]
					stack = stack[:l-2]
				} else {
					stack[l-2] *= stack[l-1]
					stack = stack[:l-1]
				}
			case opXch:
				if stereo {
					stack[l-3], stack[l-1] = stack[l-1], stack[l-3]
					stack[l-4], stack[l-2] = stack[l-2], stack[l-4]
				} else {
					stack[l-2], stack[l-1] = stack[l-1], stack[l-2]
				}
			case opPush:
				if stereo {
					stack = append(stack, stack[l-2])
				}
				stack = append(stack, stack[l-1])
			case opPop:
				if stereo {
					stack = stack[:l-2]
				} else {
					stack = stack[:l-1]
				}
			case opDistort:
				amount := params[0]
				if stereo {
					stack[l-2] = waveshape(stack[l-2], amount)
				}
				stack[l-1] = waveshape(stack[l-1], amount)
			case opLoadval:
				val := params[0]*2 - 1
				if stereo {
					stack = append(stack, val)
				}
				stack = append(stack, val)
			case opOut:
				if stereo {
					synth.outputs[0] += params[0] * stack[l-1]
					synth.outputs[1] += params[0] * stack[l-2]
					stack = stack[:l-2]
				} else {
					synth.outputs[0] += params[0] * stack[l-1]
					stack = stack[:l-1]
				}
			case opOutaux:
				if stereo {
					synth.outputs[0] += params[0] * stack[l-1]
					synth.outputs[1] += params[0] * stack[l-2]
					synth.outputs[2] += params[1] * stack[l-1]
					synth.outputs[3] += params[1] * stack[l-2]
					stack = stack[:l-2]
				} else {
					synth.outputs[0] += params[0] * stack[l-1]
					synth.outputs[2] += params[1] * stack[l-1]
					stack = stack[:l-1]
				}
			case opAux:
				var channel byte
				channel, values = values[0], values[1:]
				if stereo {
					synth.outputs[channel+1] += params[0] * stack[l-2]
				}
				synth.outputs[channel] += params[0] * stack[l-1]
				stack = stack[:l-channels]
			case opSpeed:
				r := unit.state[0] + float32(math.Exp2(float64(stack[l-1]*2.206896551724138))-1)
				w := int(r+1.5) - 1
				unit.state[0] = r - float32(w)
				time += w
				stack = stack[:l-1]
			case opIn:
				var channel byte
				channel, values = values[0], values[1:]
				if stereo {
					stack = append(stack, synth.outputs[channel+1])
					synth.outputs[channel+1] = 0
				}
				stack = append(stack, synth.outputs[channel])
				synth.outputs[channel] = 0
			case opEnvelope:
				if voices[0].release {
					unit.state[0] = envStateRelease // set state to release
				}
				state := unit.state[0]
				level := unit.state[1]
				switch state {
				case envStateAttack:
					level += nonLinearMap(params[0])
					if level >= 1 {
						level = 1
						state = envStateDecay
					}
				case envStateDecay:
					level -= nonLinearMap(params[1])
					if sustain := params[2]; level <= sustain {
						level = sustain
					}
				case envStateRelease:
					level -= nonLinearMap(params[3])
					if level <= 0 {
						level = 0
					}
				}
				unit.state[0] = state
				unit.state[1] = level
				output := level * params[4]
				stack = append(stack, output)
				if stereo {
					stack = append(stack, output)
				}
			case opNoise:
				if stereo {
					value := waveshape(synth.rand(), params[0]) * params[1]
					stack = append(stack, value)
				}
				value := waveshape(synth.rand(), params[0]) * params[1]
				stack = append(stack, value)
			case opGain:
				if stereo {
					stack[l-2] *= params[0]
				}
				stack[l-1] *= params[0]
			case opInvgain:
				if stereo {
					stack[l-2] /= params[0]
				}
				stack[l-1] /= params[0]
			case opClip:
				if stereo {
					stack[l-2] = clip(stack[l-2])
				}
				stack[l-1] = clip(stack[l-1])
			case opCrush:
				if stereo {
					stack[l-2] = crush(stack[l-2], params[0])
				}
				stack[l-1] = crush(stack[l-1], params[0])
			case opHold:
				freq2 := params[0] * params[0]
				for i := 0; i < channels; i++ {
					phase := unit.state[i] - freq2
					if phase <= 0 {
						unit.state[2+i] = stack[l-1-i]
						phase += 1.0
					}
					stack[l-1-i] = unit.state[2+i]
					unit.state[i] = phase
				}
			case opSend:
				var addrLow, addrHigh byte
				addrLow, addrHigh, values = values[0], values[1], values[2:]
				addr := (uint16(addrHigh) << 8) + uint16(addrLow)
				targetVoice := voice
				if addr&0x8000 == 0x8000 {
					addr -= 0x8010
					targetVoice = &synth.voices[addr>>10]
				}
				unitIndex := ((addr & 0x01F0) >> 4) - 1
				port := addr & 7
				amount := params[0]*2 - 1
				for i := 0; i < channels; i++ {
					targetVoice.units[unitIndex].ports[int(port)+i] += stack[l-1-i] * amount
				}
				if addr&0x8 == 0x8 {
					stack = stack[:l-channels]
				}
			case opReceive:
				if stereo {
					stack = append(stack, unit.ports[1])
					unit.ports[1] = 0
				}
				stack = append(stack, unit.ports[0])
				unit.ports[0] = 0
			case opLoadnote:
				noteFloat := float32(voice.note)/64 - 1
				stack = append(stack, noteFloat)
				if stereo {
					stack = append(stack, noteFloat)
				}
			case opPan:
				if !stereo {
					stack = append(stack, stack[l-1])
					l++
				}
				stack[l-2] *= params[0]
				stack[l-1] *= 1 - params[0]
			case opFilter:
				freq2 := params[0] * params[0]
				res := params[1]
				var flags byte
				flags, values = values[0], values[1:]
				for i := 0; i < channels; i++ {
					low, band := unit.state[0+i], unit.state[2+i]
					low += freq2 * band
					high := stack[l-1-i] - low - res*band
					band += freq2 * high
					unit.state[0+i], unit.state[2+i] = low, band
					var output float32
					if flags&0x40 == 0x40 {
						output += low
					}
					if flags&0x20 == 0x20 {
						output += band
					}
					if flags&0x10 == 0x10 {
						output += high
					}
					if flags&0x08 == 0x08 {
						output -= band
					}
					if flags&0x04 == 0x04 {
						output -= high
					}
					stack[l-1-i] = output
				}
			case opOscillator:
				var flags byte
				flags, values = values[0], values[1:]
				detuneStereo := params[1]*2 - 1
				unison := flags & 3
				for i := 0; i < channels; i++ {
					detune := detuneStereo
					var output float32
					for j := byte(0); j <= unison; j++ {
						statevar := &unit.state[byte(i)+j*2]
						pitch := float64(64*(params[0]*2-1) + detune)
						if flags&0x8 == 0 { // if lfo is disable, add note to oscillator transpose
							pitch += float64(voice.note)
						}
						pitch *= 0.083333333333 // from semitones to octaves
						omega := math.Exp2(pitch)
						if flags&0x8 == 0 {
							omega *= 0.000092696138 // scaling coefficient to get middle-C where it should be
						} else {
							omega *= 0.000038 //  pretty random scaling constant to get LFOs into reasonable range. Historical reasons, goes all the way back to 4klang
						}
						*statevar += float32(omega)
						*statevar -= float32(int(*statevar+1) - 1)
						phase := *statevar
						phase += params[2]
						phase -= float32(int(phase))
						color := params[3]
						var amplitude float32
						switch {
						case flags&0x40 == 0x40: // Sine
							if phase < color {
								amplitude = float32(math.Sin(2 * math.Pi * float64(phase/color)))
							}
						case flags&0x20 == 0x20: // Trisaw
							if phase >= color {
								phase = 1 - phase
								color = 1 - color
							}
							amplitude = phase/color*2 - 1
						case flags&0x10 == 0x10: // Pulse
							if phase >= color {
								amplitude = -1
							} else {
								amplitude = 1
							}
						case flags&0x4 == 0x4: // Gate
							maskLow, maskHigh := valuesAtTransform[3], valuesAtTransform[4]
							gateBits := (int(maskHigh) << 8) + int(maskLow)
							amplitude = float32((gateBits >> (int(phase*16+.5) & 15)) & 1)
							g := unit.state[4+i] // warning: still fucks up with unison = 3
							amplitude += 0.99609375 * (g - amplitude)
							unit.state[4+i] = amplitude
						}
						if flags&0x4 == 0 {
							output += waveshape(amplitude, params[4]) * params[5]
						} else {
							output += amplitude * params[5]
						}
						if j < unison {
							params[2] += 0.08333333 // 1/12, add small phase shift so all oscillators don't start in phase
						}
						detune = -detune * 0.5
					}
					stack = append(stack, output)
					detuneStereo = -detuneStereo
				}
			case opDelay:
				pregain2 := params[0] * params[0]
				damp := params[3]
				feedback := params[2]
				var index, count byte
				index, count, values = values[0], values[1], values[2:]
				t := uint16(s.synth.globalTime)
				for i := 0; i < channels; i++ {
					var d *delayline
					signal := stack[l-1-i]
					output := params[1] * signal // dry output
					for j := byte(0); j < count; j += 2 {
						d, delaylines = &delaylines[0], delaylines[1:]
						delay := float32(s.bytePatch.DelayTimes[index]) + unit.ports[4]*32767
						if count&1 == 0 {
							delay /= float32(math.Exp2(float64(voice.note) * 0.083333333333))
						}
						delSignal := d.buffer[t-uint16(delay+0.5)]
						output += delSignal
						d.dampState = damp*d.dampState + (1-damp)*delSignal
						d.buffer[t] = feedback*d.dampState + pregain2*signal
						index++
					}
					d.dcFiltState = output + (0.99609375*d.dcFiltState - d.dcIn)
					d.dcIn = output
					stack[l-1-i] = d.dcFiltState
				}
				unit.ports[4] = 0
			case opCompressor:
				signalLevel := stack[l-1] * stack[l-1] // square the signal to get power
				if stereo {
					signalLevel += stack[l-2] * stack[l-2]
				}
				currentLevel := unit.state[0]
				paramIndex := 0 // compressor attacking
				if signalLevel < currentLevel {
					paramIndex = 1 // compressor releasing
				}
				alpha := nonLinearMap(params[paramIndex]) // map attack or release to a smoothing coefficient
				currentLevel += (signalLevel - currentLevel) * alpha
				unit.state[0] = currentLevel
				var gain float32 = 1
				if threshold2 := params[3] * params[3]; currentLevel > threshold2 {
					gain = float32(math.Pow(float64(threshold2/currentLevel), float64(params[4]/2)))
				}
				gain /= params[2] // apply inverse gain
				stack = append(stack, gain)
				if stereo {
					stack = append(stack, gain)
				}
			case opSync:
				if byte(s.synth.globalTime) == 0 { // every 256 samples
					syncBuf[0], syncBuf = float32(stack[l-1]), syncBuf[1:]
				}
			case opFriction:
				// Dummy operator for friction
				var in = stack[l-2]
				var force = stack[l-1]
				stack = stack[:l-1]
				stack[l-1] = in + force
			default:
				return samples, syncs, time, errors.New("invalid / unimplemented opcode")
			}
			units = units[1:]
		}
		if len(stack) < 4 {
			return samples, syncs, time, errors.New("stack underflow")
		}
		if len(stack) > 4 {
			return samples, syncs, time, errors.New("stack not empty")
		}
		buffer[0] = synth.outputs[0]
		buffer[1] = synth.outputs[1]
		synth.outputs[0] = 0
		synth.outputs[1] = 0
		buffer = buffer[2:]
		samples++
		time++
		s.synth.globalTime++
	}
	s.stack = stack[:0]
	return samples, syncs, time, nil
}

func (s *synth) rand() float32 {
	s.randSeed *= 16007
	return float32(int32(s.randSeed)) / -2147483648.0
}

func nonLinearMap(value float32) float32 {
	return float32(math.Exp2(float64(-24 * value)))
}

func clip(value float32) float32 {
	if value < -1 {
		return -1
	}
	if value > 1 {
		return 1
	}
	return value
}

func crush(value, amount float32) float32 {
	return float32(math.Round(float64(value/amount)) * float64(amount))
}

func waveshape(value, amount float32) float32 {
	absVal := value
	if absVal < 0 {
		absVal = -absVal
	}
	return value * amount / (1 - amount + (2*amount-1)*absVal)
}
