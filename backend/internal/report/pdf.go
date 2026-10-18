package report

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf16"
)

const maxPDFBytes = 16 << 20
const maxPDFPages = 200
const maxReportTextBytes = 8 << 20

// PDFResult describes a PDF written by WritePDF.
type PDFResult struct {
	Bytes  int64
	Pages  int
	SHA256 string
	// LimitNote is Document.LimitNote of the rendered (redacted) document,
	// computed without holding the whole document (AN-12).
	LimitNote string
}

// RenderPDF writes a deterministic, paginated, tagged PDF 1.4 document with
// an embedded font (#107) and no native runtime dependency. All text is
// WinAnsi encoded.
// It buffers the whole file; the report worker streams with WritePDF instead.
func RenderPDF(document Document) ([]byte, error) {
	var out bytes.Buffer
	if _, err := WritePDF(context.Background(), &out, document); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// WritePDF streams the same bytes as RenderPDF to w for an in-memory
// document. See WritePDFSections.
func WritePDF(ctx context.Context, w io.Writer, document Document) (PDFResult, error) {
	return WritePDFSections(ctx, w, MemorySource(document), nil)
}

// WritePDFSections streams a PDF to w one page at a time (PF-07) and loads the
// report data one section at a time (#65): overview, findings, databases and
// tables are each loaded, laid out and released before the next one, so only
// one section is in memory at once. The footer and the object ids need the
// page total, so the sections are walked three times: once for the summary
// counts, once to count pages and once to write. The source must return the
// same data on every call (the store reads from one repeatable-read
// snapshot). ctx is checked between pages and sections so the worker can stop
// a job that exceeds its memory budget with a readable cause. When redaction
// is set it is applied per section, exactly like Document.RedactMetadata
// followed by Document.RedactSensitive on the whole document.
func WritePDFSections(ctx context.Context, w io.Writer, source SectionSource, redaction *Redaction) (PDFResult, error) {
	plan, err := planSections(ctx, source, redaction)
	if err != nil {
		return PDFResult{}, err
	}
	document := plan.overview
	textBytes := 0
	var layoutErr error
	counter := newPDFLayout(nil, nil)
	err = plan.emitLines(ctx, func(line Line) error {
		textBytes += len(line.Text)
		if textBytes > maxReportTextBytes {
			return fmt.Errorf("relatório excede o limite de texto (%d bytes); reduza o escopo ou use filtros", maxReportTextBytes)
		}
		// The text ceiling is reported before the page ceiling, as when all
		// lines were measured up front, so keep counting text after a page error.
		if layoutErr == nil {
			layoutErr = counter.place(line)
		}
		return nil
	})
	if err != nil {
		return PDFResult{}, err
	}
	if layoutErr != nil {
		return PDFResult{}, layoutErr
	}
	pageCount := counter.pages
	font, err := embeddedFont()
	if err != nil {
		return PDFResult{}, err
	}
	digest := sha256.New()
	out := &countingWriter{w: io.MultiWriter(w, digest), limit: maxPDFBytes}
	if _, err := io.WriteString(out, pdfHeader); err != nil {
		return PDFResult{}, err
	}
	offsets := []int64{0}
	writeObject := func(body string) error {
		offsets = append(offsets, out.n)
		if _, err := fmt.Fprintf(out, "%d 0 obj\n%s\nendobj\n", len(offsets)-1, body); err != nil {
			return err
		}
		return nil
	}
	header := pageHeader(document)
	var page strings.Builder
	index := 0
	tags := newTagging()
	flush := func() error {
		if err := ctx.Err(); err != nil {
			if cause := context.Cause(ctx); cause != nil {
				return cause
			}
			return err
		}
		stream := pageStream(index, pageCount, header, page.String())
		page.Reset()
		index++
		return writeObject(fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(stream), stream))
	}
	writer := newPDFLayout(flush, func(command string) { page.WriteString(command) })
	writer.tags = tags
	if err := plan.emitLines(ctx, writer.place); err != nil {
		return PDFResult{}, err
	}
	if err := flush(); err != nil {
		return PDFResult{}, err
	}
	if index != pageCount || len(tags.pages) != pageCount {
		return PDFResult{}, fmt.Errorf("relatório PDF inconsistente")
	}
	catalogID, infoID, err := writeDocumentObjects(writeObject, pageCount, document, tags, font)
	if err != nil {
		return PDFResult{}, err
	}
	if catalogID != len(offsets)-1 {
		return PDFResult{}, fmt.Errorf("relatório PDF inconsistente")
	}
	xref := out.n
	if _, err := fmt.Fprintf(out, "xref\n0 %d\n0000000000 65535 f \n", len(offsets)); err != nil {
		return PDFResult{}, err
	}
	for _, offset := range offsets[1:] {
		if _, err := fmt.Fprintf(out, "%010d 00000 n \n", offset); err != nil {
			return PDFResult{}, err
		}
	}
	if _, err := io.WriteString(out, pdfTrailer(len(offsets), catalogID, infoID, xref)); err != nil {
		return PDFResult{}, err
	}
	return PDFResult{Bytes: out.n, Pages: pageCount, SHA256: hex.EncodeToString(digest.Sum(nil)), LimitNote: limitNote(plan.overview, plan.findings)}, nil
}

