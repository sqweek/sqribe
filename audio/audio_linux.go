package audio

import (
	"code.google.com/p/portaudio-go/portaudio"
)

func PlatformHostApis() []portaudio.HostApiType {
	return []portaudio.HostApiType{portaudio.JACK, portaudio.ALSA, portaudio.OSS}
}
