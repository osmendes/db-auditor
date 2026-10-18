package report

import (
	"bytes"
	"compress/zlib"
	"crypto/sha256"
	_ "embed"
	"encoding/binary"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
)

// liberationSans is the unmodified Liberation Sans Regular 2.1.5 (SIL OFL
// 1.1, see fonts/). It is metric-compatible with Helvetica, so embedding it
// keeps the report layout of the base-14 font while making the PDF
// self-contained (#107).
//
//go:embed fonts/LiberationSans-Regular.ttf
var liberationSans []byte

// pdfFont is the embedded font program and the metrics the PDF font
// dictionaries need, built once per process.
type pdfFont struct {
	name      string // subset tag + PostScript name
	program   []byte // zlib-compressed TrueType subset (FontFile2)
	length1   int    // uncompressed length
	widths    [224]int
	bbox      [4]int
	ascent    int
	descent   int
	capHeight int
}

var embeddedFont = sync.OnceValues(func() (*pdfFont, error) { return buildPDFFont(liberationSans, "LiberationSans") })

// winAnsiUnicode maps each WinAnsiEncoding code 32-255 to its Unicode code
// point; 0 marks an undefined code.
func winAnsiUnicode(code rune) rune {
	switch {
	case code >= 0x20 && code <= 0x7e, code >= 0xa0 && code <= 0xff:
		return code
	case code >= 0x80 && code <= 0x9f:
		return [32]rune{'€', 0, '‚', 'ƒ', '„', '…', '†', '‡', 'ˆ', '‰', 'Š', '‹', 'Œ', 0, 'Ž', 0, 0, '‘', '’', '“', '”', '•', '–', '—', '˜', '™', 'š', '›', 'œ', 0, 'ž', 'Ÿ'}[code-0x80]
	}
	return 0
}

type ttfTable struct{ offset, length uint32 }

type ttf struct {
	data   []byte
	tables map[string]ttfTable
}

func (f ttf) table(tag string) ([]byte, error) {
	t, ok := f.tables[tag]
	if !ok || uint64(t.offset)+uint64(t.length) > uint64(len(f.data)) {
		return nil, fmt.Errorf("fonte sem tabela %q", tag)
	}
	return f.data[t.offset : t.offset+t.length], nil
}

func parseTTF(data []byte) (ttf, error) {
	if len(data) < 12 || binary.BigEndian.Uint32(data) != 0x00010000 {
		return ttf{}, fmt.Errorf("fonte TrueType inválida")
	}
	n := int(binary.BigEndian.Uint16(data[4:]))
	if len(data) < 12+16*n {
		return ttf{}, fmt.Errorf("fonte TrueType truncada")
	}
	f := ttf{data: data, tables: map[string]ttfTable{}}
	for i := 0; i < n; i++ {
		rec := data[12+16*i:]
		f.tables[string(rec[:4])] = ttfTable{offset: binary.BigEndian.Uint32(rec[8:]), length: binary.BigEndian.Uint32(rec[12:])}
	}
	return f, nil
}

// cmapFormat4 returns the Windows Unicode BMP (3,1) subtable.
func cmapFormat4(cmap []byte) ([]byte, error) {
	if len(cmap) < 4 {
		return nil, fmt.Errorf("cmap inválido")
	}
	for i := 0; i < int(binary.BigEndian.Uint16(cmap[2:])); i++ {
		rec := cmap[4+8*i:]
		if len(rec) < 8 {
			break
		}
		if binary.BigEndian.Uint16(rec) == 3 && binary.BigEndian.Uint16(rec[2:]) == 1 {
			off := binary.BigEndian.Uint32(rec[4:])
			if int(off)+4 > len(cmap) || binary.BigEndian.Uint16(cmap[off:]) != 4 {
				return nil, fmt.Errorf("cmap (3,1) sem formato 4")
			}
			length := int(binary.BigEndian.Uint16(cmap[off+2:]))
			if int(off)+length > len(cmap) {
				return nil, fmt.Errorf("cmap truncado")
			}
			return cmap[off : int(off)+length], nil
		}
	}
	return nil, fmt.Errorf("fonte sem cmap Unicode (3,1)")
}

