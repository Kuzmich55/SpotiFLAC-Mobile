package gobackend

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func id3ReaderTestFrame(version byte, id string, payload []byte) []byte {
	if version == 2 {
		out := []byte{id[0], id[1], id[2], byte(len(payload) >> 16), byte(len(payload) >> 8), byte(len(payload))}
		return append(out, payload...)
	}
	frame := id3v23Frame(id, payload)
	if version == 4 {
		copy(frame[4:8], syncsafeBytes(len(payload)))
	}
	return frame
}

func id3ReaderTestTag(version, flags byte, body []byte) []byte {
	header := []byte{'I', 'D', '3', version, 0, flags, 0, 0, 0, 0}
	copy(header[6:10], syncsafeBytes(len(body)))
	return append(header, body...)
}

func TestID3CombinedReaderVersionsExtendedHeadersAndUnsync(t *testing.T) {
	cover := []byte{0xff, 0xd8, 0xff, 0xe0, 1, 2, 3}
	for _, version := range []byte{2, 3, 4} {
		for _, extended := range []bool{false, true} {
			if version == 2 && extended {
				continue
			}
			for _, unsync := range []bool{false, true} {
				t.Run(fmt.Sprintf("v%d/extended=%v/unsync=%v", version, extended, unsync), func(t *testing.T) {
					titleID, artistID, pictureID := "TIT2", "TPE1", "APIC"
					picture := append([]byte{0, 'i', 'm', 'a', 'g', 'e', '/', 'j', 'p', 'e', 'g', 0, 3, 0}, cover...)
					if version == 2 {
						titleID, artistID, pictureID = "TT2", "TP1", "PIC"
						picture = append([]byte{0, 'J', 'P', 'G', 3, 0}, cover...)
					}
					frames := append(id3ReaderTestFrame(version, titleID, []byte{0, 'T', 'i', 't', 'l', 'e'}), id3ReaderTestFrame(version, artistID, []byte{0, 'A', 'r', 't', 'i', 's', 't'})...)
					var flags byte
					if version == 4 && unsync {
						picture = bytes.ReplaceAll(picture, []byte{0xff}, []byte{0xff, 0})
						flags |= 0x80
					}
					frames = append(frames, id3ReaderTestFrame(version, pictureID, picture)...)
					if extended {
						flags |= 0x40
						prefix := []byte{0, 0, 0, 6, 0, 0}
						if version == 3 {
							prefix = append(prefix, 0, 0, 0, 0)
						}
						frames = append(prefix, frames...)
					}
					if version < 4 && unsync {
						frames = bytes.ReplaceAll(frames, []byte{0xff}, []byte{0xff, 0})
						flags |= 0x80
					}
					data := id3ReaderTestTag(version, flags, frames)
					metadata, got, mime, err := readID3v2WithCover(bytes.NewReader(data), true)
					if err != nil || metadata.Title != "Title" || metadata.Artist != "Artist" || !bytes.Equal(got, cover) || mime != "image/jpeg" {
						t.Fatalf("metadata=%+v cover=%x mime=%s err=%v", metadata, got, mime, err)
					}
					path := filepath.Join(t.TempDir(), "track.mp3")
					if err := os.WriteFile(path, data, 0600); err != nil {
						t.Fatal(err)
					}
					standalone, _, err := extractMP3CoverArt(path)
					if err != nil || !bytes.Equal(standalone, got) {
						t.Fatalf("standalone cover=%x err=%v", standalone, err)
					}
				})
			}
		}
	}
}

type countingID3Reader struct {
	*bytes.Reader
	bytesRead int
}

func (reader *countingID3Reader) Read(out []byte) (int, error) {
	count, err := reader.Reader.Read(out)
	reader.bytesRead += count
	return count, err
}

func TestID3MetadataSkipsLargeCoverPayload(t *testing.T) {
	picture := append([]byte{0, 'i', 'm', 'a', 'g', 'e', '/', 'j', 'p', 'e', 'g', 0, 3, 0}, bytes.Repeat([]byte{1}, 8<<20)...)
	tag := buildID3v23Tag(id3TextFrame("TIT2", "Song"), id3v23Frame("APIC", picture), id3TextFrame("TSRC", "USRC17607839"))
	reader := &countingID3Reader{Reader: bytes.NewReader(tag)}
	metadata, cover, _, err := readID3v2WithCover(reader, false)
	if err != nil || metadata.ISRC != "USRC17607839" || len(cover) != 0 {
		t.Fatalf("metadata=%+v cover=%d err=%v", metadata, len(cover), err)
	}
	if reader.bytesRead > 1024 {
		t.Fatalf("tag-only read consumed %d bytes, including artwork", reader.bytesRead)
	}
	reader = &countingID3Reader{Reader: bytes.NewReader(tag)}
	_, cover, _, err = readID3v2WithCover(reader, true)
	if err != nil || len(cover) != 8<<20 {
		t.Fatalf("cover=%d err=%v", len(cover), err)
	}
	if reader.bytesRead != len(tag) {
		t.Fatalf("combined read consumed %d bytes for %d-byte tag", reader.bytesRead, len(tag))
	}
}

func TestID3ReaderRejectsMalformedBoundsAndSkipsUnsupportedFrames(t *testing.T) {
	tag := buildID3v23Tag(id3TextFrame("TIT2", "Song"))
	truncated := append([]byte{}, tag[:len(tag)-1]...)
	if _, _, _, err := readID3v2WithCover(bytes.NewReader(truncated), false); err == nil {
		t.Fatal("accepted truncated declared tag")
	}
	oversized := append([]byte{}, tag...)
	binary.BigEndian.PutUint32(oversized[14:18], 1<<30)
	if _, _, _, err := readID3v2WithCover(bytes.NewReader(oversized), true); err == nil {
		t.Fatal("accepted frame outside tag")
	}
	compressed := id3TextFrame("TIT2", "unsupported")
	compressed[9] = 0x80
	good := id3TextFrame("TIT2", "Song")
	metadata, _, _, err := readID3v2WithCover(bytes.NewReader(buildID3v23Tag(compressed, good)), false)
	if err != nil || metadata.Title != "Song" {
		t.Fatalf("metadata=%+v err=%v", metadata, err)
	}
}

