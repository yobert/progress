package progress

import (
	"fmt"
	"math"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/karrick/gows"
)

var blocks = []rune{' ', '▏', '▎', '▍', '▌', '▋', '▊', '▉', '█'}

//var blocks = []rune{' ', '▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

var pad = " "
var padlen = RuneCount(pad)

var suffixes = []string{"K", "M", "G", "T", "P"}

var (
	hidecursor = "\x1b[?25l"
	showcursor = "\x1b[?25h"
)

const (
	minbarsize = 5
	currentdur = time.Millisecond * 500
	sleeptime  = time.Millisecond * 50

//	sleeptime = time.Second
)

type Bar struct {
	at  int64
	max int64
	msg string

	finishmu sync.Mutex
	finished bool
	finish   chan struct{}
	done     chan struct{}
	winch    chan os.Signal
	stream   chan string
}

type segment struct {
	text     string
	size     int
	priority int
	align    int
	hide     bool
	barflex  bool
}

type sample struct {
	dur   time.Duration
	delta int
}

func NewBar(max int, msg string) *Bar {
	b := Bar{
		at:     0,
		max:    int64(max),
		msg:    msg,
		finish: make(chan struct{}),
		done:   make(chan struct{}),
		winch:  make(chan os.Signal, 1),
		stream: make(chan string),
	}

	signal.Notify(b.winch, syscall.SIGWINCH)

	go b.draw()
	return &b
}

func (b *Bar) Println(s string) {
	b.stream <- s
}

func (b *Bar) Next() {
	atomic.AddInt64(&b.at, 1)
}

func (b *Bar) Add(delta int) {
	atomic.AddInt64(&b.at, int64(delta))
}

func (b *Bar) Done() {
	b.finishmu.Lock()
	if !b.finished {
		b.finished = true
		close(b.finish)
	}
	b.finishmu.Unlock()
	_ = <-b.done
}

func getsize() int {
	size, _, _ := gows.GetWinSize()
	if size == 0 {
		size = 80
	}
	return size
}