func glyphIndex(sub []byte, r rune) int {
	if r < 0 || r > 0xffff || len(sub) < 14 {
		return 0
	}
	segX2 := int(binary.BigEndian.Uint16(sub[6:]))
	if len(sub) < 16+4*segX2 {
		return 0
	}
	ends, starts, deltas, ranges := 14, 16+segX2, 16+2*segX2, 16+3*segX2
	c := int(r)
	for i := 0; i < segX2; i += 2 {
		if c > int(binary.BigEndian.Uint16(sub[ends+i:])) {
			continue
		}
		start := int(binary.BigEndian.Uint16(sub[starts+i:]))
		if c < start {
			return 0
		}
		delta := int(binary.BigEndian.Uint16(sub[deltas+i:]))
		rangeOffset := int(binary.BigEndian.Uint16(sub[ranges+i:]))
		if rangeOffset == 0 {
			return (c + delta) & 0xffff
		}
		at := ranges + i + rangeOffset + 2*(c-start)
		if at+2 > len(sub) {
			return 0
		}
		g := int(binary.BigEndian.Uint16(sub[at:]))
		if g == 0 {
			return 0
		}
		return (g + delta) & 0xffff
	}
	return 0
}

// int16At reads a big-endian signed 16-bit value.
func int16At(b []byte) int {
	v := int(binary.BigEndian.Uint16(b))
	if v >= 0x8000 {
		v -= 0x10000
	}
	return v
}

// maxFontProgram bounds the font file, so every table offset, length and
// count written back fits the TrueType integer fields.
const maxFontProgram = 1 << 24

func u32(v int) uint32 {
	if v < 0 || v > maxFontProgram {
		panic("font program size out of range")
	}
	return uint32(v) //nolint:gosec // G115: bounded by maxFontProgram above.
}

func u16(v int) uint16 {
	if v < 0 || v > math.MaxUint16 {
		panic("font table count out of range")
	}
	return uint16(v) //nolint:gosec // G115: bounded by MaxUint16 above.
}

func ttfChecksum(b []byte) uint32 {
	var sum uint32
	for i := 0; i < len(b); i += 4 {
		var word [4]byte
		copy(word[:], b[i:])
		sum += binary.BigEndian.Uint32(word[:])
	}
	return sum
}

func pad4(b []byte) []byte {
	for len(b)%4 != 0 {
		b = append(b, 0)
	}
	return b
}

