package gobackend

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// Only an album component that was missing when the queue was created may be
// resolved later. The caller supplies its leaf template, preserving artist,
// playlist and year folder choices without replacing literal "Unknown" paths.
func resolvedAlbumFolder(req DownloadRequest, album string) string {
	if !strings.Contains(req.AlbumFolderTemplate, "{album}") || strings.TrimSpace(album) == "" {
		return ""
	}
	name := strings.ReplaceAll(req.AlbumFolderTemplate, "{album}", album)
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		if strings.ContainsRune(`<>:"/\|?*`, r) {
			return ' '
		}
		return r
	}, name)
	name = strings.Join(strings.Fields(strings.Trim(name, ". ")), " ")
	for strings.Contains(name, "__") {
		name = strings.ReplaceAll(name, "__", "_")
	}
	name = strings.Trim(name, "_ ")
	// Match the app's SAF segment limit without splitting a UTF-8 character.
	if len(name) > 120 {
		name = name[:120]
		for !utf8.ValidString(name) {
			name = name[:len(name)-1]
		}
	}
	return strings.Trim(name, "._ ")
}

func resolvedAlbumOutputDirectory(req DownloadRequest, album string) string {
	folder := resolvedAlbumFolder(req, album)
	if folder == "" || strings.TrimSpace(req.OutputDir) == "" {
		return req.OutputDir
	}
	return filepath.Join(filepath.Dir(filepath.Clean(req.OutputDir)), folder)
}

func finalizeDownloadAlbumFolder(req DownloadRequest, result *DownloadResponse) error {
	album := firstNonEmptyTrimmed(req.AlbumName, result.Album)
	if album == "" && strings.Contains(req.AlbumFolderTemplate, "{album}") && result.FilePath != "" {
		// Container tags can remain readable even when the audio payload needs
		// host-side decryption. Do not require optional provider enrichment.
		if payload, err := ReadFileMetadata(result.FilePath); err == nil {
			var metadata struct {
				Album string `json:"album"`
			}
			if json.Unmarshal([]byte(payload), &metadata) == nil {
				album = strings.TrimSpace(metadata.Album)
				result.Album = album
			}
		}
	}
	result.ResolvedAlbumFolder = resolvedAlbumFolder(req, album)
	// The Android SAF host publishes its temporary file using the resolved
	// leaf. A supplied output path/FD remains owned by that host.
	if result.ResolvedAlbumFolder == "" || req.OutputPath != "" || isFDOutput(req.OutputFD) || result.AlreadyExists {
		return nil
	}
	dir := resolvedAlbumOutputDirectory(req, album)
	if dir == "" || filepath.Clean(dir) == filepath.Dir(result.FilePath) {
		return nil
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	destination := filepath.Join(dir, filepath.Base(result.FilePath))
	source, err := os.Open(result.FilePath)
	if err != nil {
		return err
	}
	defer source.Close()
	// Exclusive creation preserves any existing download at the final path,
	// including when another queue item resolves the same album concurrently.
	output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return fmt.Errorf("resolve album folder: %w", err)
	}
	_, copyErr := io.Copy(output, source)
	closeErr := output.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(destination)
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	}
	if err := source.Close(); err != nil {
		_ = os.Remove(destination)
		return err
	}
	if err := os.Remove(result.FilePath); err != nil {
		_ = os.Remove(destination)
		return err
	}
	AddAllowedDownloadDir(dir)
	GoLog("[Download] Resolved album folder: %s\n", dir)
	result.FilePath = destination
	return nil
}
