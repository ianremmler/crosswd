package crosswd

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"io/ioutil"
	"strings"

	"golang.org/x/text/encoding/charmap"
)

const (
	Magic = "ACROSS&DOWN\x00"
	Blank = '.'
	Empty = '-'
)

type Direction int

const (
	NoDir Direction = iota
	Up
	Down
	Left
	Right
)

func (d Direction) Opposite() Direction {
	switch d {
	case Left:
		return Right
	case Right:
		return Left
	case Up:
		return Down
	case Down:
		return Up
	}
	return NoDir
}

func (d Direction) Delta() Coord {
	switch d {
	case Up:
		return Coord{0, -1}
	case Down:
		return Coord{0, 1}
	case Left:
		return Coord{-1, 0}
	case Right:
		return Coord{1, 0}
	}
	return Coord{}
}

type Coord struct{ X, Y int }

type Grid struct {
	elts  []byte
	cells [][]byte
}

func NewGrid(w, h int) *Grid {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	g := &Grid{}
	g.elts = make([]byte, w*h)
	g.cells = make([][]byte, h)
	for i := range g.cells {
		g.cells[i] = g.elts[i*w : (i+1)*w : (i+1)*w]
	}
	return g
}

func (g *Grid) Size() Coord {
	if len(g.cells) == 0 {
		return Coord{}
	}
	return Coord{len(g.cells), len(g.cells[0])}
}

func (g *Grid) Valid(p Coord) bool {
	return p.Y >= 0 && p.Y < len(g.cells) && p.X >= 0 && p.X < len(g.cells[0])
}

func (g *Grid) At(p Coord) (byte, bool) {
	if !g.Valid(p) {
		return 0, false
	}
	return g.cells[p.Y][p.X], true
}

func (g *Grid) Set(p Coord, c byte) bool {
	if !g.Valid(p) {
		return false
	}
	g.cells[p.Y][p.X] = c
	return true
}

type Header struct {
	Cksum        uint16
	Magic        [len(Magic)]byte
	BaseCksum    uint16
	MaskedCksums [4]uint16
	Version      [4]byte
	Unused       [2]byte
	Unknown      [2]byte
	Reserved     [12]byte
	Width        uint8
	Height       uint8
	NumClues     uint16
	Bitmask1     [2]byte // normally set to 0x0001
	Bitmask2     [2]byte // 0x0004 = scrambled
}

func updateCksum(data []byte, cksum uint16) uint16 {
	for _, b := range data {
		cksum = (cksum >> 1) | ((cksum & 1) << 15)
		cksum += uint16(b)
	}
	return cksum
}

type Puzzle struct {
	Header Header
	*Grid
	solution  *Grid
	Title     string
	Author    string
	Copyright string
	Clues     []string
	Notes     []string

	cellId  map[Coord]int
	idCell  map[int]Coord
	clueNum map[Direction]map[int]int
}

func New() *Puzzle {
	return &Puzzle{
		cellId:  map[Coord]int{},
		idCell:  map[int]Coord{},
		clueNum: map[Direction]map[int]int{Right: {}, Down: {}},
	}
}

func (p *Puzzle) Read(in io.Reader) error {
	if err := binary.Read(in, binary.LittleEndian, &p.Header); err != nil {
		return err
	}
	if string(p.Header.Magic[:]) != Magic {
		return errors.New("invalid magic")
	}
	w, h := int(p.Header.Width), int(p.Header.Height)
	p.solution = NewGrid(w, h)
	if _, err := io.ReadFull(in, p.solution.elts); err != nil {
		return err
	}
	p.Grid = NewGrid(w, h)
	if _, err := io.ReadFull(in, p.elts); err != nil {
		return err
	}
	rest, err := ioutil.ReadAll(in)
	if err != nil {
		return err
	}
	rest, err = charmap.ISO8859_1.NewDecoder().Bytes(rest)
	if err != nil {
		return err
	}
	fields := make([]string, 4+p.Header.NumClues)
	copy(fields, strings.Split(string(rest), "\x00"))
	p.Title = fields[0]
	p.Author = fields[1]
	p.Copyright = fields[2]
	p.Clues = fields[3 : 3+p.Header.NumClues]
	p.Notes = fields[3+p.Header.NumClues:]
	return nil
}

func (p *Puzzle) NextCell(pos Coord, dir Direction, doSkip bool) Coord {
	dlt := dir.Delta()
	if dlt == (Coord{}) {
		return pos
	}

	size := p.Size()
	nextPos := pos
	for {
		nextPos.X += dlt.X
		nextPos.Y += dlt.Y
		if doSkip {
			x := nextPos.X + ((nextPos.Y+size.Y)/size.Y - 1)
			y := nextPos.Y + ((nextPos.X+size.X)/size.X - 1)
			nextPos.X = (x%size.X + size.X) % size.X
			nextPos.Y = (y%size.Y + size.Y) % size.Y
		}
		c, ok := p.At(nextPos)
		if !ok {
			return pos
		}
		if c == Blank {
			if !doSkip {
				return pos
			}
		} else {
			return nextPos
		}
	}
}

func (p *Puzzle) WordExtent(pos Coord, dir Direction) Coord {
	for {
		end := p.NextCell(pos, dir, false)
		if end == pos {
			return end
		}
		pos = end
	}
}

func (p *Puzzle) WordExtents(pos Coord, dir Direction) (Coord, Coord) {
	start, end := p.WordExtent(pos, dir.Opposite()), p.WordExtent(pos, dir)
	if start.X > end.X || start.Y > end.Y {
		return end, start
	}
	return start, end
}

func (p *Puzzle) NextWord(pos Coord, dir Direction) Coord {
	end := p.WordExtent(pos, dir)
	nextWordCell := p.NextCell(end, dir, true)
	nextWordStart, _ := p.WordExtents(nextWordCell, dir)
	return nextWordStart
}

func (p *Puzzle) WordId(pos Coord, dir Direction) int {
	return p.cellId[p.WordExtent(pos, dir.Opposite())]
}

func (p *Puzzle) WordStart(id int) (Coord, bool) {
	pos, ok := p.idCell[id]
	return pos, ok
}

func (p *Puzzle) Clue(pos Coord, dir Direction) string {
	id := p.WordId(pos, dir)
	clue := ""
	if num, ok := p.clueNum[dir][id]; ok && num >= 0 && num < len(p.Clues) {
		clue = p.Clues[num]
	}
	return clue
}

func (p *Puzzle) Setup() {
	id := 1
	cnum := 0
	sz := p.Size()
	for y := 0; y < sz.Y; y++ {
		for x := 0; x < sz.X; x++ {
			pos := Coord{x, y}
			if c, ok := p.At(pos); !ok || c == Blank {
				continue
			}
			for _, dir := range []Direction{Right, Down} {
				start, end := p.WordExtents(pos, dir)
				if start == pos && end != pos {
					cid, seen := p.cellId[pos]
					if !seen {
						p.cellId[pos] = id
						p.idCell[id] = pos
						cid = id
						id++
					}
					p.clueNum[dir][cid] = cnum
					cnum++
				}
			}
		}
	}
}

func (p *Puzzle) Verify() bool {
	return bytes.Equal(p.elts, p.solution.elts)
}
