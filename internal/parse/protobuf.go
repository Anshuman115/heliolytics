package parse

import "fmt"

const (
	protoWireVarint  = 0
	protoWireFixed64 = 1
	protoWireBytes   = 2
	protoWireFixed32 = 5
	protoMaxDepth    = 8
	protoMaxNodes    = 4096
	protoMaxVarint   = 10
)

type protoMessage struct {
	fields map[int][]protoValue
}

type protoValue struct {
	varint bool
	number uint64
	bytes  []byte
}

type protoBudget struct {
	nodes int
}

func (b *protoBudget) spend() error {
	b.nodes++
	if b.nodes > protoMaxNodes {
		return fmt.Errorf("message holds more than %d values", protoMaxNodes)
	}
	return nil
}

func readUvarint(buf []byte, off int) (uint64, int, error) {
	if off < 0 || off >= len(buf) {
		return 0, off, fmt.Errorf("varint offset %d is outside the %d-byte buffer", off, len(buf))
	}
	var value uint64
	for i := range protoMaxVarint {
		if off+i >= len(buf) {
			return 0, off, fmt.Errorf("varint at offset %d is truncated", off)
		}
		current := buf[off+i]
		if i == protoMaxVarint-1 && current > 1 {
			return 0, off, fmt.Errorf("varint at offset %d overflows 64 bits", off)
		}
		value |= uint64(current&0x7f) << (7 * i)
		if current&0x80 == 0 {
			return value, off + i + 1, nil
		}
	}
	return 0, off, fmt.Errorf("varint at offset %d is longer than ten bytes", off)
}

func readLengthDelimited(buf []byte, off int) ([]byte, int, error) {
	length, next, err := readUvarint(buf, off)
	if err != nil {
		return nil, off, err
	}
	if next > len(buf) || length > uint64(len(buf)-next) {
		return nil, off, fmt.Errorf("declared length %d exceeds %d remaining bytes", length, len(buf)-next)
	}
	end := next + int(length)
	return buf[next:end], end, nil
}

// readMessage decodes one message into its fields. Trailing zero and 0x80 bytes
// after the last field are tolerated because workout records are concatenated
// with a two-byte separator.
func readMessage(buf []byte, depth int, budget *protoBudget) (*protoMessage, error) {
	if depth > protoMaxDepth {
		return nil, fmt.Errorf("message nesting exceeds %d levels", protoMaxDepth)
	}
	message := &protoMessage{fields: make(map[int][]protoValue)}
	offset := 0
	for offset < len(buf) {
		tag, next, err := readUvarint(buf, offset)
		if err != nil {
			return nil, err
		}
		field := int(tag >> 3)
		wire := int(tag & 7)
		if field == 0 {
			if protoPadding(buf[offset:]) {
				break
			}
			return nil, fmt.Errorf("field number 0 at offset %d", offset)
		}
		if err := budget.spend(); err != nil {
			return nil, err
		}
		offset = next
		switch wire {
		case protoWireVarint:
			number, after, err := readUvarint(buf, offset)
			if err != nil {
				return nil, err
			}
			message.fields[field] = append(message.fields[field], protoValue{varint: true, number: number})
			offset = after
		case protoWireBytes:
			payload, after, err := readLengthDelimited(buf, offset)
			if err != nil {
				return nil, err
			}
			message.fields[field] = append(message.fields[field], protoValue{bytes: payload})
			offset = after
		case protoWireFixed32:
			if len(buf)-offset < 4 {
				return nil, fmt.Errorf("fixed32 field %d at offset %d is truncated", field, offset)
			}
			offset += 4
		case protoWireFixed64:
			if len(buf)-offset < 8 {
				return nil, fmt.Errorf("fixed64 field %d at offset %d is truncated", field, offset)
			}
			offset += 8
		default:
			return nil, fmt.Errorf("unsupported wire type %d for field %d", wire, field)
		}
	}
	return message, nil
}

func protoPadding(buf []byte) bool {
	for _, value := range buf {
		if value != 0x00 && value != 0x80 {
			return false
		}
	}
	return true
}

func subMessage(parent *protoMessage, field, depth int, budget *protoBudget) (*protoMessage, error) {
	if parent == nil {
		return nil, nil
	}
	for _, value := range parent.fields[field] {
		if value.varint {
			continue
		}
		return readMessage(value.bytes, depth+1, budget)
	}
	return nil, nil
}

func varintField(message *protoMessage, field int) (uint64, bool) {
	if message == nil {
		return 0, false
	}
	for _, value := range message.fields[field] {
		if value.varint {
			return value.number, true
		}
	}
	return 0, false
}