// buildPDFFont subsets the font to the WinAnsi glyphs (unused glyph outlines
// are emptied, so glyph ids and hmtx stay valid) and keeps only the tables a
// PDF TrueType font program needs.
func buildPDFFont(data []byte, postScriptName string) (*pdfFont, error) {
	if len(data) > maxFontProgram {
		return nil, fmt.Errorf("fonte grande demais")
	}
	font, err := parseTTF(data)
	if err != nil {
		return nil, err
	}
	head, err := font.table("head")
	if err != nil {
		return nil, err
	}
	maxp, err := font.table("maxp")
	if err != nil {
		return nil, err
	}
	hhea, err := font.table("hhea")
	if err != nil {
		return nil, err
	}
	hmtx, err := font.table("hmtx")
	if err != nil {
		return nil, err
	}
	loca, err := font.table("loca")
	if err != nil {
		return nil, err
	}
	glyf, err := font.table("glyf")
	if err != nil {
		return nil, err
	}
	cmap, err := font.table("cmap")
	if err != nil {
		return nil, err
	}
	os2, err := font.table("OS/2")
	if err != nil {
		return nil, err
	}
	if len(head) < 54 || len(maxp) < 6 || len(hhea) < 36 || len(os2) < 90 {
		return nil, fmt.Errorf("tabelas da fonte truncadas")
	}
	unitsPerEm := int(binary.BigEndian.Uint16(head[18:]))
	longLoca := binary.BigEndian.Uint16(head[50:]) == 1
	numGlyphs := int(binary.BigEndian.Uint16(maxp[4:]))
	numHMetrics := int(binary.BigEndian.Uint16(hhea[34:]))
	if unitsPerEm == 0 || numHMetrics == 0 || len(hmtx) < 4*numHMetrics {
		return nil, fmt.Errorf("métricas da fonte inválidas")
	}
	glyphRange := func(g int) (int, int) {
		if longLoca {
			return int(binary.BigEndian.Uint32(loca[4*g:])), int(binary.BigEndian.Uint32(loca[4*g+4:]))
		}
		return 2 * int(binary.BigEndian.Uint16(loca[2*g:])), 2 * int(binary.BigEndian.Uint16(loca[2*g+2:]))
	}
	if (longLoca && len(loca) < 4*(numGlyphs+1)) || (!longLoca && len(loca) < 2*(numGlyphs+1)) {
		return nil, fmt.Errorf("loca truncada")
	}
	advance := func(g int) int {
		if g >= numHMetrics {
			g = numHMetrics - 1
		}
		return int(binary.BigEndian.Uint16(hmtx[4*g:]))
	}
	scale := func(v int) int { return int(math.Round(float64(v) * 1000 / float64(unitsPerEm))) }
	sub, err := cmapFormat4(cmap)
	if err != nil {
		return nil, err
	}
	out := &pdfFont{}
	keep := map[int]bool{0: true}
	for code := rune(32); code <= 255; code++ {
		g := 0
		if r := winAnsiUnicode(code); r != 0 {
			g = glyphIndex(sub, r)
		}
		if g >= numGlyphs {
			g = 0
		}
		keep[g] = true
		out.widths[int(code)-32] = scale(advance(g))
	}
	// Composite glyphs (accented letters) reference their components.
	queue := make([]int, 0, len(keep))
	for g := range keep {
		queue = append(queue, g)
	}
	for len(queue) > 0 {
		g := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		start, end := glyphRange(g)
		if end-start < 10 || end > len(glyf) || int16At(glyf[start:]) >= 0 {
			continue
		}
		for at := start + 10; at+4 <= end; {
			flags := binary.BigEndian.Uint16(glyf[at:])
			component := int(binary.BigEndian.Uint16(glyf[at+2:]))
			if component < numGlyphs && !keep[component] {
				keep[component] = true
				queue = append(queue, component)
			}
			at += 4
			if flags&0x1 != 0 {
				at += 4
			} else {
				at += 2
			}
			switch {
			case flags&0x8 != 0:
				at += 2
			case flags&0x40 != 0:
				at += 4
			case flags&0x80 != 0:
				at += 8
			}
			if flags&0x20 == 0 {
				break
			}
		}
	}
	var newGlyf []byte
	newLoca := make([]byte, 0, 4*(numGlyphs+1))
	for g := 0; g < numGlyphs; g++ {
		newLoca = binary.BigEndian.AppendUint32(newLoca, u32(len(newGlyf)))
		if keep[g] {
			start, end := glyphRange(g)
			if start <= end && end <= len(glyf) {
				newGlyf = pad4(append(newGlyf, glyf[start:end]...))
			}
		}
	}
	newLoca = binary.BigEndian.AppendUint32(newLoca, u32(len(newGlyf)))
	newHead := append([]byte(nil), head...)
	binary.BigEndian.PutUint32(newHead[8:], 0)
	binary.BigEndian.PutUint16(newHead[50:], 1)
	newCmap := []byte{0, 0, 0, 1, 0, 3, 0, 1, 0, 0, 0, 12}
	newCmap = append(newCmap, sub...)
	tables := map[string][]byte{"OS/2": os2, "cmap": newCmap, "glyf": newGlyf, "head": newHead, "hhea": hhea, "hmtx": hmtx, "loca": newLoca, "maxp": maxp}
	for _, tag := range []string{"cvt ", "fpgm", "prep"} {
		if t, err := font.table(tag); err == nil {
			tables[tag] = t
		}
	}
	if post, err := font.table("post"); err == nil && len(post) >= 32 {
		post3 := append([]byte(nil), post[:32]...)
		binary.BigEndian.PutUint32(post3, 0x00030000)
		tables["post"] = post3
	}
	tags := make([]string, 0, len(tables))
	for tag := range tables {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	n := len(tags)
	searchRange, selector := 1, 0
	for searchRange*2 <= n {
		searchRange *= 2
		selector++
	}
	var file bytes.Buffer
	header := make([]byte, 12, 12+16*n)
	binary.BigEndian.PutUint32(header, 0x00010000)
	binary.BigEndian.PutUint16(header[4:], u16(n))
	binary.BigEndian.PutUint16(header[6:], u16(searchRange*16))
	binary.BigEndian.PutUint16(header[8:], u16(selector))
	binary.BigEndian.PutUint16(header[10:], u16(n*16-searchRange*16))
	offset := 12 + 16*n
	var body []byte
	headAt := 0
	for _, tag := range tags {
		t := tables[tag]
		if tag == "head" {
			headAt = offset + len(body)
		}
		header = append(header, tag...)
		header = binary.BigEndian.AppendUint32(header, ttfChecksum(t))
		header = binary.BigEndian.AppendUint32(header, u32(offset+len(body)))
		header = binary.BigEndian.AppendUint32(header, u32(len(t)))
		body = pad4(append(body, t...))
	}
	file.Write(header)
	file.Write(body)
	program := file.Bytes()
	binary.BigEndian.PutUint32(program[headAt+8:], 0xB1B0AFBA-ttfChecksum(program))

	var compressed bytes.Buffer
	zw, err := zlib.NewWriterLevel(&compressed, zlib.BestCompression)
	if err != nil {
		return nil, err
	}
	if _, err := zw.Write(program); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	digest := sha256.Sum256(program)
	var tag strings.Builder
	for _, b := range digest[:6] {
		tag.WriteByte('A' + b%26)
	}
	out.name = tag.String() + "+" + postScriptName
	out.program = compressed.Bytes()
	out.length1 = len(program)
	for i := range out.bbox {
		out.bbox[i] = scale(int16At(head[36+2*i:]))
	}
	out.ascent = scale(int16At(hhea[4:]))
	out.descent = scale(int16At(hhea[6:]))
	out.capHeight = scale(int16At(os2[88:]))
	return out, nil
}

// fontObjects returns the bodies of the font, descriptor and font file
// objects, given their object ids.
func (f *pdfFont) fontObjects(descriptorID, fileID int) (font, descriptor, file string) {
	widths := make([]string, len(f.widths))
	for i, w := range f.widths {
		widths[i] = fmt.Sprint(w)
	}
	font = fmt.Sprintf("<< /Type /Font /Subtype /TrueType /BaseFont /%s /FirstChar 32 /LastChar 255 /Widths [%s] /Encoding /WinAnsiEncoding /FontDescriptor %d 0 R >>", f.name, strings.Join(widths, " "), descriptorID)
	descriptor = fmt.Sprintf("<< /Type /FontDescriptor /FontName /%s /Flags 32 /FontBBox [%d %d %d %d] /ItalicAngle 0 /Ascent %d /Descent %d /CapHeight %d /StemV 80 /FontFile2 %d 0 R >>", f.name, f.bbox[0], f.bbox[1], f.bbox[2], f.bbox[3], f.ascent, f.descent, f.capHeight, fileID)
	file = fmt.Sprintf("<< /Length %d /Length1 %d /Filter /FlateDecode >>\nstream\n%s\nendstream", len(f.program), f.length1, f.program)
	return font, descriptor, file
}