func (b *Bar) draw() {
	size := getsize()
	start := time.Now()

	lastrunecount := 0
	laststr := ""
	clearstr := ""
	backstr := ""

	done := false

	sample_at := 0
	sample_time := start

	cursample_at := 0
	cursample_time := start
	cursample_dur := time.Duration(0)
	cursample_count := 0

	for !done {
		select {
		case s := <-b.stream:
			cm := lastrunecount
			clearadd := cm - len(clearstr)
			if clearadd > 0 {
				for i := 0; i < clearadd; i++ {
					clearstr += " "
				}
			}
			backadd := cm - len(backstr)
			if backadd > 0 {
				for i := 0; i < backadd; i++ {
					backstr += "\b"
				}
			}
			fmt.Print(backstr[0:lastrunecount] + hidecursor + clearstr[0:lastrunecount] + backstr[0:lastrunecount])
			laststr = ""
			lastrunecount = 0
			fmt.Println(s)
		case _ = <-b.finish:
			done = true
		case _ = <-b.winch:
			cm := lastrunecount
			clearadd := cm - len(clearstr)
			if clearadd > 0 {
				for i := 0; i < clearadd; i++ {
					clearstr += " "
				}
			}
			backadd := cm - len(backstr)
			if backadd > 0 {
				for i := 0; i < backadd; i++ {
					backstr += "\b"
				}
			}
			fmt.Print(backstr[0:lastrunecount] + hidecursor + clearstr[0:lastrunecount] + backstr[0:lastrunecount])
			laststr = ""
			lastrunecount = 0
			//fmt.Println()
			size = getsize()
		case _ = <-time.After(sleeptime):
		}

		at := int(b.at)
		max := int(b.max)
		now := sample_time
		realnow := time.Now()
		if at != sample_at {
			now = realnow
			sample_at = at
			sample_time = realnow
		}

		cd := realnow.Sub(cursample_time)
		if cd > currentdur {
			cursample_dur = cd
			cursample_count = at - cursample_at
			cursample_at = at
			cursample_time = realnow
		}

		ratio := float64(at) / float64(max)
		if ratio > 1 {
			ratio = 1
		}

		_ = cursample_dur
		_ = cursample_count
		_ = now
		_ = ratio
		_ = max

		segs := []segment{
			percentage(ratio, max),
			counts(at, max),
			//			curspeed(cursample_dur, cursample_count, ratio, max),
			avgspeed(now.Sub(start), at),
			//			title(b.msg),
			pbar(ratio, 0, max, b.msg),
			//			itsbeen(start, realnow),
			//			esttotal(start, now, ratio, max),
			//			remaining(start, now, ratio, max),
			timings(start, now, ratio, max),
		}

		// the actual progress bar has variable size, so lets
		// measure the other segments first to see how much to
		// expand it.

		// 1 space at the start, 1 space at the end, the cursor, and a space after that so ^C looks good
		//fixed_size := size - 4 ?
		fixed_size := size
		avail := fixed_size

		for _, seg := range segs {
			if seg.hide {
				continue
			}
			if avail != fixed_size {
				avail -= padlen
			}
			avail -= seg.size
		}

		// expand or shrink bar
		for i, seg := range segs {
			if seg.hide {
				continue
			}
			if seg.barflex {
				avail += seg.size

				seg = pbar(ratio, avail, max, b.msg)
				segs[i] = seg

				avail -= seg.size
			}
		}

		// hide segments if there isn't enough size
		for avail < 0 {
			rip := -1
			rii := -1
			for i, seg := range segs {
				if seg.hide {
					continue
				}
				if rii == -1 || (seg.priority < rip) {
					rii = i
					rip = seg.priority
				}
			}
			if rii == -1 {
				break
			}
			segs[rii].hide = true

			avail += segs[rii].size
			avail += padlen

			// expand or shrink bar
			for i, seg := range segs {
				if seg.hide {
					continue
				}
				if seg.barflex {
					avail += seg.size

					seg = pbar(ratio, avail, max, b.msg)
					segs[i] = seg

					avail -= seg.size
				}
			}
		}

		sort.SliceStable(segs, func(a, b int) bool {
			return segs[a].align < segs[b].align
		})

		str := ""

		for _, seg := range segs {
			if seg.hide {
				continue
			}
			if str != "" {
				str += pad
			}
			str += seg.text
		}

		//str = " " + str + " "

		if str != laststr {
			runecount := RuneCount(str)

			cm := lastrunecount
			if runecount > cm {
				cm = runecount
			}

			clearadd := cm - len(clearstr)
			if clearadd > 0 {
				for i := 0; i < clearadd; i++ {
					clearstr += " "
				}
			}
			backadd := cm - len(backstr)
			if backadd > 0 {
				for i := 0; i < backadd; i++ {
					backstr += "\b"
				}
			}

			fmt.Print(backstr[0:lastrunecount] + hidecursor + clearstr[0:lastrunecount] + backstr[0:lastrunecount] + str)

			lastrunecount = runecount
			laststr = str
		}

		//time.Sleep(sleeptime)
	}

	fmt.Println(showcursor)

	signal.Stop(b.winch)
	close(b.winch)
	close(b.done)
}

func title(text string) segment {
	if text == "" {
		return segment{
			hide: true,
		}
	}
	return segment{
		text:     text,
		size:     RuneCount(text),
		priority: 12,
		align:    0,
	}
}

func counts(at, max int) segment {
	text := ""

	if max == 0 {
		text = format_int(at)
	} else {
		text = fmt.Sprintf("%5s/%-5s", format_int(at), format_int(max))
	}

	return segment{
		text:     text,
		size:     RuneCount(text),
		priority: 4,
		align:    0,
	}
}

func percentage(ratio float64, max int) segment {
	if max == 0 {
		return segment{
			hide: true,
		}
	}

	text := fmt.Sprintf("%3s%%", fmt.Sprintf("%.0f", ratio*100.0))

	return segment{
		text:     text,
		size:     RuneCount(text),
		priority: 11,
		align:    0,
	}
}

func pbar(ratio float64, avail int, max int, msg string) segment {
	if max == 0 {
		return segment{
			hide: true,
		}
	}

	avail -= 2 // for start and end caps
	text := bright(white) + "▌" + reset()

	if avail < minbarsize {
		avail = minbarsize
	}

	fs := ratio * float64(avail)
	fp := fs - math.Trunc(fs)
	whole := int(fs)
	part := int(fp * (float64(len(blocks)) - 1))

	if len(msg) > avail {
		msg = msg[:avail]
	}

	for i := 0; i < avail; i++ {
		if i < len(msg) {
			if i < whole {
				text += brightbg(white, blue) + string(msg[i]) + reset()
			} else {
				text += fg(cyan) + string(msg[i]) + reset()
			}
		} else {
			if i < whole {
				text += fg(blue) + string(blocks[len(blocks)-1]) + reset()
			} else if i == whole {
				text += fg(blue) + string(blocks[part]) + reset()
			} else {
				if i == whole+1 {
					if fp < 0.33 {
						text += bright(white)
					} else if fp < 0.66 {
						// grey
					} else {
						text += bright(black)
					}
				} else {
					text += bright(white)
				}
				text += "۰"
				text += reset()
			}
		}
	}

	text += bright(white) + "▐" + reset()

	size := RuneCount(text)

	return segment{
		text:     text,
		size:     size,
		priority: 10,
		align:    1,
		barflex:  true,
	}
}

