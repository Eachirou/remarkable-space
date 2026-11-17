package server

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	panelWidth         = 1404
	panelHeight        = 1872
	panelScale         = 3
	panelLeft          = 212
	panelTop           = 1540
	panelRight         = 1192
	panelBottom        = 1800
	panelMaxLineLength = 52
	panelLineSpacing   = 36
)

type contactPanelRequest struct {
	Name       string `json:"name"`
	Email      string `json:"email"`
	Phone      string `json:"phone"`
	Message    string `json:"message"`
	Background string `json:"background"`
}

// createContactPanel creates a PNG that can be selected as a normal sleep screen.
// The panel is deliberately opaque so contact details remain readable on any artwork.
func (s *Server) createContactPanel(w http.ResponseWriter, r *http.Request) {
	var req contactPanelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, http.StatusBadRequest, "contact panel must be JSON")
		return
	}
	if strings.TrimSpace(req.Name) == "" && strings.TrimSpace(req.Email) == "" &&
		strings.TrimSpace(req.Phone) == "" && strings.TrimSpace(req.Message) == "" {
		httpError(w, http.StatusBadRequest, "at least one contact field is required")
		return
	}

	img := image.NewRGBA(image.Rect(0, 0, panelWidth, panelHeight))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	if req.Background != "" {
		path, err := safeJoin(s.cfg.ScreensDir, req.Background)
		if err != nil {
			httpError(w, http.StatusBadRequest, "invalid background")
			return
		}
		if err := drawPNGBackground(img, path); err != nil {
			httpError(w, http.StatusBadRequest, fmt.Sprintf("background must be a PNG: %v", err))
			return
		}
	}

	panel := image.Rect(panelLeft, panelTop, panelRight, panelBottom)
	draw.Draw(img, panel, &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	drawPanelBorder(img, panel)

	lines := make([]string, 0, 8)
	for _, value := range []string{req.Name, req.Email, req.Phone} {
		if value = strings.TrimSpace(value); value != "" {
			lines = append(lines, wrapPanelText(normalizePanelText(value), panelMaxLineLength)...)
		}
	}
	for _, line := range wrapPanelText(normalizePanelText(req.Message), panelMaxLineLength) {
		lines = append(lines, line)
	}
	if len(lines) > 5 {
		lines = lines[:5]
	}
	textHeight := 7 * panelScale
	if len(lines) > 1 {
		textHeight += (len(lines) - 1) * panelLineSpacing
	}
	startY := panel.Min.Y + (panel.Dy()-textHeight)/2
	for i, line := range lines {
		textWidth := (utf8.RuneCountInString(line)-1)*6*panelScale + 5*panelScale
		x := panel.Min.X + (panel.Dx()-textWidth)/2
		drawPanelText(img, x, startY+i*panelLineSpacing, line, panelScale)
	}

	name := "contact-panel.png"
	path := filepath.Join(s.cfg.ScreensDir, name)
	file, err := os.Create(path)
	if err != nil {
		httpError(w, http.StatusInternalServerError, fmt.Sprintf("create panel: %v", err))
		return
	}
	encodeErr := png.Encode(file, img)
	closeErr := file.Close()
	if encodeErr != nil || closeErr != nil {
		if encodeErr != nil {
			httpError(w, http.StatusInternalServerError, fmt.Sprintf("encode panel: %v", encodeErr))
		} else {
			httpError(w, http.StatusInternalServerError, fmt.Sprintf("save panel: %v", closeErr))
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"name": name})
}

func drawPNGBackground(dst *image.RGBA, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	src, err := png.Decode(file)
	if err != nil {
		return err
	}
	draw.Draw(dst, dst.Bounds(), src, src.Bounds().Min, draw.Src)
	return nil
}

func drawPanelBorder(img *image.RGBA, r image.Rectangle) {
	black := &image.Uniform{C: color.Black}
	draw.Draw(img, image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+2), black, image.Point{}, draw.Src)
	draw.Draw(img, image.Rect(r.Min.X, r.Max.Y-2, r.Max.X, r.Max.Y), black, image.Point{}, draw.Src)
	draw.Draw(img, image.Rect(r.Min.X, r.Min.Y, r.Min.X+2, r.Max.Y), black, image.Point{}, draw.Src)
	draw.Draw(img, image.Rect(r.Max.X-2, r.Min.Y, r.Max.X, r.Max.Y), black, image.Point{}, draw.Src)
}

