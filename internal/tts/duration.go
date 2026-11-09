package tts

// Duration of an MP3 (1.9.4), for a Web Story page that moves on when its
// narration ends (AMP's auto-advance-after). Counted frame by frame from the
// MPEG audio headers, so constant and variable bit rates are both exact;
// ID3 tags are skipped. No decoding, no dependency.

import "time"

// mp3 bit rates in kbit/s by [MPEG-1?][layer III index].
var (
	mp3BitratesV1 = [16]int{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 0}
	mp3BitratesV2 = [16]int{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160, 0}
	mp3Rates      = [4]int{44100, 48000, 32000, 0} // MPEG-1; halved for 2, quartered for 2.5
)

// Duration is how long an MP3 (Layer III) plays; 0 when b holds no frames.
func Duration(b []byte) time.Duration {
	b = stripID3v2(b)
	var seconds float64
	for i := 0; i+4 <= len(b); {
		n, samples, rate := mp3Frame(b[i : i+4])
		if n == 0 {
			i++ // not a frame header: resynchronise
			continue
		}
		seconds += float64(samples) / float64(rate)
		i += n
	}
	return time.Duration(seconds * float64(time.Second))
}

// mp3Frame reads one Layer III frame header: the frame's length in bytes,
// its samples and its sample rate; 0 when h is not a valid header.
func mp3Frame(h []byte) (length, samples, rate int) {
	if h[0] != 0xFF || h[1]&0xE0 != 0xE0 {
		return 0, 0, 0
	}
	version := (h[1] >> 3) & 3 // 0: 2.5, 2: 2, 3: 1
	layer := (h[1] >> 1) & 3   // 1: III
	bitIdx := h[2] >> 4
	rateIdx := (h[2] >> 2) & 3
	padding := int((h[2] >> 1) & 1)
	if version == 1 || layer != 1 || rateIdx == 3 || bitIdx == 0 || bitIdx == 15 {
		return 0, 0, 0
	}
	rate = mp3Rates[rateIdx]
	kbps := mp3BitratesV1[bitIdx]
	samples, coef := 1152, 144
	switch version {
	case 2:
		rate /= 2
		kbps, samples, coef = mp3BitratesV2[bitIdx], 576, 72
	case 0:
		rate /= 4
		kbps, samples, coef = mp3BitratesV2[bitIdx], 576, 72
	}
	return coef*kbps*1000/rate + padding, samples, rate
}
