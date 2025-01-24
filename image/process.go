package image

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"  // Register GIF format
	_ "image/jpeg" // Register JPEG format
	_ "image/png"  // Register PNG format

	"github.com/buckket/go-blurhash"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
)

const (
	// Define BlurHash components (commonly 4x4 or 9x4)
	componentsX = 4
	componentsY = 4
)

// ProcessImage takes a byte array of image data, decodes it,
// retrieves dimensions, calculates BlurHash, and returns the serialized protobuf.
func ProcessImage(imageData []byte) (*commonpb.ImageInfo, error) {
	// Check if image data is empty
	if len(imageData) == 0 {
		return nil, fmt.Errorf("image data is empty")
	}

	// Decode the image
	img, _, err := image.Decode(bytes.NewReader(imageData))
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	// Get image bounds
	bounds := img.Bounds()
	width := int32(bounds.Dx())
	height := int32(bounds.Dy())

	// Calculate BlurHash
	blurhashStr, err := blurhash.Encode(componentsX, componentsY, img)
	if err != nil {
		return nil, fmt.Errorf("failed to encode blurhash: %w", err)
	}

	// Create ImageInfo protobuf message
	info := &commonpb.ImageInfo{
		Width:    width,
		Height:   height,
		BlurHash: blurhashStr,
	}

	return info, nil
}
