#!/bin/sh

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
awk '$2 != prev {print $2, $3; prev=$2} {printf "\t%s\n", $1}'
