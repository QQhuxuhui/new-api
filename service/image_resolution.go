package service

import (
	"encoding/base64"
	"encoding/binary"
	"strings"
)

// ImageResolutionTier 图片分辨率等级
type ImageResolutionTier string

const (
	ResolutionTierStandard ImageResolutionTier = "standard" // 2K 及以下
	ResolutionTierHigh     ImageResolutionTier = "4k"       // 4K
)

// High resolution threshold: max(width, height) > 2048 is considered 4K
const highResThreshold = 2048

// IsImageMimeType 判断 mime type 是否为图片类型（大小写不敏感）
func IsImageMimeType(mimeType string) bool {
	mimeType = strings.TrimSpace(strings.ToLower(mimeType))
	return strings.HasPrefix(mimeType, "image")
}

// DetectImageResolutionTier 从 base64 编码的图片数据中检测分辨率等级
// 只解码前几百字节读取图片头部，不需要完整解码
func DetectImageResolutionTier(base64Data string, mimeType string) ImageResolutionTier {
	width, height := getImageDimensions(base64Data, mimeType)
	if width == 0 && height == 0 {
		return ResolutionTierStandard // 无法检测时按标准计费
	}
	maxDim := width
	if height > maxDim {
		maxDim = height
	}
	if maxDim > highResThreshold {
		return ResolutionTierHigh
	}
	return ResolutionTierStandard
}

// getImageDimensions 从 base64 数据中解析图片宽高
func getImageDimensions(base64Data string, mimeType string) (width, height int) {
	// 只需要解码前 1KB 就足够读取任何图片格式的头部
	dataToRead := base64Data
	if len(dataToRead) > 1400 { // ~1KB decoded
		dataToRead = dataToRead[:1400]
	}

	decoded, err := base64.StdEncoding.DecodeString(dataToRead)
	if err != nil {
		// 尝试去掉末尾不完整的 base64 padding
		for len(dataToRead) > 0 && err != nil {
			dataToRead = dataToRead[:len(dataToRead)-1]
			decoded, err = base64.StdEncoding.DecodeString(dataToRead)
		}
		if err != nil {
			return 0, 0
		}
	}

	mimeType = strings.ToLower(mimeType)
	switch {
	case strings.Contains(mimeType, "png"):
		return parsePNGDimensions(decoded)
	case strings.Contains(mimeType, "jpeg") || strings.Contains(mimeType, "jpg"):
		return parseJPEGDimensions(decoded)
	case strings.Contains(mimeType, "webp"):
		return parseWebPDimensions(decoded)
	default:
		// 尝试通过 magic bytes 自动检测
		return detectAndParseDimensions(decoded)
	}
}

// parsePNGDimensions 从 PNG 头部解析宽高
// PNG IHDR chunk: bytes 16-19 = width, bytes 20-23 = height (big-endian)
func parsePNGDimensions(data []byte) (int, int) {
	if len(data) < 24 {
		return 0, 0
	}
	// PNG signature: 137 80 78 71 13 10 26 10
	if data[0] != 0x89 || data[1] != 0x50 || data[2] != 0x4E || data[3] != 0x47 {
		return 0, 0
	}
	width := int(binary.BigEndian.Uint32(data[16:20]))
	height := int(binary.BigEndian.Uint32(data[20:24]))
	return width, height
}

// parseJPEGDimensions 从 JPEG SOF marker 解析宽高
func parseJPEGDimensions(data []byte) (int, int) {
	if len(data) < 2 || data[0] != 0xFF || data[1] != 0xD8 {
		return 0, 0
	}
	i := 2
	for i < len(data)-1 {
		if data[i] != 0xFF {
			i++
			continue
		}
		marker := data[i+1]
		// SOF0 (0xC0) or SOF2 (0xC2) contain dimensions
		if marker == 0xC0 || marker == 0xC2 {
			if i+9 < len(data) {
				height := int(binary.BigEndian.Uint16(data[i+5 : i+7]))
				width := int(binary.BigEndian.Uint16(data[i+7 : i+9]))
				return width, height
			}
			return 0, 0
		}
		// Skip to next marker
		if i+3 < len(data) {
			segLen := int(binary.BigEndian.Uint16(data[i+2 : i+4]))
			i += 2 + segLen
		} else {
			break
		}
	}
	return 0, 0
}

// parseWebPDimensions 从 WebP 头部解析宽高
func parseWebPDimensions(data []byte) (int, int) {
	if len(data) < 30 {
		return 0, 0
	}
	// RIFF....WEBP
	if string(data[0:4]) != "RIFF" || string(data[8:12]) != "WEBP" {
		return 0, 0
	}
	// VP8 lossy
	if string(data[12:16]) == "VP8 " && len(data) >= 30 {
		width := int(binary.LittleEndian.Uint16(data[26:28])) & 0x3FFF
		height := int(binary.LittleEndian.Uint16(data[28:30])) & 0x3FFF
		return width, height
	}
	// VP8L lossless
	if string(data[12:16]) == "VP8L" && len(data) >= 25 {
		bits := binary.LittleEndian.Uint32(data[21:25])
		width := int(bits&0x3FFF) + 1
		height := int((bits>>14)&0x3FFF) + 1
		return width, height
	}
	// VP8X extended format
	if string(data[12:16]) == "VP8X" && len(data) >= 30 {
		width := littleEndian24(data[24:27]) + 1
		height := littleEndian24(data[27:30]) + 1
		return width, height
	}
	return 0, 0
}

func littleEndian24(data []byte) int {
	if len(data) < 3 {
		return 0
	}
	return int(data[0]) | int(data[1])<<8 | int(data[2])<<16
}

// detectAndParseDimensions 通过 magic bytes 自动检测格式并解析
func detectAndParseDimensions(data []byte) (int, int) {
	if len(data) < 4 {
		return 0, 0
	}
	if data[0] == 0x89 && data[1] == 0x50 {
		return parsePNGDimensions(data)
	}
	if data[0] == 0xFF && data[1] == 0xD8 {
		return parseJPEGDimensions(data)
	}
	if string(data[0:4]) == "RIFF" {
		return parseWebPDimensions(data)
	}
	return 0, 0
}
