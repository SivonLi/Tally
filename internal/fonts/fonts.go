// Package fonts embeds and registers Tally's UI fonts: Allo for Latin, numerals,
// and symbols, paired with LXGW WenKai GB Screen for Simplified Chinese.
package fonts

import (
	"bytes"
	_ "embed"
	"io"
	"log"
	"sync"

	"github.com/egoist/mygo/ui"
	"github.com/klauspost/compress/zstd"
)

const (
	// LatinFamily is the registered family name for Allo (SIL OFL 1.1).
	LatinFamily = "Allo"
	// CJKFamily is the registered family name for LXGW WenKai GB Screen (SIL OFL 1.1).
	CJKFamily = "LXGW WenKai Screen"
	// UIFontFamily is the comma-separated font fallback chain passed to ui.Theme.Font.
	UIFontFamily = LatinFamily + ", " + CJKFamily
)

var (
	//go:embed Allo-Regular.otf
	alloRegular []byte
	//go:embed Allo-Medium.otf
	alloMedium []byte
	//go:embed Allo-SemiBold.otf
	alloSemiBold []byte
	//go:embed Allo-Bold.otf
	alloBold []byte
	//go:embed LXGWWenKaiGBScreen.ttf.zst
	lxgwWenKaiZst []byte

	registerOnce sync.Once
)

// Register registers the embedded Allo and LXGW WenKai Screen fonts with MyGo.
// It is safe to call repeatedly from any goroutine.
func Register() {
	registerOnce.Do(func() {
		for _, face := range [][]byte{alloRegular, alloMedium, alloSemiBold, alloBold} {
			if err := ui.RegisterFont(face, LatinFamily); err != nil {
				log.Printf("fonts: register %s: %v", LatinFamily, err)
			}
		}
		if data, err := decompressZstd(lxgwWenKaiZst); err != nil {
			log.Printf("fonts: decompress %s: %v", CJKFamily, err)
		} else if err := ui.RegisterFont(data, CJKFamily); err != nil {
			log.Printf("fonts: register %s: %v", CJKFamily, err)
		}
	})
}

func decompressZstd(compressed []byte) ([]byte, error) {
	dec, err := zstd.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, err
	}
	defer dec.Close()
	return io.ReadAll(dec)
}
