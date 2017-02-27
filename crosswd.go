package crosswd

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"io/ioutil"
	"strings"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
)

// .puz constants
const (
	Magic      = "ACROSS&DOWN\x00"
	CksumMagic = "ICHEATED"
	Blank      = '.'
	Empty      = '-'
)

// Direction is a relative direction.
type Direction int

// Direction enumeration
const (
	NoDir Direction = iota
	Up
	Down
	Left
	Right
)

// Opposite returns the opposite direction of d.
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

// Delta returns a unit vector in the direction d.
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

// Coord represents a 2D coordinate.
type Coord struct{ X, Y int }

// Grid represents a 2D array stored contiguously.
type Grid struct {
	elts  []byte
	cells [][]byte
}

// NewGrid returns a new Grid of size width x height.
func NewGrid(width, height int) *Grid {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	g := &Grid{}
	g.elts = make([]byte, width*height)
	g.cells = make([][]byte, height)
	for i := range g.cells {
		g.cells[i] = g.elts[i*width : (i+1)*width : (i+1)*width]
	}
	return g
}

// Size returns the grid dimensions.
func (g *Grid) Size() Coord {
	if len(g.cells) == 0 {
		return Coord{}
	}
	return Coord{len(g.cells), len(g.cells[0])}
}

// Valid returns whether a point is within the grid range.
func (g *Grid) Valid(p Coord) bool {
	return p.Y >= 0 && p.Y < len(g.cells) && p.X >= 0 && p.X < len(g.cells[0])
}

// At returns the value at p if valid and whether p is valid.
func (g *Grid) At(p Coord) (byte, bool) {
	if !g.Valid(p) {
		return 0, false
	}
	return g.cells[p.Y][p.X], true
}

// Set stores c at point p if valid and returns whether g was updated
func (g *Grid) Set(p Coord, c byte) bool {
	if !g.Valid(p) {
		return false
	}
	g.cells[p.Y][p.X] = c
	return true
}

// Header holds .puz file header data
type Header struct {
	Cksum       uint16
	Magic       [len(Magic)]byte
	BaseCksum   uint16
	MaskedCksum [8]byte
	Version     [4]byte
	Unused      [2]byte
	Unknown     [2]byte
	Reserved    [12]byte
	Width       uint8
	Height      uint8
	NumClues    uint16
	BitMask1    [2]byte // normally set to 0x0001
	BitMask2    [2]byte // 0x0004 = scrambled
}

// Puzzle holds the state of a crossword puzzle
type Puzzle struct {
	Header Header
	*Grid
	solution  *Grid
	Title     string
	Author    string
	Copyright string
	Clues     []string
	Notes     string

	cellID  map[Coord]int
	idCell  map[int]Coord
	clueNum map[Direction]map[int]int
	enc     *encoding.Encoder
}

// New creates a Puzzle instance
func New() *Puzzle {
	return &Puzzle{
		cellID:  map[Coord]int{},
		idCell:  map[int]Coord{},
		clueNum: map[Direction]map[int]int{Right: {}, Down: {}},
		enc:     charmap.ISO8859_1.NewEncoder(),
	}
}

func (p *Puzzle) encodeString(str string) string {
	s, err := p.enc.String(str)
	if err != nil {
		return ""
	}
	return s
}

// Read reads crossword data in .puz format
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
	p.Notes = fields[3+p.Header.NumClues]

	if p.Cksum() != p.Header.Cksum {
		return errors.New("checksum does not match")
	}
	return nil
}

// NextCell returns the location one square from pos in dir direction.  If
// doSkip is true, skip blank squares and wrap around the grid, otherwise, pos
// is returned unmodified.
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

// WordExtent returns the position of the last cell of the word that includes
// pos in direction dir.
func (p *Puzzle) WordExtent(pos Coord, dir Direction) Coord {
	for {
		end := p.NextCell(pos, dir, false)
		if end == pos {
			return end
		}
		pos = end
	}
}

