# Sqribe

Sqribe is designed to assist you in transcribing music - the task of determining which notes
were played from a musical recording.


## Requirements

Sqribe runs on linux, windows, and osx.

At this stage sqribe depends on two files external to the repository - a font and a soundfont.
These files *must* be called `luxisr.ttf` and `FluidR3_GM.sf2`. It's easiest to place them (or
a symlink) in the same directory as the executable - sqribe will search several other directories
for these files but the search path is not configurable.

It makes use of the following libraries, which will also need to be installed:
* [fluidsynth](http://www.fluidsynth.org) for soundfont rendering
* [ffmpeg](http://www.ffmpeg.org) for decoding audio from audio/video files
* [portaudio](http://www.portaudio.com) for playing audio

On linux [gtk](http://www.gtk.org) is also used for system dialogs


## Usage

To begin the process sqribe requires some audio data. You can either point it at an audio/video
file via the command-line:

    $ sqribe './Ronald Jenkees/Disorganized Fun.mp3'

Or launch sqribe with no arguments and press ctrl-O to navigate to the desired song.

You will be presented with a waveform display and a time axis underneath it. The first thing to
do is to mark some "beats", which define the underlying timing for the notes we will add later.
This is achieved by pressing space to start playing the song, and then pressing enter in time
with the music. When you're done placing beats press space to stop playback.

Once the beats are laid, create a staff by clicking on the button near the bottom left of the
screen containing a + sign. Left-click creates a treble-clef staff, right-click creates a
bass-clef staff.

Now you can place notes on top of the waveform, by right-clicking. You will see a preview of
the note that would be added as you move the mouse around. Sqribe doesn't deal in rests; just
point the mouse at where you want the note to start. The note will snap to the nearest
quarter/eighth/sixteenth/triplet/half-triplet position.

Once you have some notes down play the song again (press space). The notes you have placed will
sound alongside the original recording. Hopefully this allows you to detect any errors in your
transcription!

Note that playback in sqribe is always looped. You can select a time range by dragging along the
time axis below the waveform, or by dragging along the beat axis above the waveform (which is
usually what you want). Playback will then loop over the selected time. Notes can be added or
modified during playback, so this allows you to eg. repeat a specific bar and start guessing at
the notes being played until you have the whole bar figured out.

Sqribe will automatically save your work when you exit. To resume transcribing, simply open the
same audio file again.

## Controls (subject to change)

* adjust the time period being viewed: left/right arrows, middle-click drag
* zoom in or out: up/down arrows, mouse-wheel

* cycle the key signature (follows circle of fifths): F2, F3
* adjust the midi tuning (eg. to match a recording where A is not 440Hz): F5, F6

* select beats: left-drag in beat-axis
* quantize beats within selected beat range: q
* repeat notes within selected beat range: %

* start/stop playback: space
* mute/unmute beat tones: t
* mute/unmute placed notes: m
* mute/unmute recording: a
* adjust volume of placed notes: shift-pgup, shift-pgdn
* adjust volume of recording: pgup, pgdn

* select notes: left-click, left-drag (hold shift to add further notes)
* transpose selected notes by one semitone: # (sharper), @ (flatter)
* transpose selected notes by one octave: 8 (higher), shift-8 (lower)
* delete selected notes: delete
* cut selected notes: ctrl-x, shift-delete
* copy selected notes: ctrl-c
* cancel clipboard placement: escape, ctrl-v, shift-insert, insert
* recall clipboard: ctrl-v, shift-insert, insert

* open new audio file: ctrl-o
* export to MusicXML: ctrl-e
* save work: s 
