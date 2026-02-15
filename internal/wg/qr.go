package wg

import (
	"fmt"

	qrcode "github.com/skip2/go-qrcode"
)

// GenerateQRCode generates a QR code PNG image from the given WireGuard config content.
// size specifies the width and height of the output image in pixels.
func GenerateQRCode(confContent string, size int) ([]byte, error) {
	if confContent == "" {
		return nil, fmt.Errorf("generate qr code: config content is empty")
	}
	if size <= 0 {
		size = 256
	}

	png, err := qrcode.Encode(confContent, qrcode.Medium, size)
	if err != nil {
		return nil, fmt.Errorf("generate qr code: %w", err)
	}
	return png, nil
}
