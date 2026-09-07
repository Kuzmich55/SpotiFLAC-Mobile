package gobackend

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestResolvedAlbumFolder(t *testing.T) {
	for _, tc := range []struct {
		name, template, album, want string
	}{
		{"missing metadata", "{album}", "  ", ""},
		{"no pending folder", "", "Album", ""},
		{"invalid template", "Unknown", "Album", ""},
		{"album", "{album}", "Album", "Album"},
		{"year prefix", "[2024] {album}", "Album", "[2024] Album"},
		{"unsafe characters", "{album}", " ../Album: \"Deluxe\"/Part\\Two\x00 ", "Album Deluxe Part Two"},
		{"literal unknown album", "{album}", "Unknown", "Unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := resolvedAlbumFolder(DownloadRequest{AlbumFolderTemplate: tc.template}, tc.album)
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
	got := resolvedAlbumFolder(DownloadRequest{AlbumFolderTemplate: "[2024] {album}"}, strings.Repeat("音楽", 40))
	if len(got) > 120 || !utf8.ValidString(got) {
		t.Fatalf("invalid bounded UTF-8 folder: %q", got)
	}
}

func TestFinalizeDownloadAlbumFolder(t *testing.T) {
	for _, ext := range []string{".mp4", ".m4a", ".opus", ".flac"} {
		t.Run(ext, func(t *testing.T) {
			root := t.TempDir()
			originalDir := filepath.Join(root, "Playlist", "Artist", "Unknown")
			if err := os.MkdirAll(originalDir, 0755); err != nil {
				t.Fatal(err)
			}
			original := filepath.Join(originalDir, "Track"+ext)
			data := []byte("unchanged audio and metadata")
			if err := os.WriteFile(original, data, 0644); err != nil {
				t.Fatal(err)
			}
			req := DownloadRequest{OutputDir: originalDir, AlbumFolderTemplate: "{album}"}
			result := DownloadResponse{Success: true, FilePath: original, Album: "Resolved Album"}
			if err := finalizeDownloadAlbumFolder(req, &result); err != nil {
				t.Fatal(err)
			}
			want := filepath.Join(root, "Playlist", "Artist", "Resolved Album", "Track"+ext)
			if result.FilePath != want || result.ResolvedAlbumFolder != "Resolved Album" {
				t.Fatalf("unexpected resolved output: %+v", result)
			}
			got, err := os.ReadFile(want)
			if err != nil || string(got) != string(data) {
				t.Fatalf("output changed: %q, %v", got, err)
			}
			if _, err := os.Stat(original); !os.IsNotExist(err) {
				t.Fatalf("old output remains: %v", err)
			}
		})
	}
}

func TestFinalizeAlbumFolderPreservesHostOutputAndSourceAlbum(t *testing.T) {
	req := DownloadRequest{
		OutputDir: filepath.Join(t.TempDir(), "Unknown"), OutputPath: "/host/cache/track.mp4",
		AlbumFolderTemplate: "{album}", AlbumName: "Source Album",
	}
	result := DownloadResponse{FilePath: req.OutputPath, Album: "Provider Compilation"}
	if err := finalizeDownloadAlbumFolder(req, &result); err != nil {
		t.Fatal(err)
	}
	if result.FilePath != req.OutputPath || result.ResolvedAlbumFolder != "Source Album" {
		t.Fatalf("host output or source album replaced: %+v", result)
	}
	req.OutputPath = ""
	if got := filepath.Dir(buildOutputPath(req)); filepath.Base(got) != "Source Album" {
		t.Fatalf("enriched metadata did not resolve the path before transfer: %q", got)
	}
}

func TestFinalizeAlbumFolderDoesNotOverwriteExistingFile(t *testing.T) {
	root := t.TempDir()
	original := filepath.Join(root, "track.mp4")
	dir := filepath.Join(root, "Album")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(dir, "track.mp4")
	for path, value := range map[string]string{original: "new audio", destination: "existing audio"} {
		if err := os.WriteFile(path, []byte(value), 0644); err != nil {
			t.Fatal(err)
		}
	}
	req := DownloadRequest{OutputDir: filepath.Join(root, "Unknown"), AlbumFolderTemplate: "{album}"}
	result := DownloadResponse{FilePath: original, Album: "Album"}
	if err := finalizeDownloadAlbumFolder(req, &result); err == nil {
		t.Fatal("expected destination collision")
	}
	for path, want := range map[string]string{original: "new audio", destination: "existing audio"} {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != want {
			t.Fatalf("file damaged: %q, %v", got, err)
		}
	}
}

func TestFinalizeAlbumFolderReadsContainerTagsWithoutAudioDecoding(t *testing.T) {
	albumData := buildM4AAtom("data", append([]byte{0, 0, 0, 1, 0, 0, 0, 0}, []byte("Embedded Album")...))
	ilst := buildM4AAtom("ilst", buildM4AAtom("\xa9alb", albumData))
	meta := buildM4AAtom("meta", append(make([]byte, 4), ilst...))
	data := buildM4AAtom("moov", buildM4AAtom("udta", meta))
	// The album atom is independent of the encrypted/undecoded audio bytes.
	data = append(data, buildM4AAtom("mdat", []byte("undecoded audio payload"))...)
	path := filepath.Join(t.TempDir(), "encrypted.mp4")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	req := DownloadRequest{OutputPath: path, AlbumFolderTemplate: "{album}"}
	result := DownloadResponse{FilePath: path}
	if err := finalizeDownloadAlbumFolder(req, &result); err != nil {
		t.Fatal(err)
	}
	if result.ResolvedAlbumFolder != "Embedded Album" || result.Album != "Embedded Album" {
		t.Fatalf("missing container album: %+v", result)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != string(data) {
		t.Fatalf("host-owned audio changed: %v", err)
	}
}
