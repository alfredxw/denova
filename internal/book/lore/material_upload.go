package lore

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"log/slog"
	"os"
	"path"
	"strings"
	"time"

	"denova/internal/portablepath"
	"github.com/google/uuid"
)

const MaxMaterialUploadBytes = 64 * 1024 * 1024

var ErrMaterialInvalid = errors.New("material must contain a valid PNG, JPEG, MP3 or PCM WAV file")
var ErrMaterialTooLarge = errors.New("material exceeds 64 MiB")

// MaterialFile is a ready payload from an upload or generator. Source and Entry
// are committed with the file reference; callers never overwrite existing assets.
type MaterialFile struct {
	Filename string
	Data     []byte
	Source   AssetSource
	Entry    MaterialEntry
}

func (s *Store) UploadMaterial(ctx context.Context, id, filename string, data []byte) (Item, error) {
	return s.SaveMaterial(ctx, id, MaterialFile{Filename: filename, Data: data, Source: AssetSource{Kind: "upload"}})
}

// SaveMaterial owns only the newly allocated directory until the atomic
// collection commit succeeds. The original filename is display metadata only.
func (s *Store) SaveMaterial(ctx context.Context, id string, file MaterialFile) (Item, error) {
	data := file.Data
	if len(data) > MaxMaterialUploadBytes {
		return Item{}, ErrMaterialTooLarge
	}
	mime, ext, err := MaterialFormat(data)
	if err != nil {
		return Item{}, err
	}
	if _, err := s.ReadAny(id); err != nil {
		return Item{}, err
	}
	assetID := "asset_" + uuid.NewString()
	dir := "assets/lore/media/" + assetID
	relative := dir + "/file." + ext
	if err := portablepath.CheckNoCollision(s.workspace, relative); err != nil {
		return Item{}, err
	}
	root, err := os.OpenRoot(s.workspace)
	if err != nil {
		return Item{}, err
	}
	defer root.Close()
	if err := root.MkdirAll(dir, 0755); err != nil {
		return Item{}, err
	}
	committed := false
	defer func() {
		if !committed {
			// A failed atomic replace may have reached disk before a sync error.
			assets, readErr := s.Assets()
			if readErr != nil {
				slog.WarnContext(ctx, "[lore-material] retain file after uncertain commit", "path", relative, "error", readErr)
				return
			}
			for _, a := range assets {
				if a.Path == relative {
					return
				}
			}
			if err := root.RemoveAll(dir); err != nil {
				slog.ErrorContext(ctx, "[lore-material] remove uncommitted file failed", "path", dir, "error", err)
			}
		}
	}()
	if err := ctx.Err(); err != nil {
		return Item{}, err
	}
	if err := root.WriteFile(relative, data, 0644); err != nil {
		return Item{}, err
	}
	a := Asset{ID: assetID, Path: relative, OriginalName: path.Base(strings.ReplaceAll(file.Filename, `\`, "/")), MIMEType: mime, SizeBytes: len(data), CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), Source: file.Source}
	if err := ctx.Err(); err != nil {
		return Item{}, err
	}
	item, err := s.AttachAsset(id, a, file.Entry)
	if err != nil {
		return Item{}, err
	}
	committed = true
	slog.InfoContext(ctx, "[lore-material] saved", "item_id", id, "asset_id", assetID, "path", relative, "source", file.Source.Kind)
	return item, nil
}

// MaterialFormat validates supported upload containers and returns their canonical MIME and extension.
func MaterialFormat(data []byte) (mime, ext string, err error) {
	if len(data) == 0 {
		return "", "", ErrMaterialInvalid
	}
	if _, format, e := image.DecodeConfig(bytes.NewReader(data)); e == nil && (format == "png" || format == "jpeg") {
		return "image/" + format, format, nil
	}
	// WAV is a chunk container. Require a supported format and nonempty, bounded
	// audio data instead of trusting the extension or just the RIFF signature.
	if len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WAVE" {
		end := int64(binary.LittleEndian.Uint32(data[4:8])) + 8
		if end > int64(len(data)) {
			return "", "", ErrMaterialInvalid
		}
		formatOK, audioOK := false, false
		for offset := int64(12); offset+8 <= end; {
			n := int64(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
			start := offset + 8
			if start+n > end {
				return "", "", ErrMaterialInvalid
			}
			switch string(data[offset : offset+4]) {
			case "fmt ":
				if n >= 16 {
					f := binary.LittleEndian.Uint16(data[start : start+2])
					formatOK = (f == 1 || f == 3) && binary.LittleEndian.Uint16(data[start+2:start+4]) > 0 && binary.LittleEndian.Uint32(data[start+4:start+8]) > 0
				}
			case "data":
				audioOK = n > 0
			}
			offset = start + n + n%2
		}
		if formatOK && audioOK {
			return "audio/wav", "wav", nil
		}
	}
	// Check an MPEG Layer III frame, after the optional ID3v2 tag. This bounds
	// the first encoded frame without decoding large uploads on the request path.
	offset := 0
	if len(data) >= 10 && string(data[:3]) == "ID3" {
		for _, b := range data[6:10] {
			if b&0x80 != 0 {
				return "", "", ErrMaterialInvalid
			}
			offset = (offset << 7) | int(b)
		}
		offset += 10
		if data[5]&0x10 != 0 {
			offset += 10
		}
	}
	if offset+4 <= len(data) {
		h := binary.BigEndian.Uint32(data[offset : offset+4])
		version := (h >> 19) & 3
		bitrate := (h >> 12) & 15
		rate := (h >> 10) & 3
		if h&0xffe00000 == 0xffe00000 && version != 1 && (h>>17)&3 == 1 && bitrate > 0 && bitrate < 15 && rate < 3 {
			rates := [3]int{44100, 48000, 32000}
			bps := [15]int{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320}
			sampleRate := rates[rate]
			factor := 144
			if version != 3 {
				sampleRate /= 2
				factor = 72
				bps = [15]int{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160}
				if version == 0 {
					sampleRate /= 2
				}
			}
			size := factor*bps[bitrate]*1000/sampleRate + int((h>>9)&1)
			if size >= 4 && offset+size <= len(data) {
				return "audio/mpeg", "mp3", nil
			}
		}
	}
	return "", "", ErrMaterialInvalid
}
