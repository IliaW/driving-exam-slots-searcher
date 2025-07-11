package utils

import (
	"hsc-gov/model"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
)

const (
	EmojiTada        = "🎉"
	EmojiLoudspeaker = "🔊"
	EmojiWarning     = "⚠️"
	EmojiFacepalm    = "🤦"
)

func SendNotification(ntf *model.Notification) {
	client := &http.Client{}
	var req *http.Request
	var err error
	if ntf.Filename != "" {
		file, _ := os.Open(ntf.Filename)
		req, err = http.NewRequest("PUT", "https://ntfy.sh/"+ntf.Topic, file)
	} else {
		req, err = http.NewRequest("POST", "https://ntfy.sh/"+ntf.Topic, strings.NewReader(ntf.Message))
	}
	if err != nil {
		slog.Error("can't create request to NTFY", slog.String("error", err.Error()))
		return
	}

	req.Header.Set("Content-Type", "text/plain")
	if ntf.Title != "" {
		req.Header.Set("Title", ntf.Title)
	}
	if len(ntf.Tags) > 0 {
		req.Header.Set("Tags", strings.Join(ntf.Tags, ","))
	}
	if ntf.Filename != "" {
		req.Header.Set("Filename", ntf.Filename)
		req.Header.Set("Content-Type", "image/png")
	}
	if ntf.Priority != 0 {
		req.Header.Set("Priority", strconv.Itoa(ntf.Priority))
	} else {
		req.Header.Set("Priority", "3")
	}

	resp, err := client.Do(req)
	if err != nil {
		slog.Error("can't send request to NTFY", slog.String("error", err.Error()))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		slog.Error("NTFY error response", slog.String("status", resp.Status),
			slog.String("body", string(bodyBytes)))
		return
	}

	slog.Debug("notification sent to NTFY")
}
