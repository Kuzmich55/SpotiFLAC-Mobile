package gobackend

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func readReplayGainOggPages(t *testing.T, path string) []oggEditPage {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	pages, err := readAllOggEditPages(f)
	if err != nil {
		t.Fatal(err)
	}
	return pages
}

func TestOpusReplayGainReplacesLegacyTagsAndPreservesAudio(t *testing.T) {
	for _, ext := range []string{".opus", ".ogg"} {
		t.Run(ext, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "song"+ext)
			buildTestOpus(t, path, []string{
				"TITLE=Example Song", "CUSTOM=Keep me", "METADATA_BLOCK_PICTURE=existing-picture",
				"REPLAYGAIN_TRACK_GAIN=-4.00 dB", "REPLAYGAIN_TRACK_PEAK=0.800000",
				"R128_TRACK_GAIN=-2304", "r128_track_gain=100",
				"REPLAYGAIN_ALBUM_GAIN=-3.00 dB", "REPLAYGAIN_ALBUM_PEAK=0.900000",
				"R128_ALBUM_GAIN=-2048",
			}, 3)
			// Preserve a nonzero OpusHead output gain as well as every audio page.
			pages := readReplayGainOggPages(t, path)
			binary.LittleEndian.PutUint16(pages[0].data[16:18], 256)
			var original bytes.Buffer
			for _, page := range pages {
				if err := page.serialize(&original); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(path, original.Bytes(), 0o600); err != nil {
				t.Fatal(err)
			}

			for _, scope := range []string{"track", "album"} {
				fields, _ := json.Marshal(map[string]string{
					"replaygain_" + scope + "_gain": "-12.20 dB",
					"replaygain_" + scope + "_peak": "1.258925",
				})
				result, err := EditFileMetadata(path, string(fields))
				if err != nil || !strings.Contains(result, "native_ogg") {
					t.Fatalf("write: %s, %v", result, err)
				}
				metadata, err := ReadFileMetadata(path)
				if err != nil {
					t.Fatal(err)
				}
				var readback map[string]any
				if err := json.Unmarshal([]byte(metadata), &readback); err != nil {
					t.Fatal(err)
				}
				if got := readback["replaygain_"+scope+"_gain"]; got != "-12.20 dB" {
					t.Fatalf("%s gain = %v", scope, got)
				}
				raw := mustReadFile(t, path)
				upper := strings.ToUpper(scope)
				if bytes.Count(bytes.ToUpper(raw), []byte("R128_"+upper+"_GAIN=")) != 1 ||
					!bytes.Contains(raw, []byte("R128_"+upper+"_GAIN=-4403")) ||
					bytes.Contains(bytes.ToUpper(raw), []byte("REPLAYGAIN_"+upper+"_")) {
					t.Fatalf("conflicting or incorrect %s comments", scope)
				}
				if scope == "track" && !bytes.Contains(raw, []byte("R128_ALBUM_GAIN=-2048")) {
					t.Fatal("track update changed album gain")
				}
			}
			after := readReplayGainOggPages(t, path)
			if len(after) != len(pages) {
				t.Fatalf("page count changed: %d -> %d", len(pages), len(after))
			}
			for i := range pages {
				if i == 1 { // Only OpusTags may change.
					continue
				}
				var beforePage, afterPage bytes.Buffer
				_ = pages[i].serialize(&beforePage)
				_ = after[i].serialize(&afterPage)
				if !bytes.Equal(beforePage.Bytes(), afterPage.Bytes()) {
					t.Fatalf("header/audio page %d changed", i)
				}
			}
			raw := mustReadFile(t, path)
			for _, comment := range []string{"CUSTOM=Keep me", "METADATA_BLOCK_PICTURE=existing-picture"} {
				if !bytes.Contains(raw, []byte(comment)) {
					t.Fatalf("lost %s", comment)
				}
			}
			if err := EditOggFields(path, map[string]string{"replaygain_track_gain": ""}); err != nil {
				t.Fatal(err)
			}
			raw = mustReadFile(t, path)
			if bytes.Contains(raw, []byte("R128_TRACK_GAIN=")) || !bytes.Contains(raw, []byte("R128_ALBUM_GAIN=-4403")) {
				t.Fatal("clearing track gain affected the wrong scope")
			}
		})
	}
}