const pdfHeader = "%PDF-1.4\n%\xe2\xe3\xcf\xd3\n"

// pageHeader is the running header of every page after the first.
func pageHeader(document Document) string {
	run := document.RunID
	if len(run) > 8 {
		run = run[:8]
	}
	return fmt.Sprintf("/Artifact BMC 0.45 0.50 0.55 rg BT /F1 9 Tf 42 815 Td (%s) Tj ET EMC\n", escapePDFText("DB Auditor · "+reportTypeLabel(document.Type)+" · execução "+run))
}

// pageStream wraps a page's tagged content with the background, the running
// header and the footer, all marked as artifacts so screen readers skip them.
func pageStream(index, pageCount int, header, content string) string {
	stream := "/Artifact BMC 1 1 1 rg 0 0 595 842 re f EMC\n"
	if index > 0 {
		stream += header
	}
	return stream + content + fmt.Sprintf("/Artifact BMC 0.45 0.50 0.55 rg BT /F1 9 Tf 42 30 Td (%s) Tj ET EMC\n", escapePDFText(fmt.Sprintf("Página %d de %d", index+1, pageCount)))
}

// PDFTitle is the document title stored in the Info dictionary and the XMP
// metadata, shown by viewers instead of the file name (#107).
func PDFTitle(document Document) string {
	run := document.RunID
	if len(run) > 8 {
		run = run[:8]
	}
	parts := []string{"DB Auditor", "Relatório " + reportTypeLabel(document.Type)}
	if env := strings.TrimSpace(document.Environment); env != "" {
		parts = append(parts, env)
	}
	if run != "" {
		parts = append(parts, "execução "+run)
	}
	return strings.Join(parts, " - ")
}

// markedRef is one marked-content sequence: an MCID on a page.
type markedRef struct{ page, mcid int }

// structElem is one report line in the structure tree; a line wrapped across
// a page break has a marked-content sequence on each page.
type structElem struct {
	role string
	refs []markedRef
}

// tagging collects the structure tree while pages are laid out (#107). It
// keeps a few integers per drawn line, never the text.
type tagging struct {
	elems []structElem
	pages [][]int // per page, the element of each MCID
}

func newTagging() *tagging { return &tagging{pages: [][]int{nil}} }

func (t *tagging) newPage() { t.pages = append(t.pages, nil) }

func (t *tagging) open(role string) int {
	t.elems = append(t.elems, structElem{role: role})
	return len(t.elems) - 1
}

func (t *tagging) mark(elem int) int {
	page := len(t.pages) - 1
	mcid := len(t.pages[page])
	t.pages[page] = append(t.pages[page], elem)
	t.elems[elem].refs = append(t.elems[elem].refs, markedRef{page: page, mcid: mcid})
	return mcid
}

// structRole maps a line style to a standard structure type.
func structRole(style string) string {
	switch style {
	case "title":
		return "H1"
	case "heading":
		return "H2"
	case "subheading":
		return "H3"
	}
	return "P"
}

