// Package japanmap draws a map of Japan in the terminal using Braille characters.
//
// Each terminal cell holds one Braille character, which has a 2x4 grid of dots.
// Drawing on dots instead of cells gives 8 times the resolution.
package japanmap

import (
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// The area shown on the map (longitude, latitude).
// Main islands plus Okinawa; the Sakishima Islands are cut off to keep the scale large.
const (
	viewMinLon, viewMaxLon = 127.0, 146.5
	viewMinLat, viewMaxLat = 26.0, 45.8
)

// Mark is a single character drawn on top of the map, e.g. an intensity label.
type Mark struct {
	Lon, Lat float64
	Text     string
	Style    lipgloss.Style
}

// Map is a map sized for a given number of terminal cells.
type Map struct {
	cols, rows int
	scale      float64 // dots per degree of longitude
	offX, offY float64
	coast      []uint8 // Braille bits per cell for the coastline
}

// lonFactor shrinks longitude so shapes are not stretched east-west
// (a degree of longitude is shorter than a degree of latitude at Japan's latitude).
var lonFactor = math.Cos(36 * math.Pi / 180)

func New(cols, rows int) *Map {
	m := &Map{cols: cols, rows: rows, coast: make([]uint8, cols*rows)}

	// Braille dots are roughly square in a typical terminal font
	// (a cell is about twice as tall as wide, and holds 2x4 dots).
	dotsW, dotsH := float64(cols*2), float64(rows*4)
	spanX := (viewMaxLon - viewMinLon) * lonFactor
	spanY := viewMaxLat - viewMinLat
	m.scale = math.Min(dotsW/spanX, dotsH/spanY)
	// Center the map in the canvas.
	m.offX = (dotsW - spanX*m.scale) / 2
	m.offY = (dotsH - spanY*m.scale) / 2

	for _, line := range coastlines {
		for i := 1; i < len(line); i++ {
			x0, y0 := m.project(float64(line[i-1][0]), float64(line[i-1][1]))
			x1, y1 := m.project(float64(line[i][0]), float64(line[i][1]))
			m.line(m.coast, x0, y0, x1, y1)
		}
	}
	return m
}

// project converts (lon, lat) to dot coordinates. North is up.
func (m *Map) project(lon, lat float64) (x, y int) {
	x = int(m.offX + (lon-viewMinLon)*lonFactor*m.scale)
	y = int(m.offY + (viewMaxLat-lat)*m.scale)
	return x, y
}

// Cell returns the terminal cell (column, row) for a position.
func (m *Map) Cell(lon, lat float64) (col, row int) {
	x, y := m.project(lon, lat)
	return x / 2, y / 4
}

// DotsPerDegree is used to size things that should scale with the map, such as ripples.
func (m *Map) DotsPerDegree() float64 { return m.scale }

// brailleBit[y][x] is the bit for dot (x, y) inside a cell.
var brailleBit = [4][2]uint8{
	{0x01, 0x08},
	{0x02, 0x10},
	{0x04, 0x20},
	{0x40, 0x80},
}

func (m *Map) set(layer []uint8, x, y int) {
	if x < 0 || y < 0 || x >= m.cols*2 || y >= m.rows*4 {
		return
	}
	layer[(y/4)*m.cols+x/2] |= brailleBit[y%4][x%2]
}

// line draws with Bresenham's algorithm.
func (m *Map) line(layer []uint8, x0, y0, x1, y1 int) {
	dx, dy := abs(x1-x0), -abs(y1-y0)
	sx, sy := sign(x1-x0), sign(y1-y0)
	err := dx + dy
	for {
		m.set(layer, x0, y0)
		if x0 == x1 && y0 == y1 {
			return
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

// Ring is a circle in dot coordinates, drawn on top of the coastline.
type Ring struct {
	Lon, Lat float64
	Radius   float64 // in dots
	Style    lipgloss.Style
}

// Render draws the coastline, then rings, then marks.
func (m *Map) Render(coastStyle lipgloss.Style, rings []Ring, marks []Mark) string {
	type cell struct {
		bits  uint8
		style *lipgloss.Style
		text  string
	}
	cells := make([]cell, len(m.coast))
	for i, b := range m.coast {
		cells[i] = cell{bits: b, style: &coastStyle}
	}

	// No real ripple comes near this. A bogus radius (say, from Inf) would otherwise
	// make the loop below run practically forever.
	maxRadius := 2 * math.Hypot(float64(m.cols*2), float64(m.rows*4))
	for i := range rings {
		r := &rings[i]
		if !(r.Radius >= 0 && r.Radius <= maxRadius) { // also catches NaN
			continue
		}
		layer := make([]uint8, len(m.coast))
		cx, cy := m.project(r.Lon, r.Lat)
		// Enough steps that neighboring points touch.
		steps := max(16, int(r.Radius*2*math.Pi))
		for s := 0; s < steps; s++ {
			a := 2 * math.Pi * float64(s) / float64(steps)
			m.set(layer, cx+int(r.Radius*math.Cos(a)), cy+int(r.Radius*math.Sin(a)))
		}
		for j, b := range layer {
			if b != 0 {
				cells[j].bits |= b
				cells[j].style = &r.Style
			}
		}
	}

	for i := range marks {
		mk := &marks[i]
		col, row := m.Cell(mk.Lon, mk.Lat)
		for k, ch := range []rune(mk.Text) {
			c := col + k
			if c < 0 || row < 0 || c >= m.cols || row >= m.rows {
				continue
			}
			cells[row*m.cols+c] = cell{text: string(ch), style: &mk.Style}
		}
	}

	// Group runs of cells with the same style so we don't emit
	// escape codes for every single character.
	var b strings.Builder
	for row := 0; row < m.rows; row++ {
		var run strings.Builder
		var runStyle *lipgloss.Style
		flush := func() {
			if run.Len() == 0 {
				return
			}
			if runStyle == nil {
				b.WriteString(run.String())
			} else {
				b.WriteString(runStyle.Render(run.String()))
			}
			run.Reset()
		}
		for col := 0; col < m.cols; col++ {
			c := cells[row*m.cols+col]
			ch := c.text
			style := c.style
			if ch == "" {
				if c.bits == 0 {
					ch, style = " ", nil
				} else {
					ch = string(rune(0x2800 + int(c.bits)))
				}
			}
			if style != runStyle {
				flush()
				runStyle = style
			}
			run.WriteString(ch)
		}
		flush()
		if row < m.rows-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func sign(x int) int {
	switch {
	case x > 0:
		return 1
	case x < 0:
		return -1
	}
	return 0
}
