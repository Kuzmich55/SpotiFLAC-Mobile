package gobackend

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

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
