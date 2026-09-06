package gobackend

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
)

// A malformed frame must not allocate an entire declared tag (up to 256 MiB).
// Artwork is optional; tag-only callers seek past it without allocating it.
const maxID3FrameBytes = 32 << 20

func readID3v2WithCover(file io.ReadSeeker, includeCover bool) (*AudioMetadata, []byte, string, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, nil, "", err
	}
	var header [10]byte
	if _, err := io.ReadFull(file, header[:]); err != nil {
		return nil, nil, "", err
	}
	if string(header[:3]) != "ID3" {
		return nil, nil, "", fmt.Errorf("no ID3v2 header")
	}
	version, flags := header[3], header[5]
	if version < 2 || version > 4 || header[6]|header[7]|header[8]|header[9] >= 128 {
		return nil, nil, "", fmt.Errorf("invalid ID3 version or tag size")
	}
	size := int64(syncsafeToInt(header[6:10]))
	end, err := file.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, nil, "", err
	}
	if size > end-10 {
		return nil, nil, "", io.ErrUnexpectedEOF
	}
	if _, err := file.Seek(10, io.SeekStart); err != nil {
		return nil, nil, "", err
	}
	var reader io.Reader = file
	// ID3v2.2/2.3 unsynchronization covers the entire tag, including headers;
	// decode it as a bounded stream so large pictures still need no allocation.
	if flags&0x80 != 0 && version < 4 {
		reader = &id3UnsyncReader{source: bufio.NewReader(io.LimitReader(file, size))}
	}
	if flags&0x40 != 0 {
		if version == 2 {
			return nil, nil, "", fmt.Errorf("compressed ID3v2.2 tag unsupported")
		}
		var extended [4]byte
		if _, err := io.ReadFull(reader, extended[:]); err != nil {
			return nil, nil, "", err
		}
		length := int64(binary.BigEndian.Uint32(extended[:]))
		if version == 4 {
			length = int64(syncsafeToInt(extended[:])) - 4
		}
		if length < 0 || length > size-4 {
			return nil, nil, "", fmt.Errorf("invalid ID3 extended header")
		}
		if err := skipID3Bytes(reader, length); err != nil {
			return nil, nil, "", err
		}
		size -= length + 4
	}
	metadata := &AudioMetadata{}
	var cover []byte
	var mime string
	err = walkID3Frames(reader, size, version, version == 4 && flags&0x80 != 0, func() bool { return includeCover && len(cover) == 0 },
		func(id string, data []byte) {
			if id == "APIC" || id == "PIC" {
				if len(cover) == 0 {
					cover, mime = parseAPICFrame(data, version)
				}
			} else {
				applyID3Frame(metadata, version, id, data)
			}
		})
	return metadata, cover, mime, err
}

// walkID3Frames is shared by metadata, cover extraction and combined scanning.
// Only selected frame payloads are read; the enclosing tag bounds every seek.
func walkID3Frames(reader io.Reader, remaining int64, version byte, tagUnsync bool, wantCover func() bool, visit func(string, []byte)) error {
	headerLength, idLength := 10, 4
	if version == 2 {
		headerLength, idLength = 6, 3
	}
	var header [10]byte
	for remaining >= int64(headerLength) {
		if count, err := io.ReadFull(reader, header[:headerLength]); err != nil {
			// A globally unsynchronized tag can have fewer decoded bytes than its
			// raw size; exhaustion between frames is normal.
			if err == io.EOF {
				return nil
			}
			if err == io.ErrUnexpectedEOF {
				padding := true
				for _, value := range header[:count] {
					if value != 0 {
						padding = false
						break
					}
				}
				if padding {
					return nil
				}
			}
			return err
		}
		remaining -= int64(headerLength)
		if header[0] == 0 || string(header[:3]) == "3DI" {
			return nil
		}
		id := string(header[:idLength])
		var size int64
		switch version {
		case 2:
			size = int64(header[3])<<16 | int64(header[4])<<8 | int64(header[5])
		case 4:
			if header[4]|header[5]|header[6]|header[7] >= 128 {
				return fmt.Errorf("invalid ID3 frame size")
			}
			size = int64(syncsafeToInt(header[4:8]))
		default:
			size = int64(binary.BigEndian.Uint32(header[4:8]))
		}
		if size <= 0 || size > remaining {
			return fmt.Errorf("invalid ID3 frame bounds")
		}
		remaining -= size
		picture := id == "APIC" || id == "PIC"
		wanted := (picture && wantCover != nil && wantCover()) || (!picture && (id[0] == 'T' || id == "COMM" || id == "USLT" || id == "ULT"))
		flags := byte(0)
		if version != 2 {
			flags = header[9]
		}
		unsupported := (version == 3 && flags&0xc0 != 0) || (version == 4 && flags&0x0c != 0)
		if !wanted || unsupported || size > maxID3FrameBytes {
			if err := skipID3Bytes(reader, size); err != nil {
				return err
			}
			continue
		}
		data := make([]byte, int(size))
		if _, err := io.ReadFull(reader, data); err != nil {
			return err
		}
		if version == 3 && flags&0x20 != 0 || version == 4 && flags&0x40 != 0 {
			if len(data) < 1 {
				continue
			}
			data = data[1:]
		}
		if version == 4 && flags&0x01 != 0 {
			if len(data) < 4 {
				continue
			}
			data = data[4:]
		}
		if tagUnsync || version == 4 && flags&0x02 != 0 {
			data = removeUnsync(data)
		}
		visit(id, data)
	}
	return nil
}

func skipID3Bytes(reader io.Reader, size int64) error {
	if seeker, ok := reader.(io.Seeker); ok {
		_, err := seeker.Seek(size, io.SeekCurrent)
		return err
	}
	_, err := io.CopyN(io.Discard, reader, size)
	return err
}

type id3UnsyncReader struct {
	source  *bufio.Reader
	afterFF bool
}

func (reader *id3UnsyncReader) Read(output []byte) (int, error) {
	count := 0
	for count < len(output) {
		value, err := reader.source.ReadByte()
		if err != nil {
			return count, err
		}
		if reader.afterFF && value == 0 {
			reader.afterFF = false
			continue
		}
		reader.afterFF = value == 0xff
		output[count] = value
		count++
	}
	return count, nil
}
