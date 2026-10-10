package report

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

const maxPDFBytes = 16 << 20
const maxPDFPages = 200
const maxReportTextBytes = 8 << 20

// RenderPDF writes a deterministic, paginated PDF 1.4 document with no
// external font or native runtime dependency. All text is WinAnsi encoded.
func RenderPDF(document Document) ([]byte, error) {
	lines := BuildLines(document)
	textBytes := 0
	for _, line := range lines {
		textBytes += len(line.Text)
		if textBytes > maxReportTextBytes {
			return nil, fmt.Errorf("relatório excede o limite de texto (%d bytes); reduza o escopo ou use filtros", maxReportTextBytes)
		}
	}
	pages := [][]string{{}}
	y := 790
	for _, line := range lines {
		if line.Style == "heading" && y < 110 {
			if len(pages) >= maxPDFPages {
				return nil, fmt.Errorf("relatório excede %d páginas; reduza o escopo ou use filtros", maxPDFPages)
			}
			pages = append(pages, []string{})
			y = 790
		}
		if line.Style == "bar_bytes" {
			if y < 75 {
				if len(pages) >= maxPDFPages {
					return nil, fmt.Errorf("relatório excede %d páginas; reduza o escopo ou use filtros", maxPDFPages)
				}
				pages = append(pages, []string{})
				y = 790
			}
			parts := strings.Split(line.Text, "|")
			if len(parts) == 3 {
				value, valueErr := strconv.ParseInt(parts[1], 10, 64)
				maximum, maxErr := strconv.ParseInt(parts[2], 10, 64)
				if valueErr == nil && maxErr == nil && value >= 0 && maximum >= 0 {
					width := 0
					if maximum > 0 {
						width = int(300 * float64(value) / float64(maximum))
					}
					command := fmt.Sprintf("0.94 0.96 0.97 rg 42 %d 68 11 re f 0.90 0.93 0.94 rg 165 %d 250 11 re f 0.09 0.55 0.42 rg 165 %d %d 11 re f 0.12 0.18 0.24 rg BT /F1 8 Tf 43 %d Td (%s) Tj ET BT /F1 9 Tf 425 %d Td (%d B) Tj ET\n", y-2, y-2, y-2, width*250/300, y, escapePDFText(parts[0]), y, value)
					pages[len(pages)-1] = append(pages[len(pages)-1], command)
					y -= 20
					continue
				}
			}
		}
		if line.Style == "bar" {
			if y < 75 {
				if len(pages) >= maxPDFPages {
					return nil, fmt.Errorf("relatório excede %d páginas; reduza o escopo ou use filtros", maxPDFPages)
				}
				pages = append(pages, []string{})
				y = 790
			}
			parts := strings.Split(line.Text, "|")
			if len(parts) == 3 {
				value, valueErr := strconv.Atoi(parts[1])
				maximum, maxErr := strconv.Atoi(parts[2])
				if valueErr == nil && maxErr == nil && value >= 0 && maximum >= 0 {
					width := 0
					if maximum > 0 {
						width = 300 * value / maximum
					}
					command := fmt.Sprintf("0.94 0.96 0.97 rg 42 %d 68 11 re f 0.90 0.93 0.94 rg 115 %d 300 11 re f 0.09 0.55 0.42 rg 115 %d %d 11 re f 0.12 0.18 0.24 rg BT /F1 9 Tf 43 %d Td (%s) Tj ET BT /F1 9 Tf 425 %d Td (%d) Tj ET\n", y-2, y-2, y-2, width, y, escapePDFText(parts[0]), y, value)
					pages[len(pages)-1] = append(pages[len(pages)-1], command)
					y -= 20
					continue
				}
			}
		}
		fontSize, step := 10, 14
		if line.Style == "title" {
			fontSize, step = 18, 26
		}
		if line.Style == "heading" {
			fontSize, step = 13, 21
		}
		if line.Style == "subheading" {
			fontSize, step = 10, 17
		}
		if y < 780 {
			switch line.Style {
			case "heading":
				y -= 10
			case "subheading":
				y -= 4
			}
		}
		width := 90
		if fontSize == 18 {
			width = 54
		}
		if fontSize == 13 {
			width = 75
		}
		for _, part := range wrapText(line.Text, width) {
			if y < 55 {
				if len(pages) >= maxPDFPages {
					return nil, fmt.Errorf("relatório excede %d páginas; reduza o escopo ou use filtros", maxPDFPages)
				}
				pages = append(pages, []string{})
				y = 790
			}
			color := "0.15 0.20 0.26"
			if line.Style == "title" || line.Style == "heading" {
				color = "0.08 0.45 0.35"
			}
			command := fmt.Sprintf("%s rg BT /F1 %d Tf 42 %d Td (%s) Tj ET\n", color, fontSize, y, escapePDFText(part))
			pages[len(pages)-1] = append(pages[len(pages)-1], command)
			y -= step
			fontSize, step = 10, 14
		}
	}
	lines = nil
	var out bytes.Buffer
	out.WriteString("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")
	offsets := []int{0}
	writeObject := func(body string) error {
		offsets = append(offsets, out.Len())
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", len(offsets)-1, body)
		if out.Len() > maxPDFBytes {
			return fmt.Errorf("relatório excede %d bytes; reduza o escopo ou use filtros", maxPDFBytes)
		}
		return nil
	}
	pageCount := len(pages)
	contentID := func(index int) int { return index + 1 }
	pageID := func(index int) int { return pageCount + index + 1 }
	fontID := 2*pageCount + 1
	pagesID := 2*pageCount + 2
	catalogID := 2*pageCount + 3
	for index := range pages {
		commands := pages[index]
		pages[index] = nil
		stream := "1 1 1 rg 0 0 595 842 re f\n"
		if index > 0 {
			run := document.RunID
			if len(run) > 8 {
				run = run[:8]
			}
			stream += fmt.Sprintf("0.45 0.50 0.55 rg BT /F1 9 Tf 42 815 Td (%s) Tj ET\n", escapePDFText("DB Auditor · "+reportTypeLabel(document.Type)+" · execução "+run))
		}
		stream += strings.Join(commands, "") + fmt.Sprintf("0.45 0.50 0.55 rg BT /F1 9 Tf 42 30 Td (%s) Tj ET\n", escapePDFText(fmt.Sprintf("Página %d de %d", index+1, pageCount)))
		if err := writeObject(fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(stream), stream)); err != nil {
			return nil, err
		}
	}
	pages = nil
	kids := make([]string, pageCount)
	for index := 0; index < pageCount; index++ {
		kids[index] = fmt.Sprintf("%d 0 R", pageID(index))
		body := fmt.Sprintf("<< /Type /Page /Parent %d 0 R /MediaBox [0 0 595 842] /Resources << /Font << /F1 %d 0 R >> >> /Contents %d 0 R >>", pagesID, fontID, contentID(index))
		if err := writeObject(body); err != nil {
			return nil, err
		}
	}
	if err := writeObject("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>"); err != nil {
		return nil, err
	}
	if err := writeObject(fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), pageCount)); err != nil {
		return nil, err
	}
	if err := writeObject(fmt.Sprintf("<< /Type /Catalog /Pages %d 0 R >>", pagesID)); err != nil {
		return nil, err
	}
	if catalogID != len(offsets)-1 {
		return nil, fmt.Errorf("relatório PDF inconsistente")
	}
	xref := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(offsets))
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&out, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root %d 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets), catalogID, xref)
	if out.Len() > maxPDFBytes {
		return nil, fmt.Errorf("relatório excede %d bytes; reduza o escopo ou use filtros", maxPDFBytes)
	}
	return out.Bytes(), nil
}

