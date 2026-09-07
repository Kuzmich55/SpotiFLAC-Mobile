package gobackend

import (
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestM4AAudioSampleEntryQuality(t *testing.T) {
	for _, codec := range []string{"mp4a", "alac", "fLaC", "ec-3", "ac-3", "ac-4", "Opus"} {
		t.Run(codec, func(t *testing.T) {
			mvhd := make([]byte, 100)
			binary.BigEndian.PutUint32(mvhd[12:16], 1000)
			binary.BigEndian.PutUint32(mvhd[16:20], 10000)
			sample := make([]byte, 28)
			binary.BigEndian.PutUint16(sample[6:8], 1)
			binary.BigEndian.PutUint16(sample[16:18], 2)
			binary.BigEndian.PutUint16(sample[18:20], 16)
			binary.BigEndian.PutUint32(sample[24:28], 48000<<16)
			stsd := append([]byte{0, 0, 0, 0, 0, 0, 0, 1}, buildM4AAtom(codec, sample)...)
			trak := buildM4AAtom("trak", buildM4AAtom("mdia", buildM4AAtom("minf", buildM4AAtom("stbl", buildM4AAtom("stsd", stsd)))))
			moov := buildM4AAtom("moov", append(buildM4AAtom("mvhd", mvhd), trak...))
			data := append(moov, buildM4AAtom("mdat", make([]byte, 400000))...)
			path := filepath.Join(t.TempDir(), "descriptor")
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			quality, err := GetM4AQuality(path)
			if err != nil {
				t.Fatal(err)
			}
			if quality.Codec != normalizeM4AAudioCodec(codec) || quality.SampleRate != 48000 || quality.Duration != 10 || quality.Bitrate < 320 {
				t.Fatalf("missing audio quality: %+v", quality)
			}
			if codec == "Opus" && quality.BitDepth != 0 {
				t.Fatalf("lossy Opus must not expose the sample-entry bit depth: %+v", quality)
			}
			// The full reader and lightweight Library scanner must both expose
			// bitrate, including the extensionless paths used by SAF.
			for _, read := range []func(string, string) (string, error){ReadFileMetadataWithHint, ReadAudioMetadataWithDisplayName} {
				payload, err := read(path, "Song.m4a")
				var metadata map[string]any
				if err != nil || json.Unmarshal([]byte(payload), &metadata) != nil {
					t.Fatalf("metadata=%s, error=%v", payload, err)
				}
				if bitrate, ok := metadata["bitrate"].(float64); !ok || bitrate < 320 {
					t.Fatalf("missing bitrate: %s", payload)
				}
			}
		})
	}
}

func TestParseALACSpecificConfigStandardPayload(t *testing.T) {
	payload := make([]byte, 24)
	payload[5] = 24
	payload[20] = 0x00
	payload[21] = 0x00
	payload[22] = 0xac
	payload[23] = 0x44

	bitDepth, sampleRate, ok := parseALACSpecificConfig(payload)
	if !ok {
		t.Fatal("expected standard ALAC payload to parse")
	}
	if bitDepth != 24 {
		t.Fatalf("bitDepth = %d, want 24", bitDepth)
	}
	if sampleRate != 44100 {
		t.Fatalf("sampleRate = %d, want 44100", sampleRate)
	}
}

func TestParseALACSpecificConfigPayloadWithLeadingFourBytes(t *testing.T) {
	payload := make([]byte, 28)
	payload[9] = 16
	payload[24] = 0x00
	payload[25] = 0x00
	payload[26] = 0xbb
	payload[27] = 0x80

	bitDepth, sampleRate, ok := parseALACSpecificConfig(payload)
	if !ok {
		t.Fatal("expected offset ALAC payload to parse")
	}
	if bitDepth != 16 {
		t.Fatalf("bitDepth = %d, want 16", bitDepth)
	}
	if sampleRate != 48000 {
		t.Fatalf("sampleRate = %d, want 48000", sampleRate)
	}
}

func TestParseALACSpecificConfigRejectsShortPayload(t *testing.T) {
	if _, _, ok := parseALACSpecificConfig(make([]byte, 12)); ok {
		t.Fatal("expected short ALAC payload to be rejected")
	}
}

func TestM4ACodecFormatMapping(t *testing.T) {
	cases := map[string]string{
		"mp4a": "aac",
		"alac": "alac",
		"fLaC": "flac",
		"ec-3": "eac3",
		"ac-3": "ac3",
		"ac-4": "ac4",
		"Opus": "opus",
	}
	for atomType, want := range cases {
		if got := normalizeM4AAudioCodec(atomType); got != want {
			t.Fatalf("normalizeM4AAudioCodec(%q) = %q, want %q", atomType, got, want)
		}
	}

	if got := libraryFormatForM4ACodec("flac"); got != "flac" {
		t.Fatalf("libraryFormatForM4ACodec(flac) = %q", got)
	}
	if got := libraryFormatForM4ACodec("eac3"); got != "eac3" {
		t.Fatalf("libraryFormatForM4ACodec(eac3) = %q", got)
	}
	if got := libraryFormatForM4ACodec("Opus"); got != "opus" {
		t.Fatalf("libraryFormatForM4ACodec(Opus) = %q", got)
	}
	if got := libraryFormatForM4ACodec("aac"); got != "m4a" {
		t.Fatalf("libraryFormatForM4ACodec(aac) = %q", got)
	}
}

func TestParseMP4FLACSpecificConfig(t *testing.T) {
	streamInfo := make([]byte, 34)
	sampleRate := 48000
	bitsPerSample := 24
	totalSamples := int64(48000 * 180)
	streamInfo[10] = byte(sampleRate >> 12)
	streamInfo[11] = byte(sampleRate >> 4)
	streamInfo[12] = byte((sampleRate&0x0F)<<4 | ((bitsPerSample-1)>>4)&0x01)
	streamInfo[13] = byte(((bitsPerSample-1)&0x0F)<<4 | int((totalSamples>>32)&0x0F))
	streamInfo[14] = byte(totalSamples >> 24)
	streamInfo[15] = byte(totalSamples >> 16)
	streamInfo[16] = byte(totalSamples >> 8)
	streamInfo[17] = byte(totalSamples)

	payload := append([]byte{0, 0, 0, 0, 0, 0, 0, 34}, streamInfo...)
	bitDepth, parsedRate, parsedSamples, ok := parseMP4FLACSpecificConfig(payload)
	if !ok {
		t.Fatal("expected MP4 FLAC config to parse")
	}
	if bitDepth != bitsPerSample || parsedRate != sampleRate || parsedSamples != totalSamples {
		t.Fatalf("FLAC config = %d/%d/%d", bitDepth, parsedRate, parsedSamples)
	}
}
