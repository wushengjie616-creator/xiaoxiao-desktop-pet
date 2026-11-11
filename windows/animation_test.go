package main

import (
	"bytes"
	"compress/zlib"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"image"
	"image/color"
	"reflect"
	"testing"
	"time"

	xiaoxiao "xiaoxiao-desktop-pet"
)

func TestBundledAnimations(t *testing.T) {
	// Reference frame durations and digests come from independently decoding
	// the shipped APNGs with Pillow. Alpha and fully opaque colors avoid
	// implementation differences in rounding premultiplied edge colors.
	expected := map[string]struct {
		count, millis int
		digest        string
	}{
		"belly":     {28, 3523, "c09f9275e423ba7b34e8c423195507b34245e15e9597c76a4d3d5c058c4826aa"},
		"grooming":  {36, 2990, "bd2560af78186916e1cc2e83072572224fc511513aaf054c7321b43c1fae5f53"},
		"resting":   {20, 4742, "9a638ac008a82e18b7be30c859e6faaafea4f4fdb36af74ac27121dfb7861976"},
		"scaling":   {33, 2640, "1d46672ef7b47e2d05ad5ccf4c8c5dd11f86026bb3841acaf1b2440446669de7"},
		"sitting":   {40, 5232, "26f8eb10d9bda7a082db6def0a5b7d6978bf1569f26cf03a15a57abacabbb7e7"},
		"sleeping":  {36, 3960, "5379ad424c1554b986fcfe6d7c940e74a8f6a91559a0bb5e0696ab0227be4f2a"},
		"spinning":  {48, 2160, "7085781fa492f4b0adaf352d30009e26ee740a802826a304de7f7e6e60bd2a52"},
		"sprinting": {24, 720, "d3721dce0dad4f982f19863d8abb6d81728a2abf65b4aa28f65373c54c4b3eb8"},
		"tail":      {36, 2448, "19c5c97f0c2a099de25a74cd56cdd9d07841c9b35b4770fcfb144aaf1b224668"},
		"walking":   {72, 2160, "87b2fbdb90e4d7dfa7c058ce00248d6f3ff27bd0c1baa62b11ff79a60a2ef95e"},
		"watching":  {38, 3874, "e3fcebea5a032ea871720711eb05a41a538de12cdc89c1bc474fba839db5de39"},
	}
	if len(poses) != len(expected) {
		t.Fatal("pose catalog differs from the verified assets")
	}
	for _, pose := range poses {
		t.Run(pose.key, func(t *testing.T) {
			ref, ok := expected[pose.key]
			if !ok {
				t.Fatal("missing expected asset")
			}
			data, err := xiaoxiao.Animation(pose.key)
			if err != nil {
				t.Fatal(err)
			}
			clip, err := decodeAPNG(data)
			if err != nil {
				t.Fatal(err)
			}
			if len(clip.frames) != ref.count || clip.duration != time.Duration(ref.millis)*time.Millisecond {
				t.Fatalf("got %d frames over %s, expected %d over %dms", len(clip.frames), clip.duration, ref.count, ref.millis)
			}
			hash := sha256.New()
			for _, frame := range clip.frames {
				alpha := make([]byte, 0, len(frame.Pix)/4)
				opaque := make([]byte, 0, len(frame.Pix))
				for i := 0; i < len(frame.Pix); i += 4 {
					a := frame.Pix[i+3]
					alpha = append(alpha, a)
					if a == 255 {
						opaque = append(opaque, frame.Pix[i:i+3]...)
					}
				}
				hash.Write(alpha)
				hash.Write(opaque)
			}
			if got := hex.EncodeToString(hash.Sum(nil)); got != ref.digest {
				t.Fatalf("frame contents differ from reference: %s", got)
			}
			if clip.frame(clip.ends[0]) != clip.frames[1] || clip.frame(clip.duration) != clip.frames[0] {
				t.Fatal("frame boundary or loop timing is incorrect")
			}
		})
	}
}

func TestBadAnimationFails(t *testing.T) {
	data, err := xiaoxiao.Animation("walking")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeAPNG(data[:len(data)-12]); err == nil {
		t.Fatal("accepted an animation without an IEND")
	}
	data[20] ^= 0x10
	if _, err := decodeAPNG(data); err == nil {
		t.Fatal("accepted an invalid checksum")
	}
}

