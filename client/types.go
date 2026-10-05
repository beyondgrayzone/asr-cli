package main

type ConfigMsg struct {
	Mode       string  `json:"mode"`                  // "asr" or "multitalker"
	Language   string  `json:"language,omitempty"`    // "en" or "all"
	TargetLang *string `json:"target_lang,omitempty"` // e.g., "tr-TR" or "auto"
}

type WordTimestampMsg struct {
	Word      string  `json:"word"`
	StartSecs float32 `json:"start_secs"`
	EndSecs   float32 `json:"end_secs"`
}

type OutputMsg struct {
	Text      string             `json:"text"`
	SpeakerID *int               `json:"speaker_id"`
	Words     []WordTimestampMsg `json:"words"`
}
