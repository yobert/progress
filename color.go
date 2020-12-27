package progress

import (
	"strconv"
)

type color int

const (
	black color = iota + 1
	red
	green
	yellow
	blue
	magenta
	cyan
	white
)

func fg(c color) string {
	return "\x1b[" + strconv.Itoa(int(c)+29) + "m"
}
func bright(c color) string {
	return "\x1b[1;" + strconv.Itoa(int(c)+29) + "m"
}
func bg(c color, b color) string {
	return "\x1b[" + strconv.Itoa(int(c)+29) + ";" + strconv.Itoa(int(b)+39) + "m"
}
func brightbg(c color, b color) string {
	return "\x1b[1;" + strconv.Itoa(int(c)+29) + ";" + strconv.Itoa(int(b)+39) + "m"
}
func reset() string {
	return "\x1b[m"
}
