package main

import (
	"fmt"
	"io"
)

func formatOutput(w io.Writer, out OutputMsg, timestamps bool, lastSpeaker *int) {
	if timestamps && len(out.Words) > 0 {
		for _, msg := range out.Words {
			if out.SpeakerID != nil {
				fmt.Fprintf(w, "[Speaker %d] [%.2fs - %.2fs] %s\n", *out.SpeakerID, msg.StartSecs, msg.EndSecs, msg.Word)
			} else {
				fmt.Fprintf(w, "[%.2fs - %.2fs] %s\n", msg.StartSecs, msg.EndSecs, msg.Word)
			}
		}
	} else {
		if out.SpeakerID != nil {
			if *lastSpeaker != *out.SpeakerID {
				fmt.Fprintf(w, "\n[Speaker %d] ", *out.SpeakerID)
				*lastSpeaker = *out.SpeakerID
			}
			fmt.Fprintf(w, "%s", out.Text)
		} else {
			fmt.Fprintf(w, "%s", out.Text)
		}
	}
}
