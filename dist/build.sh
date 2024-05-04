#!/bin/sh

version="$1"
[ -n "$version" ] || { echo >&2 "usage: $0 VERSION"; exit 1; }

for pkg in $(go list -f '{{.Deps}}' | sed 's/^\[//; s/\]$//'); do
	go list -f $pkg' {{.Standard}} {{.Dir}}' $pkg
done |
while read -r pkg std dir; do
	if [ 'true' = $std ]; then
		echo $pkg _ $(go version |sed 's/ /_/g')
	else
		echo $pkg $(
			(cd "$dir"
			if git rev-parse --short=10 --show-toplevel HEAD 2>/dev/null; then
				git diff-index --quiet HEAD || echo +
			else 
				hg root; hg identify
			fi)
		) | sed 's/ +/+/'
	fi
done |
sort -k2,2 -k1,1 |
awk '$2 != prev {print $2, $3; prev=$2} {printf "\t%s\n", $1}' >sqribe.exe.revs

# epic hack to get go.wde to use the icon embedded in the .exe for the window icon
rm -f sqribe.exe sqribe.syso || exit 1
id=$(/c/src/go/bin/rsrc -arch amd64 -ico aux/sqribe64.ico -o sqribe.syso |awk '{print $NF; print $0 >"/dev/stderr"}')
sed -i 's/\(icon := w32.LoadIcon(gAppInstance, w32.MakeIntResource(\).*))/\1'$id'))/' $(go list -f '{{.Dir}}' github.com/skelterjohn/go.wde)'\win\utils_windows.go'

# hack the version number in
sed -i "s/sqribe version unknown/sqribe version $version/" sqribe.go
sed -i "s/SetTitle(\"Sqribe\")/SetTitle(\"Sqribe v$version\")/" sqribe_wde.go

go build -ldflags '-H windowsgui'