func itsbeen(start time.Time, now time.Time) segment {
	dur := now.Sub(start)
	text := "+" + format_dur(dur)

	return segment{
		text:     text,
		size:     RuneCount(text),
		priority: 7,
		align:    2,
	}
}

func esttotal(start time.Time, now time.Time, ratio float64, max int) segment {
	if max == 0 || ratio == 0 || ratio == 1 {
		return segment{
			hide: true,
		}
	}

	speed := now.Sub(start).Seconds() / ratio
	dur := time.Duration(speed * float64(time.Second))
	text := format_dur_rough(dur)

	return segment{
		text:     text,
		size:     RuneCount(text),
		priority: 8,
		align:    2,
	}
}

func remaining(start time.Time, now time.Time, ratio float64, max int) segment {
	if max == 0 || ratio == 0 || ratio == 1 {
		return segment{
			hide: true,
		}
	}

	speed := now.Sub(start).Seconds() / ratio
	dur := time.Duration(speed * (1 - ratio) * float64(time.Second))
	text := "-" + format_dur_rough(dur)

	return segment{
		text:     text,
		size:     RuneCount(text),
		priority: 9,
		align:    2,
	}
}

func avgspeed(dur time.Duration, delta int) segment {
	text := "---"
	if delta > 0 && dur > 0 {
		speed := float64(delta) / dur.Seconds()
		text = format_float(speed)
	}
	text = fmt.Sprintf("%5s/s avg", text)

	return segment{
		text:     text,
		size:     RuneCount(text),
		priority: 8,
		align:    0,
	}
}

func curspeed(dur time.Duration, delta int, ratio float64, max int) segment {
	if max != 0 && (ratio == 0 || ratio == 1) {
		return segment{
			hide: true,
		}
	}

	text := "---/s"
	if delta > 0 && dur > 0 {
		speed := float64(delta) / dur.Seconds()
		text = format_float(speed) + "/s"
	}
	text = fmt.Sprintf("%7s", text)

	return segment{
		text:     text,
		size:     RuneCount(text),
		priority: 8,
		align:    0,
	}
}

func timings(start time.Time, now time.Time, ratio float64, max int) segment {

	text := format_dur(now.Sub(start))

	if max == 0 || ratio == 0 || ratio == 1 {
		text = fmt.Sprintf("%20s", text)
		return segment{
			text:     text,
			size:     RuneCount(text),
			priority: 9,
			align:    2,
		}
	}

	speed := now.Sub(start).Seconds() / ratio

	totaldur := time.Duration(speed * float64(time.Second))
	totaltxt := format_dur(totaldur)

	remaindur := time.Duration(speed * (1 - ratio) * float64(time.Second))
	remaintxt := format_dur(remaindur)

	text += "/" + totaltxt + " eta " + remaintxt

	text = fmt.Sprintf("%20s", text)

	return segment{
		text:     text,
		size:     RuneCount(text),
		priority: 9,
		align:    2,
	}
}
func format_dur(d time.Duration) string {
	d = d.Round(time.Second)
	return d.String()
}

func format_dur_rough(d time.Duration) string {
	d = d.Round(time.Second)
	s := d / time.Second
	seconds := s % 60
	s /= 60
	minutes := s % 60
	s /= 60
	hours := s

	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
}

func format_int(i int) string {
	s := ""
	for _, ss := range suffixes {
		if i > 1000 {
			i /= 1000
			s = ss
		}
	}
	return strconv.Itoa(i) + s
}

func format_float(f float64) string {
	s := ""
	for _, ss := range suffixes {
		if f >= 1000 {
			f /= 1000
			s = ss
		}
	}
	if f < 1 {
		return fmt.Sprintf("%.2f%s", f, s)
	}
	if f < 10 {
		return fmt.Sprintf("%.1f%s", f, s)
	}
	return fmt.Sprintf("%.0f%s", f, s)
}