// writeDocumentObjects writes everything after the content streams: page
// objects, the embedded font, Info and XMP metadata, the structure tree and
// the catalog. Object ids follow the content streams (1..pageCount) and the
// page objects (pageCount+1..2*pageCount).
func writeDocumentObjects(writeObject func(string) error, pageCount int, document Document, tags *tagging, font *pdfFont) (catalogID, infoID int, err error) {
	pageID := func(index int) int { return pageCount + index + 1 }
	fontID := 2*pageCount + 1
	pagesID := 2*pageCount + 2
	descriptorID := 2*pageCount + 3
	fileID := 2*pageCount + 4
	infoID = 2*pageCount + 5
	metadataID := 2*pageCount + 6
	rootID := 2*pageCount + 7
	parentTreeID := 2*pageCount + 8
	docID := 2*pageCount + 9
	elemID := func(index int) int { return docID + 1 + index }
	catalogID = elemID(len(tags.elems))
	kids := make([]string, pageCount)
	for index := 0; index < pageCount; index++ {
		kids[index] = fmt.Sprintf("%d 0 R", pageID(index))
		body := fmt.Sprintf("<< /Type /Page /Parent %d 0 R /MediaBox [0 0 595 842] /Resources << /Font << /F1 %d 0 R >> >> /Contents %d 0 R /StructParents %d >>", pagesID, fontID, index+1, index)
		if err := writeObject(body); err != nil {
			return 0, 0, err
		}
	}
	fontBody, descriptor, file := font.fontObjects(descriptorID, fileID)
	if err := writeObject(fontBody); err != nil {
		return 0, 0, err
	}
	if err := writeObject(fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), pageCount)); err != nil {
		return 0, 0, err
	}
	if err := writeObject(descriptor); err != nil {
		return 0, 0, err
	}
	if err := writeObject(file); err != nil {
		return 0, 0, err
	}
	title := PDFTitle(document)
	if err := writeObject(fmt.Sprintf("<< /Title %s /Creator (DB Auditor) /Producer (DB Auditor) >>", pdfTextString(title))); err != nil {
		return 0, 0, err
	}
	xmp := xmpMetadata(title)
	if err := writeObject(fmt.Sprintf("<< /Type /Metadata /Subtype /XML /Length %d >>\nstream\n%s\nendstream", len(xmp), xmp)); err != nil {
		return 0, 0, err
	}
	if err := writeObject(fmt.Sprintf("<< /Type /StructTreeRoot /K %d 0 R /ParentTree %d 0 R /ParentTreeNextKey %d >>", docID, parentTreeID, pageCount)); err != nil {
		return 0, 0, err
	}
	var nums strings.Builder
	for page, mcids := range tags.pages {
		fmt.Fprintf(&nums, "%d [", page)
		for i, elem := range mcids {
			if i > 0 {
				nums.WriteByte(' ')
			}
			fmt.Fprintf(&nums, "%d 0 R", elemID(elem))
		}
		nums.WriteString("] ")
	}
	if err := writeObject(fmt.Sprintf("<< /Nums [%s] >>", strings.TrimSpace(nums.String()))); err != nil {
		return 0, 0, err
	}
	var children strings.Builder
	for index := range tags.elems {
		if index > 0 {
			children.WriteByte(' ')
		}
		fmt.Fprintf(&children, "%d 0 R", elemID(index))
	}
	if err := writeObject(fmt.Sprintf("<< /Type /StructElem /S /Document /P %d 0 R /K [%s] >>", rootID, children.String())); err != nil {
		return 0, 0, err
	}
	for _, elem := range tags.elems {
		first := elem.refs[0].page
		var k strings.Builder
		for i, ref := range elem.refs {
			if i > 0 {
				k.WriteByte(' ')
			}
			if ref.page == first {
				fmt.Fprintf(&k, "%d", ref.mcid)
			} else {
				fmt.Fprintf(&k, "<< /Type /MCR /Pg %d 0 R /MCID %d >>", pageID(ref.page), ref.mcid)
			}
		}
		if err := writeObject(fmt.Sprintf("<< /Type /StructElem /S /%s /P %d 0 R /Pg %d 0 R /K [%s] >>", elem.role, docID, pageID(first), k.String())); err != nil {
			return 0, 0, err
		}
	}
	if err := writeObject(fmt.Sprintf("<< /Type /Catalog /Pages %d 0 R /Lang (pt-BR) /MarkInfo << /Marked true >> /StructTreeRoot %d 0 R /Metadata %d 0 R /ViewerPreferences << /DisplayDocTitle true >> >>", pagesID, rootID, metadataID)); err != nil {
		return 0, 0, err
	}
	return catalogID, infoID, nil
}

