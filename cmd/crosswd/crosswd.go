package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"unicode"

	"github.com/ianremmler/crosswd"
	"github.com/nsf/termbox-go"
)

type runMode int

const (
	normalMode runMode = iota
	editMode
	quitMode
)

type style struct {
	fg, bg termbox.Attribute
}

var (
	cw          *crosswd.Puzzle
	mode        = normalMode
	loc         crosswd.Coord
	dir         = crosswd.Right
	count       = 0
	normalStyle = style{termbox.ColorWhite, termbox.ColorBlack}
	selectStyle = style{termbox.ColorWhite, termbox.ColorBlue}
	editStyle   = style{termbox.ColorWhite, termbox.ColorRed}
	solvedStyle = style{termbox.ColorWhite, termbox.ColorGreen}
)

func draw() {
	termbox.Clear(normalStyle.fg, normalStyle.bg)
	start, end := cw.WordExtents(loc, dir)
	wordStyle := selectStyle
	if mode == editMode {
		wordStyle = editStyle
	}
	baseStyle := normalStyle
	if cw.Verify() {
		baseStyle = solvedStyle
	}
	sz := cw.Size()
	for y := 0; y < sz.Y; y++ {
		for x := 0; x < sz.X; x++ {
			c, ok := cw.At(crosswd.Coord{x, y})
			if !ok {
				c = crosswd.Blank
			}
			switch c {
			case crosswd.Empty:
				c = '_'
			case crosswd.Blank:
				c = '#'
			}
			sty := baseStyle
			if x >= start.X && x <= end.X && y >= start.Y && y <= end.Y {
				sty = wordStyle
			}
			termbox.SetCell(x, y, rune(c), sty.fg, sty.bg)
		}
	}
	id := cw.WordId(loc, dir)
	clue := cw.Clue(loc, dir)
	dirc := 'A'
	if dir == crosswd.Down {
		dirc = 'D'
	}
	for x, r := range fmt.Sprintf("%d%c: %s", id, dirc, clue) {
		termbox.SetCell(x, sz.Y+1, r, normalStyle.fg, normalStyle.bg)
	}
	if count > 0 {
		for x, r := range strconv.Itoa(count) {
			termbox.SetCell(sz.X+1+x, 0, r, normalStyle.fg, normalStyle.bg)
		}
	}
	termbox.SetCursor(loc.X, loc.Y)
	termbox.Flush()
}

func handleKeyEvent(evt *termbox.Event) bool {
	handled := true
	switch evt.Key {
	case termbox.KeyEsc:
		mode = normalMode
	case termbox.KeyTab:
		toggleDir()
	case termbox.KeyCtrlN:
		do(func() { loc = cw.NextWord(loc, dir) })
	case termbox.KeyCtrlP:
		loc = cw.NextWord(loc, dir.Opposite())
	default:
		handled = false
	}
	if handled {
		return true
	}
	resetCount := true
	switch mode {
	case normalMode:
		switch evt.Ch {
		case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
			count = count*10 + (int(evt.Ch) - '0')
			resetCount = false
		case 'i':
			mode = editMode
		case 'q':
			mode = quitMode
		case 'h', 'j', 'k', 'l':
			do(func() { loc = cw.NextCell(loc, keyToDir(evt.Ch), true) })
		case 'G':
			if pos, ok := cw.WordStart(count); ok {
				loc = pos
			}
		case 'x':
			do(func() {
				cw.Set(loc, crosswd.Empty)
				loc = cw.NextCell(loc, dir, true)
			})
		case 'X':
			do(func() {
				loc = cw.NextCell(loc, dir.Opposite(), true)
				cw.Set(loc, crosswd.Empty)
			})
		case 'w':
			loc = cw.NextWord(loc, dir)
		case 'W':
			loc = cw.NextWord(loc, dir.Opposite())
		}
	case editMode:
		switch evt.Key {
		case termbox.KeyDelete:
			cw.Set(loc, crosswd.Empty)
			loc = cw.NextCell(loc, dir, true)
			return true
		case termbox.KeyBackspace, termbox.KeyBackspace2:
			loc = cw.NextCell(loc, dir.Opposite(), true)
			cw.Set(loc, crosswd.Empty)
			return true
		}
		r := unicode.ToUpper(evt.Ch)
		if r >= 'A' && r <= 'Z' {
			cw.Set(loc, byte(r))
			loc = cw.NextCell(loc, dir, true)
		}
	}
	return resetCount
}

func keyToDir(key rune) crosswd.Direction {
	switch key {
	case 'k':
		return crosswd.Up
	case 'j':
		return crosswd.Down
	case 'h':
		return crosswd.Left
	case 'l':
		return crosswd.Right
	}
	return crosswd.NoDir
}

func toggleDir() {
	if dir == crosswd.Right {
		dir = crosswd.Down
	} else {
		dir = crosswd.Right
	}
}

func do(f func()) {
	if count == 0 {
		count = 1
	}
	for i := 0; i < count; i++ {
		f()
	}
}

func run() {
	for {
		draw()
		switch evt := termbox.PollEvent(); evt.Type {
		case termbox.EventKey:
			if handleKeyEvent(&evt) {
				count = 0
			}
		}
		if mode == quitMode {
			return
		}
	}
}

func main() {
	log.SetFlags(0)
	log.SetPrefix("crosswd: ")

	if len(os.Args) < 2 {
		log.Fatal("usage: crosswd crossword.puz")
	}
	file, err := os.Open(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	if err := termbox.Init(); err != nil {
		log.Fatal(err)
	}
	defer termbox.Close()

	cw = crosswd.New()
	if err := cw.Read(file); err != nil {
		log.Fatal(err)
	}
	cw.Setup()
	loc = cw.NextCell(crosswd.Coord{-1, 0}, dir, true)
	run()
}