func wrapText(value string, width int) []string {
	value = strings.Join(strings.Fields(value), " ")
	if value == "" {
		return []string{""}
	}
	words := strings.Fields(value)
	out := []string{}
	line := ""
	for _, word := range words {
		for len([]rune(word)) > width {
			if line != "" {
				out = append(out, line)
				line = ""
			}
			runes := []rune(word)
			out = append(out, string(runes[:width]))
			word = string(runes[width:])
		}
		if len([]rune(line))+len([]rune(word))+1 > width {
			out = append(out, line)
			line = ""
		}
		if line != "" {
			line += " "
		}
		line += word
	}
	if line != "" {
		out = append(out, line)
	}
	return out
}

func escapePDFText(value string) string {
	var out strings.Builder
	for _, r := range value {
		b := byte('?')
		if r >= 32 && r <= 255 {
			b = byte(r)
		}
		switch b {
		case '\\', '(', ')':
			out.WriteByte('\\')
			out.WriteByte(b)
		case '\n', '\r':
			out.WriteByte(' ')
		default:
			out.WriteByte(b)
		}
	}
	return out.String()
}

// PageCount is intentionally small and useful to tests without a PDF parser.
func PageCount(pdf []byte) int { return strings.Count(string(pdf), "/Type /Page /Parent") }

func ValidatePDF(pdf []byte) error {
	if !bytes.HasPrefix(pdf, []byte("%PDF-1.4")) || !bytes.HasSuffix(pdf, []byte("%%EOF\n")) {
		return fmt.Errorf("invalid PDF envelope")
	}
	if PageCount(pdf) < 1 {
		return fmt.Errorf("PDF has no pages")
	}
	_, err := strconv.Atoi(strings.TrimSpace(string(pdf[bytes.LastIndex(pdf, []byte("startxref\n"))+10 : bytes.LastIndex(pdf, []byte("\n%%EOF"))])))
	return err
}
