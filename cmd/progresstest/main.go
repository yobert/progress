package main

import (
	"math/rand"
	"time"

	"github.com/yobert/progress"
)

func main() {
	title := "" //"Testing progress bar..."
	max := 60000
	step := 10
	ms := 10
	rand.Seed(time.Now().Unix())
	bar := progress.NewBar(max, title)
	for i := 0; i < max; i += step {
		time.Sleep(time.Millisecond * time.Duration(rand.Intn(ms)))
		bar.Add(step)
	}
	bar.Done()
}
