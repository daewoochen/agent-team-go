package gateway

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

func decodeTelegram(r *http.Request) (InboundMessage, bool, error) {
	var payload struct {
		Message *struct {
			Text string `json:"text"`
			Chat struct {
				ID int64 `json:"id"`
			} `json:"chat"`
			From struct {
				ID int64 `json:"id"`
			} `json:"from"`
		} `json:"message"`
		EditedMessage *struct {
			Text string `json:"text"`
			Chat struct {
				ID int64 `json:"id"`
			} `json:"chat"`
			From struct {
				ID int64 `json:"id"`
			} `json:"from"`
		} `json:"edited_message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		return InboundMessage{}, false, err
	}

	var message *struct {
		Text string `json:"text"`
		Chat struct {
			ID int64 `json:"id"`
		} `json:"chat"`
		From struct {
			ID int64 `json:"id"`
		} `json:"from"`
	}
	switch {
	case payload.Message != nil:
		message = payload.Message
	case payload.EditedMessage != nil:
		message = payload.EditedMessage
	default:
		return InboundMessage{}, false, nil
	}

	text := strings.TrimSpace(message.Text)
	if text == "" {
		return InboundMessage{}, false, nil
	}
	return InboundMessage{
		Channel: "telegram",
		Target:  strconv.FormatInt(message.Chat.ID, 10),
		UserID:  strconv.FormatInt(message.From.ID, 10),
		Text:    text,
	}, true, nil
}

func decodeFeishu(r *http.Request) (InboundMessage, string, bool, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return InboundMessage{}, "", false, err
	}

	var payload struct {
		Challenge string `json:"challenge"`
		Header    struct {
			EventType string `json:"event_type"`
		} `json:"header"`
		Event struct {
			Message struct {
				ChatID      string `json:"chat_id"`
				MessageType string `json:"message_type"`
				Content     string `json:"content"`
			} `json:"message"`
			Sender struct {
				SenderID struct {
					OpenID string `json:"open_id"`
					UserID string `json:"user_id"`
				} `json:"sender_id"`
			} `json:"sender"`
		} `json:"event"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return InboundMessage{}, "", false, err
	}
	if strings.TrimSpace(payload.Challenge) != "" {
		return InboundMessage{}, payload.Challenge, false, nil
	}
	if payload.Header.EventType != "" && payload.Header.EventType != "im.message.receive_v1" {
		return InboundMessage{}, "", false, nil
	}
	if payload.Event.Message.MessageType != "" && payload.Event.Message.MessageType != "text" {
		return InboundMessage{}, "", false, nil
	}

	var content struct {
		Text string `json:"text"`
	}
	if strings.TrimSpace(payload.Event.Message.Content) == "" {
		return InboundMessage{}, "", false, nil
	}
	if err := json.Unmarshal([]byte(payload.Event.Message.Content), &content); err != nil {
		return InboundMessage{}, "", false, fmt.Errorf("decode feishu text content: %w", err)
	}
	text := strings.TrimSpace(content.Text)
	if text == "" {
		return InboundMessage{}, "", false, nil
	}

	return InboundMessage{
		Channel: "feishu",
		Target:  strings.TrimSpace(payload.Event.Message.ChatID),
		UserID:  firstNonEmpty(payload.Event.Sender.SenderID.OpenID, payload.Event.Sender.SenderID.UserID),
		Text:    text,
	}, "", true, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
