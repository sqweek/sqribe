package main

import (
	"code.google.com/p/portaudio-go/portaudio"
)

func PlatformHostApis() []portaudio.HostApiType {
	return []portaudio.HostApiType{portaudio.MME}
}
