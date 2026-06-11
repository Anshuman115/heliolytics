package parse

type protoField struct {
	kind  int
	intV  int
	bytes []byte
}

func readProtoMsg(b []byte) map[int][]protoField {
	m := map[int][]protoField{}
	o := 0
	for o < len(b) {
		field, wire, n := readTag(b, o)
		if n <= o {
			break
		}
		o = n
		switch wire {
		case 0:
			v, n2 := readVarint(b, o)
			o = n2
			m[field] = append(m[field], protoField{kind: 0, intV: v})
		case 2:
			chunk, n2 := readBytes(b, o)
			o = n2
			if chunk != nil {
				m[field] = append(m[field], protoField{kind: 2, bytes: chunk})
			}
		case 5:
			if o+4 > len(b) {
				return m
			}
			o += 4
		case 1:
			if o+8 > len(b) {
				return m
			}
			o += 8
		default:
			return m
		}
	}
	return m
}

func protoSub(m map[int][]protoField, field int) map[int][]protoField {
	for _, f := range m[field] {
		if f.kind == 2 {
			return readProtoMsg(f.bytes)
		}
	}
	return map[int][]protoField{}
}

func protoInt(m map[int][]protoField, field int) *int {
	for _, f := range m[field] {
		if f.kind == 0 {
			v := f.intV
			return &v
		}
	}
	return nil
}
