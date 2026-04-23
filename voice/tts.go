package voice

// TTS is the interface for text-to-speech backends.
// Piper adapter will implement this via CLI subprocess.
// TODO: implement
type TTS interface {
	Speak(text string) error
	Stop() error
}
