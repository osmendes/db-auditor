package report

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode/utf16"
)

// pdfObjects splits a PDF written by this package into its objects.
func pdfObjects(t *testing.T, pdf []byte) map[int][]byte {
	t.Helper()
	out := map[int][]byte{}
	header := regexp.MustCompile(`(?m)^(\d+) 0 obj\n`)
	matches := header.FindAllSubmatchIndex(pdf, -1)
	for i, m := range matches {
		id, _ := strconv.Atoi(string(pdf[m[2]:m[3]]))
		end := len(pdf)
		if i+1 < len(matches) {
			end = matches[i+1][0]
		}
		body := pdf[m[1]:end]
		body = body[:bytes.LastIndex(body, []byte("\nendobj\n"))]
		out[id] = body
	}
	if len(out) == 0 {
		t.Fatal("no PDF objects")
	}
	return out
}

func ref(t *testing.T, body []byte, key string) int {
	t.Helper()
	m := regexp.MustCompile(regexp.QuoteMeta(key) + ` (\d+) 0 R`).FindSubmatch(body)
	if m == nil {
		t.Fatalf("%s not found in %s", key, body)
	}
	id, _ := strconv.Atoi(string(m[1]))
	return id
}

func streamData(t *testing.T, body []byte) []byte {
	t.Helper()
	start := bytes.Index(body, []byte("\nstream\n"))
	m := regexp.MustCompile(`/Length (\d+)`).FindSubmatch(body)
	if start < 0 || m == nil {
		t.Fatalf("not a stream: %.80s", body)
	}
	n, _ := strconv.Atoi(string(m[1]))
	return body[start+8 : start+8+n]
}

func catalog(t *testing.T, pdf []byte, objects map[int][]byte) []byte {
	t.Helper()
	return objects[ref(t, pdf[bytes.LastIndex(pdf, []byte("trailer")):], "/Root")]
}

func taggedFixtures() map[string]Document {
	docs := goldenDocuments()
	docs["volumous"] = volumousDocument()
	return docs
}

// #107: the PDF embeds its font (a WinAnsi subset of Liberation Sans, with
// Helvetica metrics), so accents render without a viewer-side font.
func TestPDFEmbedsFontSubset(t *testing.T) {
	pdf := mustRenderPDF(t, goldenDocuments()["partial"])
	if bytes.Contains(pdf, []byte("/Type1")) || bytes.Contains(pdf, []byte("/Helvetica")) {
		t.Fatal("PDF still references the base-14 Helvetica")
	}
	objects := pdfObjects(t, pdf)
	pages := regexp.MustCompile(`/Type /Page /Parent`)
	var fontID int
	for _, body := range objects {
		if pages.Match(body) {
			fontID = ref(t, body, "/F1")
			break
		}
	}
	fontDict := objects[fontID]
	if !regexp.MustCompile(`^<< /Type /Font /Subtype /TrueType /BaseFont /[A-Z]{6}\+LiberationSans /FirstChar 32 /LastChar 255 /Widths \[`).Match(fontDict) ||
		!bytes.Contains(fontDict, []byte("/Encoding /WinAnsiEncoding")) {
		t.Fatalf("font dictionary: %.200s", fontDict)
	}
	widths := strings.Fields(string(regexp.MustCompile(`/Widths \[([^\]]*)\]`).FindSubmatch(fontDict)[1]))
	if len(widths) != 224 {
		t.Fatalf("%d widths", len(widths))
	}
	// Helvetica (and Liberation Sans) advance widths, in 1/1000 em.
	for char, want := range map[byte]string{' ': "278", 'A': "667", 'W': "944", 'a': "556", 'i': "222", 0xe7: "500", 0xe3: "556", 0x97: "1000"} {
		if got := widths[int(char)-32]; got != want {
			t.Errorf("width of %#x = %s, want %s", char, got, want)
		}
	}
	descriptor := objects[ref(t, fontDict, "/FontDescriptor")]
	if !bytes.Contains(descriptor, []byte("/Flags 32")) {
		t.Fatalf("descriptor: %s", descriptor)
	}
	fileObject := objects[ref(t, descriptor, "/FontFile2")]
	zr, err := zlib.NewReader(bytes.NewReader(streamData(t, fileObject)))
	if err != nil {
		t.Fatal(err)
	}
	program, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(fileObject, []byte(fmt.Sprintf("/Length1 %d", len(program)))) {
		t.Fatal("Length1 does not match the font program")
	}
	font, err := parseTTF(program)
	if err != nil {
		t.Fatal(err)
	}
	for _, tag := range []string{"cmap", "glyf", "head", "hhea", "hmtx", "loca", "maxp", "cvt ", "fpgm", "prep"} {
		if _, err := font.table(tag); err != nil {
			t.Fatalf("embedded font lacks %q", tag)
		}
	}
	if _, err := font.table("name"); err == nil {
		t.Fatal("subset keeps tables the PDF does not need")
	}
	if ttfChecksum(program) != 0xB1B0AFBA {
		t.Fatal("font checksum adjustment is wrong")
	}
	cmap, _ := font.table("cmap")
	sub, err := cmapFormat4(cmap)
	if err != nil {
		t.Fatal(err)
	}
	loca, _ := font.table("loca")
	glyphLen := func(r rune) int {
		g := glyphIndex(sub, r)
		return int(binary.BigEndian.Uint32(loca[4*g+4:]) - binary.BigEndian.Uint32(loca[4*g:]))
	}
	for _, r := range "çãõéêíóúÁÇ€—“”•" {
		if glyphIndex(sub, r) == 0 || glyphLen(r) == 0 {
			t.Errorf("glyph %q missing from the subset", r)
		}
	}
	if glyphIndex(sub, 'Ж') == 0 || glyphLen('Ж') != 0 {
		t.Error("glyphs outside WinAnsi must be emptied from the subset")
	}
	if len(program) > len(liberationSans)/2 {
		t.Fatalf("subset is %d bytes of %d", len(program), len(liberationSans))
	}
}

