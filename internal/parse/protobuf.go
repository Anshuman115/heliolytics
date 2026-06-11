package parse

func readVarint(b []byte, o int) (int, int) {
	v, shift := 0, 0
	for o < len(b) {
		x := int(b[o])
		o++
		v |= (x & 0x7f) << shift
		if x&0x80 == 0 {
			return v, o
		}
		shift += 7
		if shift > 63 {
			break
		}
	}
	return v, o
}

func readTag(b []byte, o int) (field, wire, next int) {
	tag, n := readVarint(b, o)
	return tag >> 3, tag & 7, n
}

func readBytes(b []byte, o int) ([]byte, int) {
	l, n := readVarint(b, o)
	end := n + l
	if end > len(b) {
		return nil, len(b)
	}
	return b[n:end], end
}
