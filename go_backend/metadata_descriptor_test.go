package gobackend

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCompleteMetadataHintReturnsFileAccessErrors(t *testing.T) {
	for _, format := range []string{"flac", "mp3", "m4a", "mp4", "aac", "opus", "ogg", "wav", "aiff", "aif", "aifc", "ape", "wv", "mpc"} {
		t.Run(format, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "descriptor")
			payload, err := ReadFileMetadataWithHint(path, "track."+format)
			if !errors.Is(err, os.ErrNotExist) || payload != "" {
				t.Fatalf("missing descriptor returned metadata=%s err=%v", payload, err)
			}
			if err := os.WriteFile(path, []byte("inaccessible"), 0000); err != nil {
				t.Fatal(err)
			}
			if file, err := os.Open(path); err == nil {
				file.Close()
				t.Skip("host can bypass file permissions; missing-path check passed")
			}
			payload, err = ReadFileMetadataWithHint(path, "track."+format)
			if !errors.Is(err, os.ErrPermission) || payload != "" {
				t.Fatalf("unreadable descriptor returned metadata=%s err=%v", payload, err)
			}
		})
	}
}

func TestCompleteMetadataHintAcceptsAudioWithoutTags(t *testing.T) {
	for _, format := range []string{"wav", "aiff"} {
		t.Run(format, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "descriptor")
			if format == "wav" {
				writeTestWAV(t, path)
			} else {
				writeTestAIFF(t, path)
			}
			payload, err := ReadFileMetadataWithHint(path, "track."+format)
			if err != nil {
				t.Fatal(err)
			}
			var metadata map[string]any
			if err := json.Unmarshal([]byte(payload), &metadata); err != nil {
				t.Fatal(err)
			}
			if metadata["title"] != "" || metadata["sample_rate"] != float64(44100) {
				t.Fatalf("unexpected tagless audio metadata: %s", payload)
			}
		})
	}
}

func TestCompleteMetadataHintMatchesNamedFileAndDescriptor(t *testing.T) {
	for _, format := range []string{"mp3", "flac", "m4a", "wav"} {
		t.Run(format, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "track."+format)
			switch format {
			case "mp3":
				data := buildID3v23Tag(id3TextFrame("TIT2", "Song"), id3TextFrame("TPE1", "Artist"), id3TextFrame("TSRC", "USRC17607839"), id3CommentFrame("USLT", "Words"), id3UserTextFrame("TXXX", "REPLAYGAIN_TRACK_GAIN", "-6.00 dB"), id3UserTextFrame("TXXX", "REPLAYGAIN_ALBUM_GAIN", "-4.00 dB"))
				if err := os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
			case "flac":
				writeSinglePassTestFlac(t, path, nil)
			case "wav":
				writeTestWAV(t, path)
			case "m4a":
				data, _ := buildTestM4A(t, buildM4ATextAtom("\xa9nam", "Song"), []byte("audio"))
				if err := os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			expected, err := ReadFileMetadata(path)
			if err != nil {
				t.Fatal(err)
			}
			extensionless := filepath.Join(dir, "descriptor")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(extensionless, data, 0600); err != nil {
				t.Fatal(err)
			}
			actual, err := ReadFileMetadataWithHint(extensionless, "track."+format)
			if err != nil || actual != expected {
				t.Fatalf("hinted metadata=%s expected=%s err=%v", actual, expected, err)
			}
			// Android uses /proc, which reopens with an independent offset.
			// macOS /dev/fd duplicates the shared offset and is not that API.
			if runtime.GOOS != "linux" {
				return
			}
			file, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			prefix := "/proc/self/fd/"
			actual, err = ReadFileMetadataWithHint(fmt.Sprintf("%s%d", prefix, file.Fd()), "track."+format)
			if err != nil || actual != expected {
				t.Fatalf("descriptor metadata=%s expected=%s err=%v", actual, expected, err)
			}
		})
	}
}
