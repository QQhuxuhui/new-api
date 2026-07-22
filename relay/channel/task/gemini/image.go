package gemini

import (
	"encoding/base64"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

const maxVeoImageSize = 20 * 1024 * 1024 // 20 MB

// ExtractMultipartImage reads the first `input_reference` file from a multipart
// form upload and returns a VeoImageInput. Returns nil if no file is present.
func ExtractMultipartImage(c *gin.Context, info *relaycommon.RelayInfo) *VeoImageInput {
	mf, err := c.MultipartForm()
	if err != nil {
		return nil
	}
	files, exists := mf.File["input_reference"]
	if !exists || len(files) == 0 {
		return nil
	}
	fh := files[0]
	if fh.Size > maxVeoImageSize {
		return nil
	}
	file, err := fh.Open()
	if err != nil {
		return nil
	}
	defer file.Close()

	fileBytes, err := io.ReadAll(file)
	if err != nil {
		return nil
	}

	mimeType := fh.Header.Get("Content-Type")
	if mimeType == "" || mimeType == "application/octet-stream" {
		mimeType = http.DetectContentType(fileBytes)
	}

	info.Action = constant.TaskActionGenerate
	return &VeoImageInput{
		BytesBase64Encoded: base64.StdEncoding.EncodeToString(fileBytes),
		MimeType:           mimeType,
	}
}

// VeoMaxReferenceImages caps how many images are forwarded upstream (Veo 3.1:
// standard 3 / fast 2; adobe2api clamps per model, this is the hard ceiling).
const VeoMaxReferenceImages = 3

// CollectVeoImages parses up to VeoMaxReferenceImages images from the request
// (multipart input_reference first, otherwise the images array) and splits them
// into the first-frame image plus additional reference images.
func CollectVeoImages(c *gin.Context, info *relaycommon.RelayInfo, images []string) (*VeoImageInput, []VeoImageInput) {
	parsed := make([]*VeoImageInput, 0, VeoMaxReferenceImages)
	if img := ExtractMultipartImage(c, info); img != nil {
		parsed = append(parsed, img)
	}
	for _, raw := range images {
		if len(parsed) >= VeoMaxReferenceImages {
			break
		}
		if img := ParseImageInput(raw); img != nil {
			parsed = append(parsed, img)
		}
	}
	if len(parsed) == 0 {
		return nil, nil
	}
	if len(parsed) > VeoMaxReferenceImages {
		parsed = parsed[:VeoMaxReferenceImages]
	}
	refs := make([]VeoImageInput, 0, len(parsed)-1)
	for _, img := range parsed[1:] {
		refs = append(refs, *img)
	}
	return parsed[0], refs
}

// ParseImageInput parses an image string (data URI or raw base64) into a
// VeoImageInput. Returns nil if the input is empty or invalid.
// TODO: support downloading HTTP URL images and converting to base64
func ParseImageInput(imageStr string) *VeoImageInput {
	imageStr = strings.TrimSpace(imageStr)
	if imageStr == "" {
		return nil
	}

	if strings.HasPrefix(imageStr, "data:") {
		return parseDataURI(imageStr)
	}

	raw, err := base64.StdEncoding.DecodeString(imageStr)
	if err != nil {
		return nil
	}
	return &VeoImageInput{
		BytesBase64Encoded: imageStr,
		MimeType:           http.DetectContentType(raw),
	}
}

func parseDataURI(uri string) *VeoImageInput {
	// data:image/png;base64,iVBOR...
	rest := uri[len("data:"):]
	idx := strings.Index(rest, ",")
	if idx < 0 {
		return nil
	}
	meta := rest[:idx]
	b64 := rest[idx+1:]
	if b64 == "" {
		return nil
	}

	mimeType := "application/octet-stream"
	parts := strings.SplitN(meta, ";", 2)
	if len(parts) >= 1 && parts[0] != "" {
		mimeType = parts[0]
	}

	return &VeoImageInput{
		BytesBase64Encoded: b64,
		MimeType:           mimeType,
	}
}
