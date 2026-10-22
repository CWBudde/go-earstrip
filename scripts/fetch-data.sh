#!/usr/bin/env bash
# Downloads the benchmark polygons of earstrip's bench-data-1 release into data/ and checks them against
# scripts/SHA256SUMS. The files are OpenStreetMap data under the ODbL, see https://github.com/GLMap/earstrip/blob/main/DATA.md
set -euo pipefail
cd "$(dirname "$0")/.."
mkdir -p data
base=https://github.com/GLMap/earstrip/releases/download/bench-data-1
files=("$@")
[ ${#files[@]} -eq 0 ] && files=(z13 z9 world gom4old)
for f in "${files[@]}"; do
	[ -f "data/$f.bin" ] && continue
	# gh is often much faster than the plain download; it needs a login, so curl is the fallback
	if ! { command -v gh >/dev/null 2>&1 &&
		gh release download bench-data-1 -R GLMap/earstrip -p "$f.bin.tar.xz" -D data --clobber; }; then
		curl -fL --retry 5 --retry-all-errors -C - -o "data/$f.bin.tar.xz" "$base/$f.bin.tar.xz"
	fi
	tar -xJf "data/$f.bin.tar.xz" -C data
	rm "data/$f.bin.tar.xz"
done
sha=$(command -v sha256sum >/dev/null 2>&1 && echo sha256sum || echo "shasum -a 256")
cd data
for f in "${files[@]}"; do grep " $f.bin\$" ../scripts/SHA256SUMS; done | $sha -c -
