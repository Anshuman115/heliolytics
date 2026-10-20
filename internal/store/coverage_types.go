package store

var MeasuredTypeCodes = []string{
	"0x01", "0x05", "0x06", "0x0D", "0x13",
	"0x25", "0x2E", "0x38", "0x46", "0x48", "0x49",
}

func isMeasuredType(code string) bool {
	for _, c := range MeasuredTypeCodes {
		if c == code {
			return true
		}
	}
	return false
}
