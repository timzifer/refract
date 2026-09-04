package pdf

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"math"
	"strconv"
)

// document accumulates numbered PDF objects and serialises them with a cross
// reference table.
//
// Objects are numbered as they are reserved rather than as they are written,
// because a page has to name its content stream before that stream exists.
type document struct {
	bodies [][]byte
}

// reserve allocates an object number without a body yet.
func (d *document) reserve() int {
	d.bodies = append(d.bodies, nil)
	return len(d.bodies)
}

// set fills in a reserved object.
func (d *document) set(num int, body []byte) { d.bodies[num-1] = body }

// add appends an object and returns its number.
func (d *document) add(body []byte) int {
	n := d.reserve()
	d.set(n, body)
	return n
}

// addStream appends a stream object, deflating it unless compression is off.
func (d *document) addStream(dict string, data []byte, compress bool) int {
	var b bytes.Buffer
	b.WriteString("<< ")
	b.WriteString(dict)
	if compress {
		var z bytes.Buffer
		w := zlib.NewWriter(&z)
		_, _ = w.Write(data)
		// zlib.Writer only errors through the underlying writer, and a
		// bytes.Buffer has no failure mode.
		_ = w.Close()
		data = z.Bytes()
		b.WriteString(" /Filter /FlateDecode")
	}
	fmt.Fprintf(&b, " /Length %d >>\nstream\n", len(data))
	b.Write(data)
	b.WriteString("\nendstream")
	return d.add(b.Bytes())
}

// writeTo serialises the document with root as its catalogue and, when info
// is non-zero, that object as its information dictionary.
func (d *document) writeTo(w io.Writer, root, info int) error {
	var out bytes.Buffer
	out.WriteString("%PDF-1.7\n")
	// A comment of high-bit bytes marks the file as binary, so that tools
	// which sniff content do not mangle a deflated stream as if it were text.
	out.Write([]byte{'%', 0xE2, 0xE3, 0xCF, 0xD3, '\n'})

	offsets := make([]int, len(d.bodies))
	for i, body := range d.bodies {
		offsets[i] = out.Len()
		fmt.Fprintf(&out, "%d 0 obj\n", i+1)
		out.Write(body)
		out.WriteString("\nendobj\n")
	}

	start := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n", len(d.bodies)+1)
	out.WriteString("0000000000 65535 f \n")
	for _, off := range offsets {
		fmt.Fprintf(&out, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root %d 0 R", len(d.bodies)+1, root)
	if info != 0 {
		fmt.Fprintf(&out, " /Info %d 0 R", info)
	}
	fmt.Fprintf(&out, " >>\nstartxref\n%d\n%%%%EOF\n", start)

	_, err := w.Write(out.Bytes())
	return err
}

// coordDecimals is the precision every coordinate is written with. Three
// decimals is far below a pixel, and fixing the precision is what makes the
// output byte-stable rather than dependent on the last bit of a computation —
// the same reason backend/svg fixes it.
const coordDecimals = 3

// num writes a number in PDF's real-number syntax, which has no exponent form.
func num(w *bytes.Buffer, v float64) {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		// A non-finite coordinate would make the content stream unparseable.
		// The geom layer filters these out of anything carrying data, so this
		// is a backstop rather than a policy.
		w.WriteByte('0')
		return
	}
	start := w.Len()
	w.Write(strconv.AppendFloat(w.AvailableBuffer(), v, 'f', coordDecimals, 64))
	trimZeros(w, start)
}

// trimZeros rewrites the number just written at or after start, dropping a
// trailing zero run and normalising a signed zero to "0".
func trimZeros(w *bytes.Buffer, start int) {
	s := w.Bytes()[start:]
	if !bytes.ContainsRune(s, '.') {
		return
	}
	end := len(s)
	for end > 0 && s[end-1] == '0' {
		end--
	}
	if end > 0 && s[end-1] == '.' {
		end--
	}
	if isZero(s[:end]) {
		w.Truncate(start)
		w.WriteByte('0')
		return
	}
	w.Truncate(start + end)
}

func isZero(s []byte) bool {
	if len(s) > 0 && (s[0] == '-' || s[0] == '+') {
		s = s[1:]
	}
	for _, c := range s {
		if c != '0' {
			return false
		}
	}
	return true
}

// nums writes a space-separated run of numbers.
func nums(w *bytes.Buffer, vs ...float64) {
	for i, v := range vs {
		if i > 0 {
			w.WriteByte(' ')
		}
		num(w, v)
	}
}
