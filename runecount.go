package progress

import (
	"regexp"

	"github.com/mattn/go-runewidth"
)

// Shamelessly ripped off from github.com/cheggaaa/pb/runecount.go

// Finds the control character sequences (like colors)
var ctrlRe = regexp.MustCompile("\x1b\x5b[0-9]+\x6d")

func RuneCount(s string) int {
	n := runewidth.StringWidth(s)
	for _, sm := range ctrlRe.FindAllString(s, -1) {
		n -= runewidth.StringWidth(sm)
	}
	return n
}
