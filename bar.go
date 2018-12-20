package progress

import (
	"fmt"
	"math"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/karrick/gows"
)

var blocks = []rune{' ', '▏', '▎', '▍', '▌', '▋', '▊', '▉', '█'}

//var blocks = []rune{' ', '▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

var pad = " │ "
var padlen = RuneCount(pad)

var suffixes = []string{"K", "M", "G", "T", "P"}

const (
	minbarsize = 5
)

type Bar struct {
	at  int64
	max int64
	msg string

	finish chan struct{}
	done   chan struct{}
	winch  chan os.Signal
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
	}

	signal.Notify(b.winch, syscall.SIGWINCH)

	go b.draw()
	return &b
}

func (b *Bar) Next() {
	atomic.AddInt64(&b.at, 1)
}

func (b *Bar) Add(delta int) {
	atomic.AddInt64(&b.at, int64(delta))
}

func (b *Bar) Done() {
	close(b.finish)
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
	clearstr := ""
	backstr := ""

	done := false

	sample_at := 0
	sample_time := start

	for !done {
		select {
		case _ = <-b.finish:
			done = true
		case _ = <-b.winch:
			size = getsize()
		default:
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

		ratio := float64(at) / float64(max)
		if ratio > 1 {
			ratio = 1
		}

		segs := []segment{
			title(b.msg),
			counts(at, max),
			speed(now.Sub(start), at),
			percentage(ratio),
			pbar(ratio, 0),
			itsbeen(start, realnow),
			esttotal(start, now, ratio),
			remaining(start, now, ratio),
		}

		// the actual progress bar has variable size, so lets
		// measure the other segments first to see how much to
		// expand it.

		// 1 space at the start, 1 space at the end, the cursor, and a space after that so ^C looks good
		fixed_size := size - 4
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

				seg = pbar(ratio, avail)
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

					seg = pbar(ratio, avail)
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

		str = " " + str + " "

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

		fmt.Print(backstr[0:lastrunecount] + clearstr[0:lastrunecount] + backstr[0:lastrunecount] + str)

		lastrunecount = runecount

		time.Sleep(time.Millisecond * 50)
	}

	fmt.Println()

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
	ms := format_int(max)
	msl := strconv.Itoa(len(ms))
	text := fmt.Sprintf("%"+msl+"s / %s", format_int(at), ms)

	return segment{
		text:     text,
		size:     RuneCount(text),
		priority: 4,
		align:    0,
	}
}

func percentage(ratio float64) segment {
	text := format_float(ratio*100.0) + " %"

	return segment{
		text:     text,
		size:     RuneCount(text),
		priority: 11,
		align:    2,
	}
}

func pbar(ratio float64, avail int) segment {
	text := ""

	if avail < minbarsize {
		avail = minbarsize
	}

	fs := ratio * float64(avail)
	whole := int(fs)
	part := int((fs - math.Trunc(fs)) * (float64(len(blocks)) - 1))

	for i := 0; i < avail; i++ {
		if i < whole {
			text += string(blocks[len(blocks)-1])
		} else if i == whole {
			text += string(blocks[part])
		} else {
			text += "۰"
		}
	}

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

func esttotal(start time.Time, now time.Time, ratio float64) segment {
	if ratio == 0 || ratio == 1 {
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

func remaining(start time.Time, now time.Time, ratio float64) segment {
	if ratio == 0 || ratio == 1 {
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

func speed(dur time.Duration, delta int) segment {
	text := "---/s"
	if dur > 0 {
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
