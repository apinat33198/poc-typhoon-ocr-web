package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

const popplerHint = "Is poppler installed? (apt-get install poppler-utils / brew install poppler)"

// pdfPageCount returns the number of pages using `pdfinfo`.
func pdfPageCount(path string) (int, error) {
	out, err := exec.Command("pdfinfo", path).Output()
	if err != nil {
		return 0, fmt.Errorf("pdfinfo failed: %v. %s", err, popplerHint)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "Pages:") {
			n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "Pages:")))
			if err != nil {
				return 0, fmt.Errorf("could not parse page count: %v", err)
			}
			return n, nil
		}
	}
	return 0, fmt.Errorf("pdfinfo output had no Pages line")
}

// renderPDFPagePNG rasterizes one page with `pdftoppm`, scaling the longest
// side to scaleTo pixels (the same target the upstream package uses).
func renderPDFPagePNG(path string, page, scaleTo int) ([]byte, error) {
	cmd := exec.Command("pdftoppm", "-png",
		"-f", strconv.Itoa(page), "-l", strconv.Itoa(page),
		"-scale-to", strconv.Itoa(scaleTo), path)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("pdftoppm failed: %v: %s. %s", err, errBuf.String(), popplerHint)
	}
	return out.Bytes(), nil
}

// --- anchor text -------------------------------------------------------
//
// The default/structure prompts include "anchor text": page dimensions plus
// positioned text extracted from the PDF layer, which helps the model align
// its output. Upstream builds this with pypdf; here we approximate it with
// `pdftotext -bbox`, which yields word-level boxes. v1.5 mode doesn't use
// anchor text at all.

type bboxPage struct {
	Width  float64 `xml:"width,attr"`
	Height float64 `xml:"height,attr"`
	Words  []struct {
		XMin float64 `xml:"xMin,attr"`
		YMin float64 `xml:"yMin,attr"`
		Text string  `xml:",chardata"`
	} `xml:"word"`
}

type bboxHTML struct {
	Pages []bboxPage `xml:"body>doc>page"`
}

func pdfAnchorText(path string, page, maxLen int) (string, error) {
	cmd := exec.Command("pdftotext",
		"-f", strconv.Itoa(page), "-l", strconv.Itoa(page), "-bbox", path, "-")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("pdftotext failed: %v. %s", err, popplerHint)
	}

	var doc bboxHTML
	if err := xml.Unmarshal(out, &doc); err != nil || len(doc.Pages) == 0 {
		return "", fmt.Errorf("could not parse pdftotext output: %v", err)
	}
	p := doc.Pages[0]

	var sb strings.Builder
	fmt.Fprintf(&sb, "Page dimensions: %.1fx%.1f\n", p.Width, p.Height)
	for _, wd := range p.Words {
		text := strings.TrimSpace(wd.Text)
		if text == "" {
			continue
		}
		line := fmt.Sprintf("[%dx%d]%s\n", int(wd.XMin), int(wd.YMin), text)
		if sb.Len()+len(line) > maxLen {
			break
		}
		sb.WriteString(line)
	}
	return sb.String(), nil
}

func imageAnchorText(width, height int) string {
	return fmt.Sprintf("Page dimensions: %.1fx%.1f\n[Image 0x0 to %dx%d]\n",
		float64(width), float64(height), width, height)
}
