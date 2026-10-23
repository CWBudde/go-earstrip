// Command naturalearth converts the admin-0 countries of Natural Earth into the world map of the demo's Showcase
// tab: web/public/world-110m.bin and world-50m.bin.
//
//	naturalearth [-o dir] [-src dir] [-bits n] [level ...]
//
// The levels are Natural Earth's scales, 110m and 50m by default. Without -src it downloads
// ne_<level>_admin_0_countries.geojson from github.com/nvkelso/natural-earth-vector; with -src it reads that file
// from the directory. Natural Earth (https://www.naturalearthdata.com) is in the public domain.
//
// The points are projected to Web Mercator, latitude clamped to ±85.0511° (where the map is square), and quantized
// to a grid of 1<<bits by 1<<bits (16 bits by default: 611 m at the equator, finer than either level), x east and
// y north. The triangulator then runs on what the GPU draws. Consecutive points that fall on the same grid point
// are merged, a ring's closing point (GeoJSON repeats the first) is dropped, and rings with fewer than three points
// are dropped (a polygon with its outer ring). The polygons are sorted west to east by the middle of their bounds,
// so that drawing them in order sweeps across the map.
//
// # Format
//
// All integers are little endian; uvarint and varint are those of encoding/binary (varint zig-zag encoded).
//
//	"ESNE"          magic
//	byte 1          version
//	byte bits       grid resolution: coordinates run from 0 to 1<<bits - 1
//	uvarint C       countries, then per country:
//	  uvarint len, len bytes   the name (Natural Earth's NAME), UTF-8
//	  byte                     MAPCOLOR7, 1 to 7: neighbours differ
//	uvarint P       polygons, then per polygon:
//	  uvarint       its country, 0 to C-1
//	  uvarint R     its rings, the outer ring first, then per ring:
//	    uvarint     its points
//	then per point of every ring of every polygon, in order:
//	  varint dx, varint dy     the change from the previous point (from 0, 0 for the first)
//
// internal/demo.DecodeWorld reads it in Go, web/src/showcase/map/world.ts in the browser.
package main

import (
	"cmp"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/cwbudde/go-earstrip/internal/demo"
)

const source = "https://raw.githubusercontent.com/nvkelso/natural-earth-vector/master/geojson/"

func main() {
	out := flag.String("o", "web/public", "the directory to write world-<level>.bin to")
	src := flag.String("src", "", "read the GeoJSON files from this directory instead of downloading them")
	bits := flag.Uint("bits", 16, "the grid's resolution in bits, 8 to 24")
	flag.Parse()
	levels := flag.Args()
	if len(levels) == 0 {
		levels = []string{"110m", "50m"}
	}
	if *bits < 8 || *bits > 24 {
		fmt.Fprintln(os.Stderr, "naturalearth: -bits: want 8 to 24")
		os.Exit(2)
	}
	for _, level := range levels {
		if err := convert(level, *src, *out, uint8(*bits)); err != nil {
			fmt.Fprintf(os.Stderr, "naturalearth: %s: %v\n", level, err)
			os.Exit(1)
		}
	}
}

// convert reads the level's GeoJSON and writes its world file.
func convert(level, src, out string, bits uint8) error {
	name := "ne_" + level + "_admin_0_countries.geojson"
	data, err := read(name, src)
	if err != nil {
		return err
	}
	w, err := world(data, bits)
	if err != nil {
		return err
	}
	b := w.Encode()
	file := filepath.Join(out, "world-"+level+".bin")
	if err := os.WriteFile(file, b, 0o644); err != nil {
		return err
	}
	rings, points := 0, 0
	for _, p := range w.Polygons {
		rings += len(p.Rings)
		for _, r := range p.Rings {
			points += len(r)
		}
	}
	fmt.Printf("%s: %d countries, %d polygons, %d rings, %d points, %d bytes\n",
		file, len(w.Countries), len(w.Polygons), rings, points, len(b))
	return nil
}