func wrapPanelText(value string, width int) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	var result []string
	for _, paragraph := range strings.Split(value, "\n") {
		words := strings.Fields(paragraph)
		line := ""
		for _, word := range words {
			if line != "" && utf8.RuneCountInString(line)+1+utf8.RuneCountInString(word) > width {
				result = append(result, line)
				line = ""
			}
			for utf8.RuneCountInString(word) > width {
				runes := []rune(word)
				result = append(result, string(runes[:width]))
				word = string(runes[width:])
			}
			if line != "" {
				line += " "
			}
			line += word
		}
		if line != "" {
			result = append(result, line)
		}
	}
	return result
}

func normalizePanelText(value string) string {
	value = strings.NewReplacer(
		"ß", "ss", "ẞ", "SS",
		"é", "e", "è", "e", "ê", "e", "ë", "e",
		"à", "a", "â", "a", "ä", "a",
		"î", "i", "ï", "i", "ô", "o", "ö", "o",
		"ù", "u", "û", "u", "ü", "u", "ç", "c",
		"É", "E", "È", "E", "Ê", "E", "Ë", "E",
		"À", "A", "Â", "A", "Ä", "A", "Î", "I",
		"Ï", "I", "Ô", "O", "Ö", "O", "Ù", "U",
		"Û", "U", "Ü", "U", "Ç", "C",
	).Replace(value)
	var out strings.Builder
	for _, r := range strings.ToUpper(value) {
		if r <= unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune(" @._-+/(),:?!", r)) {
			out.WriteRune(r)
		} else {
			out.WriteRune('?')
		}
	}
	return out.String()
}

var panelGlyphs = map[rune]string{
	' ': "0000000000000000000000000000000000000",
	'A': "01110100011000111111100011000110001",
	'B': "11110100011000111110100011000111110",
	'C': "01111100001000010000100001000011111",
	'D': "11110100011000110001100011000111110",
	'E': "11111100001000011110100001000011111",
	'F': "11111100001000011110100001000010000",
	'G': "01111100001000010111100011000101111",
	'H': "10001100011000111111100011000110001",
	'I': "11111001000010000100001000010011111",
	'J': "00111000100001000010000101001001100",
	'K': "10001100101010011000101001001010001",
	'L': "10000100001000010000100001000011111",
	'M': "10001110111010110101100011000110001",
	'N': "10001110011010110011100011000110001",
	'O': "01110100011000110001100011000101110",
	'P': "11110100011000111110100001000010000",
	'Q': "01110100011000110001101011001001101",
	'R': "11110100011000111110101001001010001",
	'S': "01111100001000001110000011000111110",
	'T': "11111001000010000100001000010000100",
	'U': "10001100011000110001100011000101110",
	'V': "10001100011000110001100011000101010",
	'W': "10001100011000110101101011101110001",
	'X': "10001100010101000100010101000110001",
	'Y': "10001100010101000100001000010000100",
	'Z': "11111000010001000100010001000011111",
	'0': "01110100011001110101110011000101110",
	'1': "00100011000010000100001000010011111",
	'2': "01110100010000100010001000100011111",
	'3': "11111000010000100110000011000101110",
	'4': "00010001100101010010111110001000010",
	'5': "11111100001000011110000011000111110",
	'6': "01110100001000011110100011000101110",
	'7': "11111000010001000100010000100001000",
	'8': "01110100011000101110100011000101110",
	'9': "01110100011000101111000011000101110",
	'.': "00000000000000000000000000000110000",
	'-': "00000000000000000000011111000000000",
	':': "00000011000000000000011000000000000",
	'!': "00100001000010000100000000010000100",
	'?': "01110100010000100010001000000000100",
	'@': "01110100011101110101110111000101110",
	'/': "00001000100010001000010001000100000",
	'+': "00000001000010011111001000010000000",
	'(': "00010001000100001000010000100000100",
	')': "00100000100001000010000100010001000",
	',': "00000000000000000000000000010001000",
	'_': "00000000000000000000000000000111111",
}

func drawPanelText(img *image.RGBA, x, y int, text string, scale int) {
	black := &image.Uniform{C: color.Black}
	for _, r := range text {
		glyph, ok := panelGlyphs[r]
		if !ok {
			glyph = panelGlyphs['?']
		}
		for i, pixel := range glyph {
			if pixel != '1' {
				continue
			}
			col, row := i%5, i/5
			rect := image.Rect(x+col*scale, y+row*scale, x+(col+1)*scale, y+(row+1)*scale)
			draw.Draw(img, rect, black, image.Point{}, draw.Src)
		}
		x += 6 * scale
	}
}
