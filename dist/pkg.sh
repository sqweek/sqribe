#!/bin/sh

V="$1"
[ -n "$V" ] || { echo >&2 usage: pkg.sh VERSION; exit 1; }
vdir="$dist/$V"
mkdir -p "$vdir" || exit 1
cp sqribe.exe sqribe.exe.revs "$vdir"

##old zip-building approach
#pkg="dist/$V/sqribe-$V"
#mkdir -p "$pkg" || exit 1
#cp dist/$V/sqribe.exe $pkg
#cp FluidR3_GM.sf2 luxisr.ttf $pkg
#cp *dll $pkg

MSIDIR=./out/sqribe.mm/MSI
MSI="$MSIDIR/Sqribe-$V.msi"
rm -f "$MSI"
makemsi sqribe.mm
[ -e "$MSI" ] || exit 1
mv "$MSIDIR/"* "$vdir"