// xmlChar reports whether r is allowed in XML 1.0 text.
func xmlChar(r rune) bool {
	return r == 0x9 || r == 0xA || r == 0xD || (r >= 0x20 && r <= 0xD7FF) || (r >= 0xE000 && r <= 0xFFFD) || (r >= 0x10000 && r <= 0x10FFFF)
}

func pdfTrailer(size, catalogID, infoID int, xref int64) string {
	return fmt.Sprintf("trailer\n<< /Size %d /Root %d 0 R /Info %d 0 R >>\nstartxref\n%d\n%%%%EOF\n", size, catalogID, infoID, xref)
}

// pdfTextString encodes a text string as UTF-16BE with a byte order mark, so
// accents survive in the Info dictionary.
func pdfTextString(value string) string {
	var out strings.Builder
	out.WriteString("<FEFF")
	for _, unit := range utf16.Encode([]rune(value)) {
		fmt.Fprintf(&out, "%04X", unit)
	}
	out.WriteString(">")
	return out.String()
}

// xmpMetadata carries the title and language for tools that read XMP. The
// title is written as XML character references only, so untrusted text
// (an environment name) never appears verbatim inside the stream.
func xmpMetadata(title string) string {
	var escaped strings.Builder
	for _, r := range title {
		if r == '\uFFFD' || !xmlChar(r) {
			r = '?'
		}
		fmt.Fprintf(&escaped, "&#x%X;", r)
	}
	return `<?xpacket begin="` + "\ufeff" + `" id="W5M0MpCehiHzreSzNTczkc9d"?><x:xmpmeta xmlns:x="adobe:ns:meta/"><rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"><rdf:Description rdf:about="" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:pdf="http://ns.adobe.com/pdf/1.3/" xmlns:xmp="http://ns.adobe.com/xap/1.0/"><dc:title><rdf:Alt><rdf:li xml:lang="x-default">` + escaped.String() + `</rdf:li></rdf:Alt></dc:title><dc:language><rdf:Bag><rdf:li>pt-BR</rdf:li></rdf:Bag></dc:language><pdf:Producer>DB Auditor</pdf:Producer><xmp:CreatorTool>DB Auditor</xmp:CreatorTool></rdf:Description></rdf:RDF></x:xmpmeta><?xpacket end="r"?>`
}

// errPDFTooLarge keeps the user-facing reason when the byte ceiling is hit.
var errPDFTooLarge = fmt.Errorf("relatório excede %d bytes; reduza o escopo ou use filtros", maxPDFBytes)

