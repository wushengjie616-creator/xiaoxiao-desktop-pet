// Package xiaoxiao embeds the same transparent animations used by the Mac app.
package xiaoxiao

import "embed"

//go:embed motions/*.png
var animations embed.FS

func Animation(name string) ([]byte, error) {
	return animations.ReadFile("motions/" + name + ".png")
}