func TestOpusReplayGainInvalidValuesLeaveFileUntouched(t *testing.T) {
	for _, gain := range []string{"invalid", "NaN", "+Inf", "-Inf", "-124 dB", "133 dB"} {
		t.Run(gain, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "song.opus")
			buildTestOpus(t, path, []string{"R128_TRACK_GAIN=-1280"}, 1)
			before := mustReadFile(t, path)
			if err := EditOggFields(path, map[string]string{"replaygain_track_gain": gain}); err == nil {
				t.Fatal("invalid gain was accepted")
			}
			if !bytes.Equal(before, mustReadFile(t, path)) {
				t.Fatal("failed write changed the original")
			}
		})
	}
	for _, raw := range []string{"32768", "-32769", "1.5"} {
		if _, ok := r128ToReplayGainDb(raw); ok {
			t.Fatalf("invalid R128 value accepted: %s", raw)
		}
	}
}

func TestOpusReplayGainWithoutTitleOrArtist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "song.opus")
	buildTestOpus(t, path, nil, 1)
	if err := EditOggFields(path, map[string]string{"replaygain_track_gain": "0.00 dB"}); err != nil {
		t.Fatal(err)
	}
	metadata, err := ReadFileMetadata(path)
	if err != nil || !strings.Contains(metadata, `"replaygain_track_gain":"0.00 dB"`) {
		t.Fatalf("ReplayGain-only comments were not read: %s, %v", metadata, err)
	}
}

func TestOpusReplayGainMediaRoundTrip(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("requires ffmpeg on PATH")
	}
	run := func(t *testing.T, args ...string) []byte {
		t.Helper()
		cmd := exec.Command(ffmpeg, append([]string{"-hide_banner", "-v", "error"}, args...)...)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("ffmpeg: %v: %s", err, &stderr)
		}
		return out
	}
	for _, cover := range []bool{false, true} {
		name := "without-cover"
		if cover {
			name = "with-cover"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "song.opus")
			run(t, "-f", "lavfi", "-i", "sine=frequency=440:duration=0.2", "-c:a", "libopus",
				"-metadata", "title=Example Song", "-metadata", "REPLAYGAIN_TRACK_GAIN=-4.00 dB",
				"-metadata", "R128_TRACK_GAIN=-2304", path)
			var picture []byte
			if cover {
				picture, err = base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aB1sAAAAASUVORK5CYII=")
				if err != nil {
					t.Fatal(err)
				}
				coverPath := filepath.Join(dir, "cover.png")
				if err := os.WriteFile(coverPath, picture, 0o600); err != nil {
					t.Fatal(err)
				}
				if err := EditOggFields(path, map[string]string{"cover_path": coverPath}); err != nil {
					t.Fatal(err)
				}
			}
			audioHash := func() []byte {
				return run(t, "-i", path, "-map", "0:a:0", "-c:a", "copy", "-f", "hash", "-hash", "sha256", "-")
			}
			beforeHash := audioHash()
			if err := EditOggFields(path, map[string]string{
				"replaygain_track_gain": "-12.20 dB", "replaygain_track_peak": "1.258925",
				"replaygain_album_gain": "-10.00 dB", "replaygain_album_peak": "1.300000",
			}); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(beforeHash, audioHash()) {
				t.Fatal("encoded audio changed")
			}
			run(t, "-i", path, "-map", "0:a:0", "-f", "null", "-")
			metadata, err := ReadOggVorbisComments(path)
			if err != nil || metadata.ReplayGainTrackGain != "-12.20 dB" || metadata.ReplayGainAlbumGain != "-10.00 dB" {
				t.Fatalf("gain reread: %+v, %v", metadata, err)
			}
			if cover {
				got, _, err := extractOggCoverArt(path)
				if err != nil || !bytes.Equal(got, picture) {
					t.Fatalf("cover changed: %v", err)
				}
			}
		})
	}
}