func TestScanMP3CombinedMetadataAndCover(t *testing.T) {
	dir := t.TempDir()
	cover := []byte{0xff, 0xd8, 0xff, 1, 2, 3}
	picture := append([]byte{0, 'i', 'm', 'a', 'g', 'e', '/', 'j', 'p', 'e', 'g', 0, 3, 0}, cover...)
	path, _ := writeTestMP3(t, dir, id3TextFrame("TIT2", "Song"), id3TextFrame("TPE1", "Artist"), id3CommentFrame("USLT", "words"), id3v23Frame("APIC", picture))
	cache := filepath.Join(dir, "covers")
	for _, pass := range []string{"cold", "warm"} {
		result, err := scanMP3FileWithCoverCache(path, &LibraryScanResult{FilePath: path}, "", cache, "key")
		if err != nil || result.TrackName != "Song" || !result.HasLyrics || result.CoverPath == "" {
			t.Fatalf("%s result=%+v err=%v", pass, result, err)
		}
		got, err := os.ReadFile(result.CoverPath)
		if err != nil || !bytes.Equal(got, cover) {
			t.Fatalf("cover=%x err=%v", got, err)
		}
	}
}

func BenchmarkID3MetadataLargeCover(b *testing.B) {
	picture := append([]byte{0, 'i', 'm', 'a', 'g', 'e', '/', 'j', 'p', 'e', 'g', 0, 3, 0}, bytes.Repeat([]byte{1}, 8<<20)...)
	tag := buildID3v23Tag(id3TextFrame("TIT2", "Song"), id3v23Frame("APIC", picture))
	reader := bytes.NewReader(tag)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		reader.Seek(0, io.SeekStart)
		if _, _, _, err := readID3v2WithCover(reader, false); err != nil {
			b.Fatal(err)
		}
	}
}

func TestID3PartialTagsAndCoverSurviveMalformedTrailingFrame(t *testing.T) {
	cover := []byte{0xff, 0xd8, 0xff, 1, 2, 3}
	picture := append([]byte{0, 'i', 'm', 'a', 'g', 'e', '/', 'j', 'p', 'e', 'g', 0, 3, 0}, cover...)
	bad := id3TextFrame("TALB", "bad")
	binary.BigEndian.PutUint32(bad[4:8], 1<<30)
	tag := buildID3v23Tag(id3TextFrame("TIT2", "Song"), id3v23Frame("APIC", picture), bad)
	metadata, _, _, err := readID3v2WithCover(bytes.NewReader(tag), true)
	if err == nil || metadata.Title != "Song" {
		t.Fatalf("partial metadata=%+v err=%v", metadata, err)
	}
	path := filepath.Join(t.TempDir(), "song.mp3")
	if err := os.WriteFile(path, tag, 0600); err != nil {
		t.Fatal(err)
	}
	metadata, err = ReadID3Tags(path)
	if err != nil || metadata.Title != "Song" {
		t.Fatalf("path metadata=%+v err=%v", metadata, err)
	}
	got, _, err := extractMP3CoverArt(path)
	if err != nil || !bytes.Equal(got, cover) {
		t.Fatalf("partial cover=%x err=%v", got, err)
	}
}

func TestID3GlobalUnsyncAllowsShortDecodedPadding(t *testing.T) {
	for _, version := range []byte{2, 3} {
		pictureID, titleID := "APIC", "TIT2"
		if version == 2 {
			pictureID, titleID = "PIC", "TT2"
		}
		body := id3ReaderTestFrame(version, titleID, []byte{0, 'S', 'o', 'n', 'g'})
		body = append(body, id3ReaderTestFrame(version, pictureID, bytes.Repeat([]byte{0xff, 0xe0}, 20))...)
		body = append(body, 0, 0, 0)
		body = bytes.ReplaceAll(body, []byte{0xff}, []byte{0xff, 0})
		metadata, _, _, err := readID3v2WithCover(bytes.NewReader(id3ReaderTestTag(version, 0x80, body)), false)
		if err != nil || metadata.Title != "Song" {
			t.Fatalf("v%d metadata=%+v err=%v", version, metadata, err)
		}
	}
}

func TestID3CombinedReaderSkipsAdditionalArtwork(t *testing.T) {
	picture := append([]byte{0, 'i', 'm', 'a', 'g', 'e', '/', 'j', 'p', 'e', 'g', 0, 3, 0}, bytes.Repeat([]byte{1}, 2<<20)...)
	tag := buildID3v23Tag(id3TextFrame("TIT2", "Song"), id3v23Frame("APIC", picture), id3v23Frame("APIC", picture), id3TextFrame("TSRC", "USRC17607839"))
	reader := &countingID3Reader{Reader: bytes.NewReader(tag)}
	metadata, cover, _, err := readID3v2WithCover(reader, true)
	if err != nil || metadata.ISRC != "USRC17607839" || len(cover) != 2<<20 {
		t.Fatalf("metadata=%+v cover=%d err=%v", metadata, len(cover), err)
	}
	if reader.bytesRead > len(picture)+1024 {
		t.Fatalf("read %d bytes, including unused second cover", reader.bytesRead)
	}
}
