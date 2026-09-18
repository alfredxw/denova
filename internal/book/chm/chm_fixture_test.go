package chm

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/text/encoding/simplifiedchinese"
)

// The sample.chm fixture under internal/book/testdata is an uncompressed ITSF
// v3 container holding three GBK-encoded HTML topics plus a .hhc table of
// contents, shaped like what the Windows HTML Help compiler emits for a
// Chinese novel. buildSampleCHM constructs those bytes; run
// `CHM_REGEN_FIXTURE=1 go test ./internal/book/chm/ -run TestRegenerateFixture`
// from the repository root to rewrite the committed fixture after changing it.

type fixtureTopic struct {
	name  string
	title string
	lines []string
	// noHeading omits the topic's own <h1> so nested leaves exercise the
	// importer's section-title prefix path.
	noHeading bool
}

var fixtureTopics = []fixtureTopic{
	{"01.html", "第一章 初雪", []string{"夜色沉沉，长街上的灯笼一盏盏熄灭。", "她裹紧了斗篷，踏着积雪向城门走去。"}, false},
	{"02.html", "第二章 夜行", []string{"城门下的守卫缩在火盆旁，谁也没有拦她。", "出了城，风雪忽然大了起来。"}, false},
	{"03.html", "第三章 归途", []string{"驿站的老马认得她，打了个响鼻。", "她翻身上马，朝着北方的群山疾驰而去。"}, false},
	{"04.html", "附录A", []string{"附录A 的第一段内容。"}, true},
	{"05.html", "附录B", []string{"附录B 的第一段内容。"}, true},
}

func gbkBytes(t *testing.T, s string) []byte {
	t.Helper()
	b, err := simplifiedchinese.GB18030.NewEncoder().Bytes([]byte(s))
	if err != nil {
		t.Fatalf("encode gbk: %v", err)
	}
	return b
}

