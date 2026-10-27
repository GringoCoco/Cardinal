// Package tg — минимальный клиент Telegram Bot API (только нужное Cardinal).
package tg

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const apiBase = "https://api.telegram.org"

// Update — сообщение или callback из getUpdates.
type Update struct {
	UpdateID int `json:"update_id"`
	Message  *struct {
		Chat struct {
			ID int64 `json:"id"`
		} `json:"chat"`
		Text string `json:"text"`
	} `json:"message"`
	CallbackQuery *struct {
		ID   string `json:"id"`
		From struct {
			ID int64 `json:"id"`
		} `json:"from"`
		Data      string `json:"data"`
		MessageID int    `json:"message,omitempty"`
	} `json:"callback_query"`
}

type InlineButton struct {
	Text string `json:"text"`
	Data string `json:"callback_data"` // ≤64 байта
}

type ReplyMarkup struct {
	InlineKeyboard [][]InlineButton `json:"inline_keyboard,omitempty"`
}

type Client struct {
	Token string
	HTTP  *http.Client
}

// New создаёт клиент.
func New(token string) *Client {
	return &Client{Token: token, HTTP: &http.Client{Timeout: 45 * time.Second}}
}

func (c *Client) call(method string, params url.Values, out any) error {
	resp, err := c.HTTP.PostForm(apiBase+"/bot"+c.Token+"/"+method, params)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	var envelope struct {
		OK          bool            `json:"ok"`
		Result      json.RawMessage `json:"result"`
		Description string          `json:"description"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return fmt.Errorf("%s: некорректный ответ: %w", method, err)
	}
	if !envelope.OK {
		return fmt.Errorf("%s: %s", method, envelope.Description)
	}
	if out != nil && len(envelope.Result) > 0 {
		return json.Unmarshal(envelope.Result, out)
	}
	return nil
}

// GetUpdates — long poll возможен только одним потребителем; возвращает новые.
func (c *Client) GetUpdates(offset int, timeoutSec int) ([]Update, error) {
	params := url.Values{
		"offset":          {fmt.Sprint(offset)},
		"timeout":         {fmt.Sprint(timeoutSec)},
		"allowed_updates": {`["message","callback_query"]`},
	}
	var updates []Update
	err := c.call("getUpdates", params, &updates)
	return updates, err
}

// Send — текст в чат. silent => без звука; markup => inline-кнопки.
func (c *Client) Send(chatID int64, text string, silent bool, markup *ReplyMarkup) (int64, error) {
	params := url.Values{
		"chat_id":                  {fmt.Sprint(chatID)},
		"text":                     {text},
		"disable_notification":     {fmt.Sprint(silent)},
		"disable_web_page_preview": {"true"},
	}
	if markup != nil {
		b, _ := json.Marshal(markup)
		params.Set("reply_markup", string(b))
	}
	var result struct {
		MessageID int64 `json:"message_id"`
	}
	if err := c.call("sendMessage", params, &result); err != nil {
		return 0, err
	}
	return result.MessageID, nil
}

// AnswerCallback — подтверждение нажатия (убирает «часики» на кнопке).
func (c *Client) AnswerCallback(callbackID, text string) error {
	params := url.Values{
		"callback_query_id": {callbackID},
		"text":              {text},
		"show_alert":        {"false"},
	}
	return c.call("answerCallbackQuery", params, nil)
}

// EditMarkup — убрать/заменить кнопки у уже отправленного сообщения.
func (c *Client) EditMarkup(chatID, messageID int64, markup *ReplyMarkup) error {
	params := url.Values{
		"chat_id":    {fmt.Sprint(chatID)},
		"message_id": {fmt.Sprint(messageID)},
	}
	if markup != nil {
		b, _ := json.Marshal(markup)
		params.Set("reply_markup", string(b))
	} else {
		params.Set("reply_markup", `{"inline_keyboard":[]}`)
	}
	return c.call("editMessageReplyMarkup", params, nil)
}

// Me — базовая проверка токена (getMe): возвращает @username бота.
func (c *Client) Me() (string, error) {
	var me struct {
		Username string `json:"username"`
	}
	if err := c.call("getMe", url.Values{}, &me); err != nil {
		return "", err
	}
	return me.Username, nil
}
