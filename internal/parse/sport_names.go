package parse

import "fmt"

// SportName maps Zepp OS workout subType IDs to display labels.
func SportName(id int) string {
	if n, ok := sportNamesAll[id]; ok {
		return n
	}
	if id == 0 {
		return "Workout"
	}
	return fmt.Sprintf("Activity %d", id)
}

var sportNamesAll = func() map[int]string {
	out := map[int]string{}
	for k, v := range sportNamesA {
		out[k] = v
	}
	for k, v := range sportNamesB {
		out[k] = v
	}
	return out
}()