// WordExtents returns the positions of the first and last cells of the word
// that includes pos along direction dir.
func (p *Puzzle) WordExtents(pos Coord, dir Direction) (Coord, Coord) {
	start, end := p.WordExtent(pos, dir.Opposite()), p.WordExtent(pos, dir)
	if start.X > end.X || start.Y > end.Y {
		return end, start
	}
	return start, end
}

// NextWord returns the position of the first cell of the word after the one
// that includes pos in direction dir, wrapping if necessary.
func (p *Puzzle) NextWord(pos Coord, dir Direction) Coord {
	end := p.WordExtent(pos, dir)
	nextWordCell := p.NextCell(end, dir, true)
	nextWordStart, _ := p.WordExtents(nextWordCell, dir)
	return nextWordStart
}

// WordID returns the ID number of the word that includes pos along direction
// dir.
func (p *Puzzle) WordID(pos Coord, dir Direction) int {
	return p.cellID[p.WordExtent(pos, dir.Opposite())]
}

// WordStart returns the position of the cell with ID id if valid and whether
// id is valid.
func (p *Puzzle) WordStart(id int) (Coord, bool) {
	pos, ok := p.idCell[id]
	return pos, ok
}

// Clue returns the clue for the word that contains pos in direction dir, or an
// empty string if invalid.
func (p *Puzzle) Clue(pos Coord, dir Direction) string {
	id := p.WordID(pos, dir)
	clue := ""
	if num, ok := p.clueNum[dir][id]; ok && num >= 0 && num < len(p.Clues) {
		clue = p.Clues[num]
	}
	return clue
}

// Setup initializes the puzzle.
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
					cid, seen := p.cellID[pos]
					if !seen {
						p.cellID[pos] = id
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

// Verify returns whether the working grid matches the solition.
func (p *Puzzle) Verify() bool {
	return bytes.Equal(p.elts, p.solution.elts)
}

// BaseCksum calculates base checksum
func (p *Puzzle) BaseCksum() uint16 {
	buf := bytes.NewBuffer([]byte{})
	binary.Write(buf, binary.LittleEndian, p.Header.Width)
	binary.Write(buf, binary.LittleEndian, p.Header.Height)
	binary.Write(buf, binary.LittleEndian, p.Header.NumClues)
	binary.Write(buf, binary.LittleEndian, p.Header.BitMask1)
	binary.Write(buf, binary.LittleEndian, p.Header.BitMask2)
	return calcCksum(buf.Bytes(), 0)
}

// TextCksum calculates checksum of text fields
func (p *Puzzle) TextCksum(cksum uint16) uint16 {
	cksum = calcCksum([]byte(p.encodeString(p.Title+"\x00")), cksum)
	cksum = calcCksum([]byte(p.encodeString(p.Author+"\x00")), cksum)
	cksum = calcCksum([]byte(p.encodeString(p.Copyright+"\x00")), cksum)
	for _, clue := range p.Clues {
		cksum = calcCksum([]byte(p.encodeString(clue)), cksum)
	}
	cksum = calcCksum([]byte(p.encodeString(p.Notes+"\x00")), cksum)
	return cksum
}

// Cksum calculates full checksum
func (p *Puzzle) Cksum() uint16 {
	cksum := p.BaseCksum()
	cksum = calcCksum(p.solution.elts, cksum)
	cksum = calcCksum(p.elts, cksum)
	if string(p.Header.Version[:]) >= "1.3" {
		cksum = p.TextCksum(cksum)
	}
	return cksum
}

// MaskedCksum calculates masked checksum
func (p *Puzzle) MaskedCksum() [8]byte {
	cksum := [8]byte{}
	for i, cs := range []uint16{
		p.BaseCksum(),
		calcCksum(p.solution.elts, 0),
		calcCksum(p.elts, 0),
		p.TextCksum(0),
	} {
		cksum[i] = byte(cs) ^ CksumMagic[i]
		cksum[i+4] = byte(cs>>8) ^ CksumMagic[i+4]
	}
	return cksum
}

func calcCksum(data []byte, cksum uint16) uint16 {
	for _, b := range data {
		cksum = (cksum >> 1) | ((cksum & 1) << 15)
		cksum += uint16(b)
	}
	return cksum
}
