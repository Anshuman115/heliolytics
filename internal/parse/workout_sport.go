package parse

import (
	"fmt"
	"math"
)

// workoutSport reads the confirmed numeric sport id from field 3. The name
// table never decides whether an id is kept.
func workoutSport(message *protoMessage, budget *protoBudget) (int, error) {
	if message == nil {
		return 0, nil
	}
	for _, value := range message.fields[3] {
		if value.varint {
			return checkedSport(value.number)
		}
		sub, err := readMessage(value.bytes, 1, budget)
		if err != nil {
			return 0, err
		}
		if number, ok := varintField(sub, 1); ok {
			return checkedSport(number)
		}
	}
	return 0, nil
}

func checkedSport(value uint64) (int, error) {
	if value > math.MaxInt16 {
		return 0, fmt.Errorf("sport id %d cannot be stored in int16", value)
	}
	return int(value), nil
}
