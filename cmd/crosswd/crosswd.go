package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
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
	modified    = false
	count       = 0
	cheat       = false
	notice      string
	filename    string
	normalStyle = style{termbox.ColorBlack, termbox.ColorWhite}
	selectStyle = style{termbox.ColorWhite, termbox.ColorBlue}
	editStyle   = style{termbox.ColorWhite, termbox.ColorRed}
	solvedStyle = style{termbox.ColorBlack, termbox.ColorGreen}
	cheatColor  = termbox.ColorYellow
)

func draw() {
	termbox.Clear(termbox.ColorDefault, termbox.ColorDefault)
	wordStyle := selectStyle
	if mode == editMode {
		wordStyle = editStyle
	}
	baseStyle := normalStyle
	if cw.Verify() {
		baseStyle = solvedStyle
	}
	start, end := cw.WordExtents(loc, dir)
	sz := cw.Size()

	// grid
	for y := 0; y < sz.Y; y++ {
		for x := 0; x < sz.X; x++ {
			c, _ := cw.At(crosswd.Coord{x, y})
			sty := baseStyle
			if x >= start.X && x <= end.X && y >= start.Y && y <= end.Y {
				sty = wordStyle
			}
			if cheat {
				sc, _ := cw.Solution.At(crosswd.Coord{x, y})
				if c != crosswd.Empty && c != sc {
					if sty == normalStyle {
						sty.bg = cheatColor
					} else {
						sty.fg = cheatColor
					}
				}
			}
			switch c {
			case crosswd.Empty:
				c = '_'
			case crosswd.Blank:
				c = ' '
				sty.fg, sty.bg = sty.bg, sty.fg
			}
			termbox.SetCell(x+1, y+1, rune(c), sty.fg, sty.bg)
		}
	}

	// puzzle info
	for y, s := range []string{cw.Title, cw.Author, cw.Copyright} {
		for x, r := range s {
			termbox.SetCell(sz.X+3+x, y+1, r, termbox.ColorDefault, termbox.ColorDefault)
		}
	}

	// status info
	var status []string
	if len(notice) > 0 {
		status = append(status, notice)
	}
	if modified {
		status = append(status, "[+]")
	}
	if count != 0 {
		status = append(status, strconv.Itoa(count))
	}
	for x, r := range strings.Join(status, " ") {
		termbox.SetCell(sz.X+3+x, sz.Y, r, termbox.ColorDefault, termbox.ColorDefault)
	}

	// clue
	id := cw.WordID(loc, dir)
	clue := cw.Clue(loc, dir)
	dirc := 'A'
	if dir == crosswd.Down {
		dirc = 'D'
	}
	wordLen := end.X - start.X + 1
	if dir == crosswd.Down {
		wordLen = end.Y - start.Y + 1
	}
	for x, r := range fmt.Sprintf("%d%c(%d): %s", id, dirc, wordLen, clue) {
		termbox.SetCell(x+1, sz.Y+2, r, termbox.ColorDefault, termbox.ColorDefault)
	}

	// notes
	width, _ := termbox.Size()
	var notes []string
	for _, para := range strings.Split(cw.Notes, "\n") {
		notes = append(notes, wrapText(strings.TrimSpace(para), width-2))
	}
	noteText := strings.Join(notes, "\n")
	x, y := 0, 0
	for _, r := range noteText {
		if r == '\n' {
			x = 0
			y++
		} else {
			termbox.SetCell(x+1, sz.Y+4+y, r, termbox.ColorDefault, termbox.ColorDefault)
			x++
		}
	}

	termbox.SetCursor(loc.X+1, loc.Y+1)
	termbox.Flush()
}

func handleKeyEvent(evt *termbox.Event) bool {
	notice = ""
	handled := true
	switch evt.Key {
	case termbox.KeyEsc:
		mode = normalMode
	case termbox.KeyTab:
		toggleDir()
	case termbox.KeySpace:
		countDo(func() {
			loc = cw.NextCell(loc, dir, true)
		})
	case termbox.KeyCtrlN:
		countDo(func() {
			loc = cw.NextWord(loc, dir)
		})
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
		if evt.Ch >= '0' && evt.Ch <= '9' {
			count = count*10 + (int(evt.Ch) - '0')
			resetCount = false
			break
		}
		switch evt.Ch {
		case 'i':
			mode = editMode
		case 'q':
			save()
			mode = quitMode
		case 'Q':
			mode = quitMode
		case 'h', 'j', 'k', 'l':
			countDo(func() {
				loc = cw.NextCell(loc, keyToDir(evt.Ch), true)
			})
		case 'G':
			if pos, ok := cw.WordStart(count); ok {
				loc = pos
			}
		case 'x':
			countDo(func() {
				set(crosswd.Empty)
				loc = cw.NextCell(loc, dir, true)
			})
		case 'X':
			countDo(func() {
				loc = cw.NextCell(loc, dir.Opposite(), true)
				set(crosswd.Empty)
			})
		case 'w':
			countDo(func() {
				loc = cw.NextWord(loc, dir)
			})
		case 'W':
			countDo(func() {
				loc = cw.NextWord(loc, dir.Opposite())
			})
		case 's':
			save()
		case 'C':
			if count == 1053 {
				cw.Solve()
				cheat = false
			} else {
				cheat = !cheat
			}
		}
	case editMode:
		switch evt.Key {
		case termbox.KeyDelete:
			set(crosswd.Empty)
			loc = cw.NextCell(loc, dir, true)
			return true
		case termbox.KeyBackspace, termbox.KeyBackspace2:
			loc = cw.NextCell(loc, dir.Opposite(), true)
			set(crosswd.Empty)
			return true
		}
		r := unicode.ToUpper(evt.Ch)
		if r >= 'A' && r <= 'Z' {
			set(byte(r))
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

func countDo(f func()) {
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
	filename = os.Args[1]
	in, err := os.Open(filename)
	if err != nil {
		log.Fatal(err)
	}
	defer in.Close()
	cw = crosswd.New()
	if err := cw.Read(in); err != nil {
		switch err.(type) {
		case crosswd.Warning:
			notice = fmt.Sprintf("[Note: %s]", err)
		default:
			log.Fatal(err)
		}
	}
	cw.Setup()
	loc = cw.NextCell(crosswd.Coord{-1, 0}, dir, true)

	if err := termbox.Init(); err != nil {
		log.Fatal(err)
	}
	defer termbox.Close()

	run()
}

func wrapText(text string, width int) string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return ""
	}
	out := words[0]
	left := width - len(out)
	for _, word := range words[1:] {
		if len(word)+1 > left {
			out += "\n" + word
			left = width - len(word)
		} else {
			out += " " + word
			left -= 1 + len(word)
		}
	}
	return out
}

func save() error {
	out, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer out.Close()
	err = cw.Write(out)
	if err == nil {
		modified = false
	}
	return err
}

func set(c byte) {
	if cw.Set(loc, c) {
		modified = true
	}
}
