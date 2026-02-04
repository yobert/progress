package main

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/yobert/progress"
)

func main() {
	rand.Seed(time.Now().Unix())

	fast()
	smooth()
	//infinite()
	//chunky()
}

func infinite() {
	ms := 100
	step := 1000
	bar := progress.NewBar(0, "Infinite")
	defer bar.Done()
	for {
		time.Sleep(time.Millisecond * time.Duration(rand.Intn(ms)))
		bar.Add(rand.Intn(step))
	}
}

func chunky() {
	max := 600
	step := 10
	ms := 1000
	bar := progress.NewBar(max, "Chunky")
	for i := 0; i < max; i += step {
		time.Sleep(time.Millisecond * time.Duration(rand.Intn(ms)))
		bar.Add(step)
	}
	bar.Done()
}

func smooth() {
	max := 12550
	step := 1
	ms := 10
	bar := progress.NewBar(max, "Smooth...")
	for i := 0; i < max; i += step {
		time.Sleep(time.Millisecond * time.Duration(rand.Intn(ms)))
		bar.Add(step)
		if i == max/2 {
			bar.SetMsg("Smooth part 2...")
		}
	}
	bar.Done()
}

func fast() {
	max := 6000
	step := 50
	ms := 10
	bar := progress.NewBar(max, "Fast")
	for i := 0; i < max; i += step {
		time.Sleep(time.Millisecond * time.Duration(rand.Intn(ms)))
		bar.Add(step)
		if i > 1000 && i < 1020 {
			bar.Println(fmt.Sprintf("Sweet %d !!!", i))
		}
	}
	bar.Done()
}
