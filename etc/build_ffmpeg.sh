SUFFIX=_a

./configure --disable-gpl --disable-nonfree --disable-programs --disable-libssh --disable-gnutls --disable-devices --disable-outdevs --disable-filters --disable-iconv --disable-bzlib --disable-protocols --disable-encoders --disable-bsfs --disable-hwaccels --disable-muxers --disable-sdl --disable-libxcb --disable-doc --enable-shared --disable-iconv --disable-libass --disable-libcaca --disable-xlib --disable-zlib --disable-lzma --build_suffix=$SUFFIX --enable-protocol=file --disable-vda --disable-dxva2 --disable-vaapi --disable-vdpau &&

make &&

for i in libavcodec libavformat libavutil libswresample; do
	cp -a $i/$i$SUFFIX.so* .
done


