package demo

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// The world map of the Showcase: the countries of Natural Earth (public domain), projected to Web Mercator and
// quantized, as cmd/naturalearth writes them to web/public/world-<level>.bin. The format is documented there.

// worldMagic starts every world file; worldVersion follows it.
const (
	worldMagic   = "ESNE"
	worldVersion = 1
)

// Country is one admin-0 country of the world map.
type Country struct {
	Name string
	// MapColor is Natural Earth's MAPCOLOR7, 1 to 7: no two neighbours share one.
	MapColor uint8
}

// WorldPolygon is one polygon of a country: ring 0 the outer ring, the others holes, points on the grid.
type WorldPolygon struct {
	Country int
	Rings   [][][2]int32
}

// World is a level of the world map.
type World struct {
	// Bits is the grid's resolution: coordinates run from 0 to 1<<Bits - 1, x east and y north.
	Bits      uint8
	Countries []Country
	Polygons  []WorldPolygon
}

// Encode writes the world in the format of cmd/naturalearth.
func (w *World) Encode() []byte {
	b := append([]byte(worldMagic), worldVersion, w.Bits)
	b = binary.AppendUvarint(b, uint64(len(w.Countries)))
	for _, c := range w.Countries {
		b = binary.AppendUvarint(b, uint64(len(c.Name)))
		b = append(b, c.Name...)
		b = append(b, c.MapColor)
	}
	b = binary.AppendUvarint(b, uint64(len(w.Polygons)))
	for _, p := range w.Polygons {
		b = binary.AppendUvarint(b, uint64(p.Country))
		b = binary.AppendUvarint(b, uint64(len(p.Rings)))
		for _, r := range p.Rings {
			b = binary.AppendUvarint(b, uint64(len(r)))
		}
	}
	var prev [2]int32
	for _, p := range w.Polygons {
		for _, r := range p.Rings {
			for _, q := range r {
				b = binary.AppendVarint(b, int64(q[0]-prev[0]))
				b = binary.AppendVarint(b, int64(q[1]-prev[1]))
				prev = q
			}
		}
	}
	return b
}

// worldReader reads the varints of a world file and keeps the first error.
type worldReader struct {
	r   *bytes.Reader
	err error
}

func (r *worldReader) uvarint(limit uint64, what string) int {
	if r.err != nil {
		return 0
	}
	v, err := binary.ReadUvarint(r.r)
	if err == nil && v > limit {
		err = fmt.Errorf("%d, want at most %d", v, limit)
	}
	if err != nil {
		r.err = fmt.Errorf("%s: %w", what, err)
		return 0
	}
	return int(v)
}

func (r *worldReader) varint(what string) int32 {
	if r.err != nil {
		return 0
	}
	v, err := binary.ReadVarint(r.r)
	if err != nil {
		r.err = fmt.Errorf("%s: %w", what, err)
	}
	return int32(v)
}

func (r *worldReader) byte(what string) byte {
	if r.err != nil {
		return 0
	}
	c, err := r.r.ReadByte()
	if err != nil {
		r.err = fmt.Errorf("%s: %w", what, err)
	}
	return c
}

// DecodeWorld reads a world file. It checks the counts against the file's length, so a broken file gives an error,
// not a huge allocation, and checks that every point lies on the grid.
func DecodeWorld(data []byte) (World, error) {
	var w World
	if len(data) < len(worldMagic)+2 || string(data[:len(worldMagic)]) != worldMagic {
		return w, errors.New("not a world file")
	}
	if v := data[len(worldMagic)]; v != worldVersion {
		return w, fmt.Errorf("version %d, want %d", v, worldVersion)
	}
	w.Bits = data[len(worldMagic)+1]
	if w.Bits == 0 || w.Bits > 24 {
		return w, fmt.Errorf("%d bits, want 1 to 24", w.Bits)
	}
	r := worldReader{r: bytes.NewReader(data[len(worldMagic)+2:])}
	// Every item takes at least a byte, which bounds each count by what is left.
	left := func() uint64 { return uint64(r.r.Len()) }

	w.Countries = make([]Country, r.uvarint(left(), "countries"))
	for i := range w.Countries {
		name := make([]byte, r.uvarint(left(), "name length"))
		if r.err == nil {
			if _, err := io.ReadFull(r.r, name); err != nil {
				r.err = fmt.Errorf("name: %w", err)
			}
		}
		w.Countries[i] = Country{Name: string(name), MapColor: r.byte("map color")}
	}
	w.Polygons = make([]WorldPolygon, r.uvarint(left(), "polygons"))
	for i := range w.Polygons {
		p := &w.Polygons[i]
		p.Country = r.uvarint(uint64(max(len(w.Countries)-1, 0)), "country")
		p.Rings = make([][][2]int32, r.uvarint(left(), "rings"))
		for k := range p.Rings {
			// Two bytes per point at least.
			p.Rings[k] = make([][2]int32, r.uvarint(left()/2, "points"))
		}
	}
	var prev [2]int32
	limit := int32(1)<<w.Bits - 1
	for _, p := range w.Polygons {
		for _, ring := range p.Rings {
			for k := range ring {
				q := [2]int32{prev[0] + r.varint("x"), prev[1] + r.varint("y")}
				if r.err == nil && (q[0] < 0 || q[0] > limit || q[1] < 0 || q[1] > limit) {
					r.err = fmt.Errorf("point %v off the grid of %d bits", q, w.Bits)
				}
				ring[k] = q
				prev = q
			}
		}
	}
	if r.err == nil && r.r.Len() > 0 {
		r.err = fmt.Errorf("%d bytes after the points", r.r.Len())
	}
	if r.err != nil {
		return World{}, r.err
	}
	return w, nil
}

// Batch returns the world's polygons as one batch for RunMany.
func (w *World) Batch() Batch {
	var b Batch
	for _, p := range w.Polygons {
		b.PolygonStarts = append(b.PolygonStarts, len(b.RingStarts))
		for _, r := range p.Rings {
			b.RingStarts = append(b.RingStarts, len(b.Coords)/2)
			for _, q := range r {
				b.Coords = append(b.Coords, float64(q[0]), float64(q[1]))
			}
		}
	}
	return b
}
