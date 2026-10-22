package cases

// Decode turns fuzz bytes into a polygon with holes and small integer coordinates (within int16): byte 0 picks the
// number of rings (1 to 4) and byte 1 the scale (1, 2, 16 or 256); then each ring is a count byte (0 to 23
// points) followed by one signed byte per coordinate. Missing bytes read as zero, so every input decodes.
func Decode(data []byte) Rings {
	pos := 0
	next := func() byte {
		if pos >= len(data) {
			return 0
		}
		b := data[pos]
		pos++
		return b
	}
	count := 1 + int(next()%4)
	scale := [...]int64{1, 2, 16, 256}[next()%4]
	rings := make(Rings, count)
	for r := range rings {
		n := int(next() % 24)
		ring := make(Ring, n)
		for i := range ring {
			x, y := int64(int8(next())), int64(int8(next()))
			ring[i] = [2]int64{x * scale, y * scale}
		}
		rings[r] = ring
	}
	return rings
}

// Encode is the inverse of Decode for rings it can represent (coordinates multiples of scale within int8 range).
func Encode(rings Rings, scale int64) []byte {
	sc := map[int64]byte{1: 0, 2: 1, 16: 2, 256: 3}[scale]
	out := []byte{byte(len(rings) - 1), sc}
	for _, r := range rings {
		out = append(out, byte(len(r)))
		for _, p := range r {
			out = append(out, byte(int8(p[0]/scale)), byte(int8(p[1]/scale)))
		}
	}
	return out
}
