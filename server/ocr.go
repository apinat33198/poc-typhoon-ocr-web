package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png" // register PNG decoder
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	ocrImageDim      = 1800  // longest side of the image sent to the model
	anchorTextLength = 8000  // max chars of anchor text (default/structure)
	maxTokens        = 16384 // same as upstream
)

// --- message structures (OpenAI chat format) ---------------------------

type contentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *imageURL `json:"image_url,omitempty"`
}

type imageURL struct {
	URL string `json:"url"`
}

type message struct {
	Role    string        `json:"role"`
	Content []contentPart `json:"content"`
}

// buildOCRMessages mirrors typhoon_ocr.prepare_ocr_messages: render the page
// (or load the image), build the anchor text when the mode needs it, and wrap
// both in a single user message.
func buildOCRMessages(doc *Doc, page int, taskType, figureLanguage string) ([]message, error) {
	var imageDataURL, anchor string

	if doc.IsPDF {
		png, err := renderPDFPagePNG(doc.Path, page, ocrImageDim)
		if err != nil {
			return nil, err
		}
		imageDataURL = "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
		if taskType != "v1.5" {
			anchor, err = pdfAnchorText(doc.Path, page, anchorTextLength)
			if err != nil {
				return nil, err
			}
		}
	} else {
		img, err := loadImage(doc.Path)
		if err != nil {
			return nil, err
		}
		// Upstream resizes images for v1.5 only; other modes send them as-is.
		if taskType == "v1.5" {
			img = resizeToLongest(img, ocrImageDim)
		}
		jpg, err := encodeJPEG(img)
		if err != nil {
			return nil, err
		}
		imageDataURL = "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(jpg)
		if taskType != "v1.5" {
			b := img.Bounds()
			anchor = imageAnchorText(b.Dx(), b.Dy())
		}
	}

	var prompt string
	switch taskType {
	case "default":
		prompt = promptDefault(anchor)
	case "structure":
		prompt = promptStructure(anchor)
	default:
		prompt = promptV15(figureLanguage)
	}

	return []message{{
		Role: "user",
		Content: []contentPart{
			{Type: "text", Text: prompt},
			{Type: "image_url", ImageURL: &imageURL{URL: imageDataURL}},
		},
	}}, nil
}

// --- image helpers ------------------------------------------------------

func loadImage(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	return img, err
}

// resizeToLongest scales the image so its longest side equals target
// (bilinear, no external deps). Mirrors upstream resize_if_needed.
func resizeToLongest(src image.Image, target int) image.Image {
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	if sw <= 300 && sh <= 300 {
		return src // upstream leaves tiny images alone
	}
	var dw, dh int
	if sw >= sh {
		dw, dh = target, int(float64(sh)*float64(target)/float64(sw))
	} else {
		dw, dh = int(float64(sw)*float64(target)/float64(sh)), target
	}
	if dw == sw && dh == sh {
		return src
	}

	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < dh; y++ {
		fy := (float64(y)+0.5)*float64(sh)/float64(dh) - 0.5
		y0 := clamp(int(fy), 0, sh-1)
		y1 := clamp(y0+1, 0, sh-1)
		wy := fy - float64(y0)
		for x := 0; x < dw; x++ {
			fx := (float64(x)+0.5)*float64(sw)/float64(dw) - 0.5
			x0 := clamp(int(fx), 0, sw-1)
			x1 := clamp(x0+1, 0, sw-1)
			wx := fx - float64(x0)

			r00, g00, b00, _ := src.At(b.Min.X+x0, b.Min.Y+y0).RGBA()
			r10, g10, b10, _ := src.At(b.Min.X+x1, b.Min.Y+y0).RGBA()
			r01, g01, b01, _ := src.At(b.Min.X+x0, b.Min.Y+y1).RGBA()
			r11, g11, b11, _ := src.At(b.Min.X+x1, b.Min.Y+y1).RGBA()

			lerp2 := func(a, bv, c, d uint32) uint8 {
				top := float64(a)*(1-wx) + float64(bv)*wx
				bot := float64(c)*(1-wx) + float64(d)*wx
				return uint8((top*(1-wy) + bot*wy) / 257)
			}
			i := dst.PixOffset(x, y)
			dst.Pix[i+0] = lerp2(r00, r10, r01, r11)
			dst.Pix[i+1] = lerp2(g00, g10, g01, g11)
			dst.Pix[i+2] = lerp2(b00, b10, b01, b11)
			dst.Pix[i+3] = 255
		}
	}
	return dst
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func encodeJPEG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// --- model client -------------------------------------------------------

type chatRequest struct {
	Model             string    `json:"model"`
	Messages          []message `json:"messages"`
	MaxTokens         int       `json:"max_tokens"`
	Temperature       float64   `json:"temperature"`
	TopP              float64   `json:"top_p"`
	RepetitionPenalty float64   `json:"repetition_penalty"` // vLLM extension, same as upstream extra_body
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

var httpClient = &http.Client{Timeout: 10 * time.Minute}

// ocrPage renders one page, calls the model, and returns the markdown.
func ocrPage(ctx context.Context, doc *Doc, page int, taskType, figureLanguage string) (string, error) {
	messages, err := buildOCRMessages(doc, page, taskType, figureLanguage)
	if err != nil {
		return "", err
	}
	return callModel(ctx, messages, taskType)
}

// callModel sends the messages to the OpenAI-compatible endpoint with the
// same sampling parameters as upstream and unwraps the result for the mode.
func callModel(ctx context.Context, messages []message, taskType string) (string, error) {
	repPenalty := 1.2
	if taskType == "v1.5" {
		repPenalty = 1.1
	}
	body, err := json.Marshal(chatRequest{
		Model:             cfg.Model,
		Messages:          messages,
		MaxTokens:         maxTokens,
		Temperature:       0.1,
		TopP:              0.6,
		RepetitionPenalty: repPenalty,
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", cfg.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 32<<20))

	var parsed chatResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(respBody), 300))
	}
	if parsed.Error != nil {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, parsed.Error.Message)
	}
	if resp.StatusCode != 200 || len(parsed.Choices) == 0 {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(respBody), 300))
	}

	raw := parsed.Choices[0].Message.Content

	// default/structure wrap output as {"natural_text": ...}; v1.5 is plain markdown
	if taskType == "v1.5" {
		return strings.TrimSpace(raw), nil
	}
	var wrapped struct {
		NaturalText string `json:"natural_text"`
	}
	if err := json.Unmarshal([]byte(raw), &wrapped); err == nil && wrapped.NaturalText != "" {
		return strings.TrimSpace(wrapped.NaturalText), nil
	}
	return strings.TrimSpace(raw), nil // fall back to whatever the model returned
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