// countingWriter tracks the offset for the xref table and enforces the
// artifact ceiling before the bytes reach the destination.
type countingWriter struct {
	w     io.Writer
	n     int64
	limit int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	if c.n+int64(len(p)) > c.limit {
		return 0, errPDFTooLarge
	}
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

// pdfLayout places report lines on pages one at a time, so sections can be
// laid out as they are loaded. newPage is called before a page break (to
// flush the finished page) and emit receives each drawing command; both may
// be nil to only count pages.
type pdfLayout struct {
	pages   int
	y       int
	newPage func() error
	emit    func(string)
	// tags, when set, receives the structure of every drawn line (#107).
	tags *tagging
}

func newPDFLayout(newPage func() error, emit func(string)) *pdfLayout {
	return &pdfLayout{pages: 1, y: 790, newPage: newPage, emit: emit}
}

func (l *pdfLayout) breakPage() error {
	if l.pages >= maxPDFPages {
		return fmt.Errorf("relatório excede %d páginas; reduza o escopo ou use filtros", maxPDFPages)
	}
	if l.newPage != nil {
		if err := l.newPage(); err != nil {
			return err
		}
	}
	l.pages++
	if l.tags != nil {
		l.tags.newPage()
	}
	return nil
}

// markText returns the opening operator of a tagged text run. elem is the
// line's structure element, opened on its first drawn part.
func (l *pdfLayout) markText(elem *int, role string) string {
	if l.tags == nil {
		return "/" + role + " BMC "
	}
	if *elem < 0 {
		*elem = l.tags.open(role)
	}
	return fmt.Sprintf("/%s << /MCID %d >> BDC ", role, l.tags.mark(*elem))
}

func (l *pdfLayout) draw(command string) {
	if l.emit != nil {
		l.emit(command)
	}
}

func (l *pdfLayout) place(line Line) error {
	if line.Style == "heading" && l.y < 110 {
		if err := l.breakPage(); err != nil {
			return err
		}
		l.y = 790
	}
	if line.Style == "bar_bytes" {
		if l.y < 75 {
			if err := l.breakPage(); err != nil {
				return err
			}
			l.y = 790
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
				if l.emit != nil {
					elem := -1
					l.draw(fmt.Sprintf("/Artifact BMC 0.94 0.96 0.97 rg 42 %d 68 11 re f 0.90 0.93 0.94 rg 165 %d 250 11 re f 0.09 0.55 0.42 rg 165 %d %d 11 re f EMC %s0.12 0.18 0.24 rg BT /F1 8 Tf 43 %d Td (%s) Tj ET BT /F1 9 Tf 425 %d Td (%d B) Tj ET EMC\n", l.y-2, l.y-2, l.y-2, width*250/300, l.markText(&elem, "P"), l.y, escapePDFText(parts[0]), l.y, value))
				}
				l.y -= 20
				return nil
			}
		}
	}
	if line.Style == "bar" {
		if l.y < 75 {
			if err := l.breakPage(); err != nil {
				return err
			}
			l.y = 790
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
				if l.emit != nil {
					elem := -1
					l.draw(fmt.Sprintf("/Artifact BMC 0.94 0.96 0.97 rg 42 %d 68 11 re f 0.90 0.93 0.94 rg 115 %d 300 11 re f 0.09 0.55 0.42 rg 115 %d %d 11 re f EMC %s0.12 0.18 0.24 rg BT /F1 9 Tf 43 %d Td (%s) Tj ET BT /F1 9 Tf 425 %d Td (%d) Tj ET EMC\n", l.y-2, l.y-2, l.y-2, width, l.markText(&elem, "P"), l.y, escapePDFText(parts[0]), l.y, value))
				}
				l.y -= 20
				return nil
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
	if l.y < 780 {
		switch line.Style {
		case "heading":
			l.y -= 10
		case "subheading":
			l.y -= 4
		}
	}
	width := 90
	if fontSize == 18 {
		width = 54
	}
	if fontSize == 13 {
		width = 75
	}
	elem := -1
	role := structRole(line.Style)
	for _, part := range wrapText(line.Text, width) {
		if l.y < 55 {
			if err := l.breakPage(); err != nil {
				return err
			}
			l.y = 790
		}
		color := "0.15 0.20 0.26"
		if line.Style == "title" || line.Style == "heading" {
			color = "0.08 0.45 0.35"
		}
		if l.emit != nil {
			// A blank spacer line is layout, not content.
			open := "/Artifact BMC "
			if strings.TrimSpace(part) != "" {
				open = l.markText(&elem, role)
			}
			l.draw(fmt.Sprintf("%s%s rg BT /F1 %d Tf 42 %d Td (%s) Tj ET EMC\n", open, color, fontSize, l.y, escapePDFText(part)))
		}
		l.y -= step
		fontSize, step = 10, 14
	}
	return nil
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

// winAnsiExtra maps the punctuation WinAnsiEncoding places in 0x80-0x9F, so
// report text is not silently replaced by "?".
var winAnsiExtra = map[rune]byte{'€': 0x80, '‚': 0x82, '„': 0x84, '…': 0x85, '‘': 0x91, '’': 0x92, '“': 0x93, '”': 0x94, '•': 0x95, '–': 0x96, '—': 0x97, '™': 0x99}

func escapePDFText(value string) string {
	var out strings.Builder
	for _, r := range value {
		b := byte('?')
		if r >= 32 && r <= 255 {
			b = byte(r)
		} else if mapped, ok := winAnsiExtra[r]; ok {
			b = mapped
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