func utf16Hex(value string) string {
	var out strings.Builder
	out.WriteString("<FEFF")
	for _, unit := range utf16.Encode([]rune(value)) {
		fmt.Fprintf(&out, "%04X", unit)
	}
	return out.String() + ">"
}

// #107: viewers show the document title (Info and XMP) instead of the file
// name, with accents intact.
func TestPDFTitleMetadata(t *testing.T) {
	for name, d := range goldenDocuments() {
		t.Run(name, func(t *testing.T) {
			pdf := mustRenderPDF(t, d)
			objects := pdfObjects(t, pdf)
			trailer := pdf[bytes.LastIndex(pdf, []byte("trailer")):]
			info := objects[ref(t, trailer, "/Info")]
			want := "DB Auditor - Relatório " + reportTypeLabel(d.Type) + " - staging-erp - execução 00000000"
			if PDFTitle(d) != want || !bytes.Contains(info, []byte("/Title "+utf16Hex(want))) || !bytes.Contains(info, []byte("/Producer (DB Auditor)")) {
				t.Fatalf("info %s, title %q", info, PDFTitle(d))
			}
			root := catalog(t, pdf, objects)
			if !bytes.Contains(root, []byte("/ViewerPreferences << /DisplayDocTitle true >>")) {
				t.Fatalf("catalog %s", root)
			}
			xmp := string(streamData(t, objects[ref(t, root, "/Metadata")]))
			var escaped strings.Builder
			for _, r := range want {
				fmt.Fprintf(&escaped, "&#x%X;", r)
			}
			if !strings.Contains(xmp, `<dc:title><rdf:Alt><rdf:li xml:lang="x-default">`+escaped.String()+`</rdf:li>`) || !strings.Contains(xmp, "<rdf:li>pt-BR</rdf:li>") {
				t.Fatalf("xmp %s", xmp)
			}
		})
	}
	if got := PDFTitle(Document{Type: "executive"}); got != "DB Auditor - Relatório executivo" {
		t.Fatalf("title without environment or run: %q", got)
	}
	pdf := mustRenderPDF(t, Document{Type: "executive", Environment: "x\nendstream\nendobj (", RunID: "safe"})
	if bytes.Contains(pdf, []byte("x\nendstream")) {
		t.Fatal("untrusted environment name written verbatim into the metadata")
	}
}

var markedOperator = regexp.MustCompile(`/(\w+) (?:<< /MCID (\d+) >> BDC|BMC)|EMC|BT`)

