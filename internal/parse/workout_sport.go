package parse

// pickSportType resolves Zepp sport subtype from protobuf summary blobs.
func pickSportType(blob []byte, meta map[int][]protoField) int {
	top := readProtoMsg(trimWorkoutHeader(blob))
	if sp := bestSportFromField3(top); sp > 0 {
		return sp
	}
	if sp := bestSportFromField3(meta); sp > 0 {
		return sp
	}
	alts := sportIDsFromField3(blob)
	if len(alts) == 0 {
		return 0
	}
	last := alts[len(alts)-1]
	if last != 0 && isKnownSport(last) {
		return last
	}
	return 0
}

func bestSportFromField3(m map[int][]protoField) int {
	var varintCand int
	for _, f := range m[3] {
		if f.kind == 2 {
			if sp := decodeSportField(f.bytes); sp > 0 && isKnownSport(sp) {
				return sp
			}
			sub := readProtoMsg(f.bytes)
			if v := protoInt(sub, 1); v != nil && *v > 0 && isKnownSport(*v) {
				return *v
			}
		} else if f.kind == 0 && f.intV > 0 && isKnownSport(f.intV) {
			varintCand = f.intV
		}
	}
	return varintCand
}

func sportIDsFromField3(b []byte) []int {
	var out []int
	var walk func([]byte)
	walk = func(buf []byte) {
		o := 0
		for o < len(buf) {
			field, wire, n := readTag(buf, o)
			if n <= o {
				break
			}
			o = n
			switch wire {
			case 2:
				chunk, n2 := readBytes(buf, o)
				o = n2
				if chunk == nil {
					continue
				}
				if field == 3 {
					if sp := decodeSportField(chunk); sp > 0 {
						out = append(out, sp)
					}
					if v := protoInt(readProtoMsg(chunk), 1); v != nil && *v > 0 {
						out = append(out, *v)
					}
				}
				walk(chunk)
			case 0:
				_, n2 := readVarint(buf, o)
				o = n2
			case 5:
				if o+4 > len(buf) {
					return
				}
				o += 4
			case 1:
				if o+8 > len(buf) {
					return
				}
				o += 8
			default:
				return
			}
		}
	}
	walk(b)
	return out
}

func isKnownSport(id int) bool {
	_, ok := sportNamesAll[id]
	return ok
}
