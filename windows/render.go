package main

import (
	"image"
	"math"
)

// The source is already premultiplied. Windows expects top-down BGRA in the DIB.
func renderBGRA(pixels []byte, width, height int, src *image.RGBA, mirror bool) {
	clear(pixels)
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	scale := min(float64(width)/float64(sw), float64(height)/float64(sh))
	w, h := max(1, int(math.Round(float64(sw)*scale))), max(1, int(math.Round(float64(sh)*scale)))
	left, top := (width-w)/2, height-h
	for y := 0; y < h; y++ {
		sy := min(float64(sh-1), max(0, (float64(y)+0.5)*float64(sh)/float64(h)-0.5))
		y0 := int(sy)
		y1 := min(y0+1, sh-1)
		wy := sy - float64(y0)
		for x := 0; x < w; x++ {
			sx := min(float64(sw-1), max(0, (float64(x)+0.5)*float64(sw)/float64(w)-0.5))
			if mirror {
				sx = float64(sw-1) - sx
			}
			x0 := int(sx)
			x1 := min(x0+1, sw-1)
			wx := sx - float64(x0)
			i0, i1 := y0*src.Stride, y1*src.Stride
			at := ((top+y)*width + left + x) * 4
			for c := 0; c < 4; c++ {
				channel := c
				if c == 0 || c == 2 {
					channel = 2 - c
				}
				a := float64(src.Pix[i0+x0*4+channel])*(1-wx) + float64(src.Pix[i0+x1*4+channel])*wx
				b := float64(src.Pix[i1+x0*4+channel])*(1-wx) + float64(src.Pix[i1+x1*4+channel])*wx
				pixels[at+c] = byte(a*(1-wy) + b*wy + 0.5)
			}
		}
	}
}