func TestRenderPremultipliedMirror(t *testing.T) {
	frame := image.NewRGBA(image.Rect(0, 0, 2, 1))
	copy(frame.Pix, []byte{0x60, 0x20, 0x10, 0x80, 0x11, 0x22, 0x33, 0xff})
	pixels := make([]byte, 16)
	renderBGRA(pixels, 2, 2, frame, true)
	expected := []byte{0, 0, 0, 0, 0, 0, 0, 0, 0x33, 0x22, 0x11, 0xff, 0x10, 0x20, 0x60, 0x80}
	if !reflect.DeepEqual(pixels, expected) {
		t.Fatalf("incorrect mirrored, bottom-aligned BGRA: %v", pixels)
	}
}

func TestPatrolPausesAtBothEdges(t *testing.T) {
	now := time.Unix(0, 0)
	walk := patrol{x: 20, direction: -1}
	if walk.advance(now, .5, 20, 0, 100) || walk.x != 10 {
		t.Fatalf("expected first step to move left: %+v", walk)
	}
	edge := now.Add(time.Second)
	walk.advance(edge, .5, 20, 0, 100)
	if walk.x != 0 || walk.restUntil != edge.Add(2*time.Second) {
		t.Fatalf("expected stop at left edge: %+v", walk)
	}
	if walk.advance(edge.Add(1999*time.Millisecond), 1, 100, 0, 100) || walk.x != 0 {
		t.Fatal("moved during edge rest")
	}
	if !walk.advance(edge.Add(2*time.Second), .5, 20, 0, 100) || walk.direction != 1 || walk.x != 0 {
		t.Fatal("did not stand up facing the return direction")
	}
	walk.advance(edge.Add(3*time.Second), 1, 120, 0, 100)
	if walk.x != 100 || walk.restUntil != edge.Add(5*time.Second) {
		t.Fatalf("expected stop at right edge: %+v", walk)
	}
}

func TestAPNGOverRestoresPreviousCanvas(t *testing.T) {
	var stream bytes.Buffer
	stream.WriteString(pngSignature)
	header := []byte{0, 0, 0, 2, 0, 0, 0, 1, 8, 6, 0, 0, 0}
	pngChunk(&stream, "IHDR", header)
	pngChunk(&stream, "acTL", []byte{0, 0, 0, 3, 0, 0, 0, 0})
	add := func(sequence uint32, width, x int, pixel color.NRGBA, dispose, blend byte) {
		t.Helper()
		var control [26]byte
		binary.BigEndian.PutUint32(control[:4], sequence)
		binary.BigEndian.PutUint32(control[4:8], uint32(width))
		binary.BigEndian.PutUint32(control[8:12], 1)
		binary.BigEndian.PutUint32(control[12:16], uint32(x))
		control[21], control[23] = 1, 10
		control[24], control[25] = dispose, blend
		pngChunk(&stream, "fcTL", control[:])
		var encoded bytes.Buffer
		writer := zlib.NewWriter(&encoded)
		row := append([]byte{0}, bytes.Repeat([]byte{pixel.R, pixel.G, pixel.B, pixel.A}, width)...)
		if _, err := writer.Write(row); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		if sequence == 0 {
			pngChunk(&stream, "IDAT", encoded.Bytes())
		} else {
			var index [4]byte
			binary.BigEndian.PutUint32(index[:], sequence+1)
			pngChunk(&stream, "fdAT", append(index[:], encoded.Bytes()...))
		}
	}
	add(0, 2, 0, color.NRGBA{R: 255, A: 255}, 0, 0)
	add(1, 1, 1, color.NRGBA{B: 255, A: 128}, 2, 1)
	add(3, 1, 0, color.NRGBA{G: 255, A: 255}, 0, 0)
	pngChunk(&stream, "IEND", nil)
	clip, err := decodeAPNG(stream.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if got := clip.frames[1].RGBAAt(1, 0); got != (color.RGBA{R: 127, B: 128, A: 255}) {
		t.Fatalf("second frame did not composite its alpha over red: %#v", got)
	}
	if got := clip.frames[2].RGBAAt(1, 0); got != (color.RGBA{R: 255, A: 255}) {
		t.Fatalf("third frame did not restore the red canvas: %#v", got)
	}
}
