package relay

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

// InitData — распарсенная и проверенная авторизация Mini App.
type InitData struct {
	ChatID   int64
	UserName string
}

// ValidateInitData проверяет подпись Telegram initData:
//
//	secret = HMAC_SHA256("WebAppData", botToken)
//	hash   = HMAC_SHA256(secret, data_check_string)
//
// data_check_string — пары key=value (кроме hash), отсортированные, через \n.
// Свежесть: auth_date не старше maxAge.
func ValidateInitData(botToken, rawQuery string, maxAge time.Duration) (*InitData, error) {
	if rawQuery == "" {
		return nil, fmt.Errorf("нет initData")
	}
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return nil, fmt.Errorf("не парсится initData: %w", err)
	}
	hashHex := values.Get("hash")
	if hashHex == "" {
		return nil, fmt.Errorf("нет hash в initData")
	}

	// data_check_string
	keys := make([]string, 0, len(values))
	for k := range values {
		if k == "hash" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	pairs := make([]string, 0, len(keys))
	for _, k := range keys {
		pairs = append(pairs, k+"="+values.Get(k))
	}
	checkString := strings.Join(pairs, "\n")

	secret := hmac.New(sha256.New, []byte("WebAppData"))
	secret.Write([]byte(botToken))
	mac := hmac.New(sha256.New, secret.Sum(nil))
	mac.Write([]byte(checkString))
	if !hmac.Equal([]byte(mac.Sum(nil)), []byte(mustHex(hashHex))) {
		return nil, fmt.Errorf("подпись initData не сходится")
	}

	// свежесть
	if maxAge > 0 {
		authDate := values.Get("auth_date")
		if authDate == "" {
			return nil, fmt.Errorf("нет auth_date")
		}
		var ts int64
		if _, err := fmt.Sscanf(authDate, "%d", &ts); err != nil {
			return nil, fmt.Errorf("кривой auth_date")
		}
		if time.Since(time.Unix(ts, 0)) > maxAge {
			return nil, fmt.Errorf("initData устарел")
		}
	}

	// user.id — chat id пользователя (личный чат)
	userJSON := values.Get("user")
	if userJSON == "" {
		return nil, fmt.Errorf("нет user в initData")
	}
	var user struct {
		ID        int64  `json:"id"`
		Username  string `json:"username"`
		FirstName string `json:"first_name"`
	}
	if err := json.Unmarshal([]byte(userJSON), &user); err != nil {
		return nil, fmt.Errorf("кривой user: %w", err)
	}
	if user.ID == 0 {
		return nil, fmt.Errorf("нет id у user")
	}
	name := user.Username
	if name == "" {
		name = user.FirstName
	}
	return &InitData{ChatID: user.ID, UserName: name}, nil
}

func mustHex(s string) []byte {
	b, _ := hex.DecodeString(s)
	return b
}
