package parse

// ParseActivitySessions parses 0x3B auto-detected activity sessions.
// Payload uses the same protobuf workout layout as 0x05.
func ParseActivitySessions(raw []byte) []WorkoutRecord {
	return ParseWorkouts(raw)
}