// #107: the PDF is tagged. Every text run is inside marked content: page
// furniture (background, running header, footer, bar rectangles, blank
// lines) is an artifact, every report line is a structure element (H1, H2,
// H3 or P) reached from the structure tree and the parent tree, in reading
// order.
func TestPDFIsTagged(t *testing.T) {
	for name, d := range taggedFixtures() {
		t.Run(name, func(t *testing.T) {
			pdf := mustRenderPDF(t, d)
			objects := pdfObjects(t, pdf)
			root := catalog(t, pdf, objects)
			if !bytes.Contains(root, []byte("/MarkInfo << /Marked true >>")) || !bytes.Contains(root, []byte("/Lang (pt-BR)")) {
				t.Fatalf("catalog %s", root)
			}
			treeRoot := objects[ref(t, root, "/StructTreeRoot")]
			if !bytes.HasPrefix(treeRoot, []byte("<< /Type /StructTreeRoot ")) {
				t.Fatalf("tree root %s", treeRoot)
			}
			document := objects[ref(t, treeRoot, "/K")]
			if !bytes.HasPrefix(document, []byte("<< /Type /StructElem /S /Document ")) {
				t.Fatalf("document element %.100s", document)
			}
			children := regexp.MustCompile(`(\d+) 0 R`).FindAllSubmatch(regexp.MustCompile(`/K \[([^\]]*)\]`).FindSubmatch(document)[1], -1)
			nums := regexp.MustCompile(`(\d+) \[([^\]]*)\]`).FindAllSubmatch(objects[ref(t, treeRoot, "/ParentTree")], -1)
			pagesObject := objects[ref(t, root, "/Pages")]
			kids := regexp.MustCompile(`(\d+) 0 R`).FindAllSubmatch(regexp.MustCompile(`/Kids \[([^\]]*)\]`).FindSubmatch(pagesObject)[1], -1)
			if len(nums) != len(kids) || !bytes.Contains(treeRoot, []byte(fmt.Sprintf("/ParentTreeNextKey %d", len(kids)))) {
				t.Fatalf("parent tree has %d pages of %d", len(nums), len(kids))
			}
			order := []string{}
			lines := BuildLines(d)
			tagged := 0
			for _, line := range lines {
				if strings.TrimSpace(line.Text) != "" {
					tagged++
				}
			}
			for pageIndex, kid := range kids {
				pageID, _ := strconv.Atoi(string(kid[1]))
				page := objects[pageID]
				if !bytes.Contains(page, []byte(fmt.Sprintf("/StructParents %d ", pageIndex))) {
					t.Fatalf("page %d lacks StructParents: %s", pageIndex, page)
				}
				content := streamData(t, objects[ref(t, page, "/Contents")])
				depth, mcids := 0, 0
				for _, m := range markedOperator.FindAllSubmatch(content, -1) {
					switch {
					case string(m[0]) == "EMC":
						depth--
					case string(m[0]) == "BT":
						if depth != 1 {
							t.Fatalf("page %d: text outside marked content", pageIndex)
						}
					default:
						if depth != 0 {
							t.Fatalf("page %d: nested marked content", pageIndex)
						}
						depth++
						if m[2] != nil {
							if mcid, _ := strconv.Atoi(string(m[2])); mcid != mcids {
								t.Fatalf("page %d: MCID %d out of order", pageIndex, mcid)
							}
							mcids++
						} else if string(m[1]) != "Artifact" {
							t.Fatalf("page %d: untagged %s", pageIndex, m[1])
						}
					}
				}
				if depth != 0 {
					t.Fatalf("page %d: unbalanced marked content", pageIndex)
				}
				entries := regexp.MustCompile(`(\d+) 0 R`).FindAllSubmatch(nums[pageIndex][2], -1)
				if string(nums[pageIndex][1]) != strconv.Itoa(pageIndex) || len(entries) != mcids {
					t.Fatalf("page %d: parent tree lists %d of %d MCIDs", pageIndex, len(entries), mcids)
				}
				for mcid, entry := range entries {
					elemID, _ := strconv.Atoi(string(entry[1]))
					elem := objects[elemID]
					role := regexp.MustCompile(`^<< /Type /StructElem /S /(H1|H2|H3|P) /P \d+ 0 R /Pg (\d+) 0 R /K \[`).FindSubmatch(elem)
					if role == nil {
						t.Fatalf("element %s", elem)
					}
					own := string(role[2]) == strconv.Itoa(pageID) && regexp.MustCompile(fmt.Sprintf(`/K \[(?:[^\]]* )?%d(?: |\])`, mcid)).Match(elem)
					if !own && !bytes.Contains(elem, []byte(fmt.Sprintf("<< /Type /MCR /Pg %d 0 R /MCID %d >>", pageID, mcid))) {
						t.Fatalf("element %d does not own MCID %d of page %d: %s", elemID, mcid, pageIndex, elem)
					}
					if len(order) == 0 || order[len(order)-1] != string(entry[1]) {
						order = append(order, string(entry[1]))
					}
				}
			}
			if len(children) != tagged {
				t.Fatalf("%d structure elements for %d non-blank lines", len(children), tagged)
			}
			for i, child := range children {
				if i >= len(order) || order[i] != string(child[1]) {
					t.Fatalf("structure order differs from the reading order at %d", i)
				}
			}
			first, _ := strconv.Atoi(string(children[0][1]))
			if !bytes.Contains(objects[first], []byte("/S /H1")) {
				t.Fatal("first element must be the H1 title")
			}
		})
	}
}

// #107: the strings QA flagged render as accented Portuguese, and the rule
// summary is Portuguese.
func TestPDFReportStringsArePortuguese(t *testing.T) {
	d := goldenDocuments()["partial"]
	d.Findings[0].Summary = "idx_scan did not grow between two collections. Manual review required; no automatic DROP is suggested."
	pdf := mustRenderPDF(t, d)
	text := string(pdf)
	for _, want := range []string{"Limite: Execução parcial; ausência de objetos/achados não é prova de resolução.", "Resumo da regra: idx_scan igual nas duas coletas, sem stats_reset no intervalo."} {
		if !strings.Contains(text, escapePDFText(want)) {
			t.Errorf("PDF lacks %q", want)
		}
	}
	for _, unwanted := range []string{"Execucao", "ausencia", "nao e prova", "Resumo original", "did not grow", "Manual review"} {
		if strings.Contains(text, unwanted) {
			t.Errorf("PDF still contains %q", unwanted)
		}
	}
	lines := BuildLines(d)
	for _, line := range lines {
		if strings.HasPrefix(line.Text, "Limite:") && unaccented.MatchString(line.Text) {
			t.Errorf("unaccented limit line %q", line.Text)
		}
	}
}

var unaccented = regexp.MustCompile(`(?i)\b(execucao|ausencia|nao|resolucao|relatorio|findings)\b`)