func buildSampleCHM(t *testing.T) []byte {
	t.Helper()

	files := map[string][]byte{}
	for _, topic := range fixtureTopics {
		var body []byte
		body = append(body, gbkBytes(t, "<!DOCTYPE HTML PUBLIC \"-//IETF//DTD HTML//EN\">\n<html>\n<head>\n<meta http-equiv=\"Content-Type\" content=\"text/html; charset=gb2312\">\n<title>")...)
		body = append(body, gbkBytes(t, topic.title)...)
		body = append(body, gbkBytes(t, "</title>\n</head>\n<body>\n")...)
		if !topic.noHeading {
			body = append(body, gbkBytes(t, "<h1>"+topic.title+"</h1>\n")...)
		}
		for _, line := range topic.lines {
			body = append(body, gbkBytes(t, "<p>"+line+"</p>\n")...)
		}
		body = append(body, gbkBytes(t, "</body>\n</html>\n")...)
		files[topic.name] = body
	}

	var hhc []byte
	hhc = append(hhc, gbkBytes(t, "<!DOCTYPE HTML PUBLIC \"-//IETF//DTD HTML//EN\">\n<HTML>\n<BODY>\n<UL>\n")...)
	for _, topic := range fixtureTopics[:3] {
		hhc = append(hhc, gbkBytes(t, "\t<LI> <OBJECT type=\"text/sitemap\">\n\t\t<param name=\"Name\" value=\"")...)
		hhc = append(hhc, gbkBytes(t, topic.title)...)
		hhc = append(hhc, gbkBytes(t, "\">\n\t\t<param name=\"Local\" value=\"")...)
		hhc = append(hhc, []byte(topic.name)...)
		hhc = append(hhc, gbkBytes(t, "\">\n\t\t</OBJECT>\n")...)
	}
	// A group entry without its own page, nesting a pure section group one
	// level below and the leaf topics another level down — the
	// volume/group/chapter layering the importer maps to Denova's tree.
	hhc = append(hhc, gbkBytes(t, "\t<LI> <OBJECT type=\"text/sitemap\">\n\t\t<param name=\"Name\" value=\"附录\">\n\t\t</OBJECT>\n\t\t<UL>\n")...)
	hhc = append(hhc, gbkBytes(t, "\t\t<LI> <OBJECT type=\"text/sitemap\">\n\t\t\t<param name=\"Name\" value=\"附录图表\">\n\t\t\t</OBJECT>\n\t\t\t<UL>\n")...)
	for _, topic := range fixtureTopics[3:] {
		hhc = append(hhc, gbkBytes(t, "\t\t\t\t<LI> <OBJECT type=\"text/sitemap\">\n\t\t\t\t\t<param name=\"Name\" value=\"")...)
		hhc = append(hhc, gbkBytes(t, topic.title)...)
		hhc = append(hhc, gbkBytes(t, "\">\n\t\t\t\t\t<param name=\"Local\" value=\"")...)
		hhc = append(hhc, []byte(topic.name)...)
		hhc = append(hhc, gbkBytes(t, "\">\n\t\t\t\t</OBJECT>\n\t\t\t\t")...)
	}
	hhc = append(hhc, gbkBytes(t, "</UL>\n\t\t")...)
	hhc = append(hhc, gbkBytes(t, "</UL>\n")...)
	hhc = append(hhc, gbkBytes(t, "</UL>\n</BODY>\n</HTML>\n")...)
	files["sample.hhc"] = hhc

	sys := []byte{3, 0, 4, 0}
	sys = binary.LittleEndian.AppendUint32(sys, 0x0804)
	sys = append(sys, 2, 0, byte(len("01.html")), 0)
	sys = append(sys, "01.html"...)
	sys = append(sys, 3, 0, byte(len("Sample Novel")), 0)
	sys = append(sys, "Sample Novel"...)
	files["#SYSTEM"] = sys

	names := []string{"#SYSTEM"}
	for _, topic := range fixtureTopics {
		names = append(names, topic.name)
	}
	names = append(names, "sample.hhc")

	data := []byte{}
	starts := make(map[string]int, len(names))
	for _, name := range names {
		starts[name] = len(data)
		data = append(data, files[name]...)
	}

	var entries []byte
	for _, name := range names {
		entries = append(entries, byte(len(name)))
		entries = append(entries, name...)
		entries = appendCHMWord(entries, 0)
		entries = appendCHMWord(entries, uint64(starts[name]))
		entries = appendCHMWord(entries, uint64(len(files[name])))
	}

	const blockLen = 0x1000
	const headLen = 0x54
	if 0x14+len(entries) > blockLen-2 {
		t.Fatalf("directory entries overflow the PMGL chunk")
	}
	chunk := make([]byte, blockLen)
	copy(chunk, "PMGL")
	binary.LittleEndian.PutUint32(chunk[4:], uint32(blockLen-0x14-len(entries)-2))
	copy(chunk[0x14:], entries)

	dir := make([]byte, headLen)
	copy(dir, "ITSP")
	binary.LittleEndian.PutUint32(dir[0x04:], 1)
	binary.LittleEndian.PutUint32(dir[0x08:], headLen)
	binary.LittleEndian.PutUint32(dir[0x0C:], 0x0A000002)
	binary.LittleEndian.PutUint32(dir[0x10:], blockLen)
	binary.LittleEndian.PutUint32(dir[0x14:], 2)
	binary.LittleEndian.PutUint32(dir[0x18:], 1)
	binary.LittleEndian.PutUint32(dir[0x1C:], 1)
	binary.LittleEndian.PutUint32(dir[0x20:], 0x0804)
	copy(dir[0x24:], []byte{0x6A, 0x92, 0x02, 0x5D, 0x2E, 0x21, 0xD0, 0x11, 0x9D, 0xF9, 0x00, 0xA0, 0xC9, 0x22, 0xE6, 0xEC})
	binary.LittleEndian.PutUint32(dir[0x34:], 1)
	dir = append(dir, chunk...)

	const dirOff = 0x60
	dirLen := len(dir)
	head := make([]byte, dirOff)
	copy(head, "ITSF")
	binary.LittleEndian.PutUint32(head[0x04:], 3)
	binary.LittleEndian.PutUint32(head[0x0C:], dirOff)
	binary.LittleEndian.PutUint32(head[0x14:], 0x0804)
	binary.LittleEndian.PutUint64(head[0x48:], dirOff)
	binary.LittleEndian.PutUint64(head[0x50:], uint64(dirLen))
	binary.LittleEndian.PutUint64(head[0x58:], uint64(dirOff+dirLen))

	out := append([]byte{}, head...)
	out = append(out, dir...)
	return append(out, data...)
}

func appendCHMWord(b []byte, v uint64) []byte {
	if v == 0 {
		return append(b, 0)
	}
	var groups [10]byte
	n := 0
	for v > 0 {
		groups[n] = byte(v & 0x7f)
		v >>= 7
		n++
	}
	for i := n - 1; i >= 0; i-- {
		c := groups[i]
		if i > 0 {
			c |= 0x80
		}
		b = append(b, c)
	}
	return b
}

func TestRegenerateFixture(t *testing.T) {
	if os.Getenv("CHM_REGEN_FIXTURE") == "" {
		t.Skip("set CHM_REGEN_FIXTURE=1 to rewrite internal/book/testdata/sample.chm")
	}
	path := filepath.Join("..", "testdata", "sample.chm")
	if err := os.WriteFile(path, buildSampleCHM(t), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	t.Logf("fixture written to %s", path)
}
