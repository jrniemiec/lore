package voice

// STT is the interface for speech-to-text backends.
// whisper.cpp adapter will implement this via CLI subprocess.
// TODO: implement
type STT interface {
	Transcribe(audioPath string) (string, error)
}