// read returns the file from src, or downloads it when src is empty.
func read(name, src string) ([]byte, error) {
	if src != "" {
		return os.ReadFile(filepath.Join(src, name))
	}
	client := http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Get(source + name)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", source+name, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// The part of a GeoJSON FeatureCollection of countries that the map keeps.
type collection struct {
	Features []struct {
		Properties struct {
			Name      string `json:"NAME"`
			MapColor7 int    `json:"MAPCOLOR7"`
		} `json:"properties"`
		Geometry struct {
			Type        string          `json:"type"`
			Coordinates json.RawMessage `json:"coordinates"`
		} `json:"geometry"`
	} `json:"features"`
}

// world projects and quantizes the countries of the GeoJSON.
func world(data []byte, bits uint8) (demo.World, error) {
	var c collection
	if err := json.Unmarshal(data, &c); err != nil {
		return demo.World{}, err
	}
	w := demo.World{Bits: bits}
	for _, f := range c.Features {
		var polygons [][][][]float64
		switch f.Geometry.Type {
		case "Polygon":
			var p [][][]float64
			if err := json.Unmarshal(f.Geometry.Coordinates, &p); err != nil {
				return w, fmt.Errorf("%s: %w", f.Properties.Name, err)
			}
			polygons = [][][][]float64{p}
		case "MultiPolygon":
			if err := json.Unmarshal(f.Geometry.Coordinates, &polygons); err != nil {
				return w, fmt.Errorf("%s: %w", f.Properties.Name, err)
			}
		default:
			return w, fmt.Errorf("%s: geometry %q, want Polygon or MultiPolygon", f.Properties.Name, f.Geometry.Type)
		}
		if f.Properties.MapColor7 < 1 || f.Properties.MapColor7 > 7 {
			return w, fmt.Errorf("%s: MAPCOLOR7 %d, want 1 to 7", f.Properties.Name, f.Properties.MapColor7)
		}
		country := len(w.Countries)
		kept := false
		for _, p := range polygons {
			var rings [][][2]int32
			for _, r := range p {
				q, err := quantize(r, bits)
				if err != nil {
					return w, fmt.Errorf("%s: %w", f.Properties.Name, err)
				}
				if len(q) < 3 {
					if len(rings) == 0 {
						break // the outer ring: the polygon is gone
					}
					continue
				}
				rings = append(rings, q)
			}
			if len(rings) > 0 {
				w.Polygons = append(w.Polygons, demo.WorldPolygon{Country: country, Rings: rings})
				kept = true
			}
		}
		if kept {
			w.Countries = append(w.Countries, demo.Country{Name: f.Properties.Name, MapColor: uint8(f.Properties.MapColor7)})
		}
	}
	if len(w.Polygons) == 0 {
		return w, errors.New("no polygons")
	}
	slices.SortStableFunc(w.Polygons, func(a, b demo.WorldPolygon) int { return cmp.Compare(middle(a), middle(b)) })
	return w, nil
}

// maxLatitude is where Web Mercator's y reaches ±π, which makes the map square.
var maxLatitude = math.Atan(math.Sinh(math.Pi))

// quantize projects a GeoJSON ring of [longitude, latitude] to the grid and merges the points that fall together.
func quantize(ring [][]float64, bits uint8) ([][2]int32, error) {
	top := float64(int32(1)<<bits - 1)
	var out [][2]int32
	for _, p := range ring {
		if len(p) < 2 || math.IsNaN(p[0]) || math.IsNaN(p[1]) {
			return nil, fmt.Errorf("point %v: want longitude, latitude", p)
		}
		lat := math.Max(-maxLatitude, math.Min(maxLatitude, p[1]*math.Pi/180))
		x := math.Max(-1, math.Min(1, p[0]/180))
		y := math.Log(math.Tan(math.Pi/4+lat/2)) / math.Pi
		q := [2]int32{int32(math.Round((x + 1) / 2 * top)), int32(math.Round((y + 1) / 2 * top))}
		if len(out) == 0 || q != out[len(out)-1] {
			out = append(out, q)
		}
	}
	for len(out) > 1 && out[0] == out[len(out)-1] {
		out = out[:len(out)-1]
	}
	return out, nil
}

// middle is the x in the middle of a polygon's outer ring's bounds.
func middle(p demo.WorldPolygon) int32 {
	lo, hi := int32(math.MaxInt32), int32(math.MinInt32)
	for _, q := range p.Rings[0] {
		lo, hi = min(lo, q[0]), max(hi, q[0])
	}
	return lo + (hi-lo)/2
}
