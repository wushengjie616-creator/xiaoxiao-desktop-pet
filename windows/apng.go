package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"image"
	"image/draw"
	"image/png"
	"sort"
	"time"
)

const pngSignature = "\x89PNG\r\n\x1a\n"

type animation struct {
	frames   []*image.RGBA
	ends     []time.Duration
	duration time.Duration
}

func (a *animation) frame(elapsed time.Duration) *image.RGBA {
	position := elapsed % a.duration
	return a.frames[sort.Search(len(a.ends), func(i int) bool { return position < a.ends[i] })]
}

type frameControl struct {
	bounds  image.Rectangle
	delay   time.Duration
	dispose byte
	blend   byte
}

func decodeAPNG(data []byte) (*animation, error) {
	if !bytes.HasPrefix(data, []byte(pngSignature)) {
		return nil, fmt.Errorf("invalid PNG signature")
	}
	order := binary.BigEndian
	result := &animation{}
	var header []byte
	var palette [][]byte
	var canvas *image.RGBA
	var control *frameControl
	var compressed [][]byte
	var declared uint32

	flush := func() error {
		if control == nil {
			return nil
		}
		if len(compressed) == 0 || len(result.frames) >= int(declared) {
			return fmt.Errorf("invalid frame data or count")
		}
		var encoded bytes.Buffer
		encoded.WriteString(pngSignature)
		frameHeader := append([]byte(nil), header...)
		order.PutUint32(frameHeader[0:4], uint32(control.bounds.Dx()))
		order.PutUint32(frameHeader[4:8], uint32(control.bounds.Dy()))
		pngChunk(&encoded, "IHDR", frameHeader)
		for _, chunk := range palette {
			encoded.Write(chunk)
		}
		for _, payload := range compressed {
			pngChunk(&encoded, "IDAT", payload)
		}
		pngChunk(&encoded, "IEND", nil)
		pixels, err := png.Decode(&encoded)
		if err != nil {
			return err
		}

		var previous *image.RGBA
		if control.dispose == 2 {
			previous = copyImage(canvas)
		}
		op := draw.Src
		if control.blend == 1 {
			op = draw.Over
		}
		draw.Draw(canvas, control.bounds, pixels, image.Point{}, op)
		result.frames = append(result.frames, copyImage(canvas))
		result.duration += control.delay
		result.ends = append(result.ends, result.duration)

		switch control.dispose {
		case 1:
			draw.Draw(canvas, control.bounds, image.Transparent, image.Point{}, draw.Src)
		case 2:
			canvas = previous
		}
		compressed = nil
		return nil
	}

	data = data[len(pngSignature):]
	for len(data) >= 12 {
		size := int(order.Uint32(data[:4]))
		if size > len(data)-12 {
			return nil, fmt.Errorf("truncated PNG chunk")
		}
		kind := string(data[4:8])
		payload := data[8 : 8+size]
		chunk := data[:12+size]
		if crc32.ChecksumIEEE(chunk[4:8+size]) != order.Uint32(chunk[8+size:]) {
			return nil, fmt.Errorf("invalid checksum for %s", kind)
		}
		data = data[12+size:]

		switch kind {
		case "IHDR":
			if len(payload) != 13 || canvas != nil {
				return nil, fmt.Errorf("invalid PNG header")
			}
			w, h := int(order.Uint32(payload[:4])), int(order.Uint32(payload[4:8]))
			if w < 1 || h < 1 || w > 2048 || h > 2048 {
				return nil, fmt.Errorf("unsupported canvas %dx%d", w, h)
			}
			header = payload
			canvas = image.NewRGBA(image.Rect(0, 0, w, h))
		case "PLTE", "tRNS":
			palette = append(palette, chunk)
		case "acTL":
			if len(payload) != 8 || declared != 0 || canvas == nil {
				return nil, fmt.Errorf("invalid animation header")
			}
			declared = order.Uint32(payload[:4])
			if declared < 1 || declared > 300 || uint64(declared)*uint64(canvas.Bounds().Dx()*canvas.Bounds().Dy()) > 40_000_000 {
				return nil, fmt.Errorf("unsupported animation size")
			}
		case "fcTL":
			if len(payload) != 26 || declared == 0 {
				return nil, fmt.Errorf("invalid frame control")
			}
			if err := flush(); err != nil {
				return nil, err
			}
			w, h := int(order.Uint32(payload[4:8])), int(order.Uint32(payload[8:12]))
			x, y := int(order.Uint32(payload[12:16])), int(order.Uint32(payload[16:20]))
			box := image.Rect(x, y, x+w, y+h)
			if box.Empty() || !box.In(canvas.Bounds()) || payload[24] > 2 || payload[25] > 1 {
				return nil, fmt.Errorf("invalid frame bounds or operation")
			}
			num, den := order.Uint16(payload[20:22]), order.Uint16(payload[22:24])
			if den == 0 {
				den = 100
			}
			delay := time.Second * time.Duration(num) / time.Duration(den)
			control = &frameControl{box, max(delay, 25*time.Millisecond), payload[24], payload[25]}
		case "IDAT":
			// An IDAT before any frame control is a separate default image.
			if control != nil {
				compressed = append(compressed, payload)
			}
		case "fdAT":
			if len(payload) < 5 || control == nil {
				return nil, fmt.Errorf("unexpected frame data")
			}
			compressed = append(compressed, payload[4:])
		case "IEND":
			if err := flush(); err != nil {
				return nil, err
			}
			if declared == 0 || len(result.frames) != int(declared) {
				return nil, fmt.Errorf("animation frame count does not match")
			}
			return result, nil
		}
	}
	return nil, fmt.Errorf("missing PNG end")
}

func copyImage(source *image.RGBA) *image.RGBA {
	target := image.NewRGBA(source.Bounds())
	copy(target.Pix, source.Pix)
	return target
}

func pngChunk(out *bytes.Buffer, kind string, data []byte) {
	binary.Write(out, binary.BigEndian, uint32(len(data)))
	start := out.Len()
	out.WriteString(kind)
	out.Write(data)
	binary.Write(out, binary.BigEndian, crc32.ChecksumIEEE(out.Bytes()[start:]))
}
