package audio

import (
	"github.com/gordonklaus/portaudio"
)

func PlatformHostApis() []portaudio.HostApiType {
	return []portaudio.HostApiType{portaudio.JACK, portaudio.ALSA, portaudio.OSS}
}
