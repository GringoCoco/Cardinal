package relay

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
	"time"
)

// buildSigned — генерирует корректно подписанный initData для теста.
func buildSigned(t *testing.T, botToken string, authDate time.Time, extra map[string]string) string {
	t.Helper()
	params := map[string]string{
		"user":      `{"id":7599942832,"username":"tester","first_name":"T"}`,
		"auth_date": fmt.Sprint(authDate.Unix()),
	}
	for k, v := range extra {
		params[k] = v
	}
	// data_check_string: отсортированные key=value через \n
	check := ""
	for _, k := range []string{"auth_date", "user"} {
		check += k + "=" + params[k] + "\n"
	}
	check = check[:len(check)-1]
	if extra != nil {
		// пересобираем честно: сортировка ключей
		keys := []string{"auth_date", "user"}
		for k := range extra {
			keys = append(keys, k)
		}
		// сортировка пузырьком (ключей мало)
		for i := 0; i < len(keys); i++ {
			for j := i + 1; j < len(keys); j++ {
				if keys[j] < keys[i] {
					keys[i], keys[j] = keys[j], keys[i]
				}
			}
		}
		check = ""
		for _, k := range keys {
			check += k + "=" + params[k] + "\n"
		}
		check = check[:len(check)-1]
	}

	secret := hmac.New(sha256.New, []byte("WebAppData"))
	secret.Write([]byte(botToken))
	mac := hmac.New(sha256.New, secret.Sum(nil))
	mac.Write([]byte(check))
	hash := hex.EncodeToString(mac.Sum(nil))

	q := "auth_date=" + params["auth_date"] + "&user=" + params["user"]
	for k, v := range extra {
		q += "&" + k + "=" + v
	}
	return q + "&hash=" + hash
}

func TestValidateInitData(t *testing.T) {
	botToken := "123456:TESTTOKEN"

	// корректная свежая подпись
	raw := buildSigned(t, botToken, time.Now(), nil)
	got, err := ValidateInitData(botToken, raw, time.Hour)
	if err != nil {
		t.Fatalf("валидная подпись не прошла: %v", err)
	}
	if got.ChatID != 7599942832 {
		t.Errorf("ChatID = %d", got.ChatID)
	}
	if got.UserName != "tester" {
		t.Errorf("UserName = %q", got.UserName)
	}

	// подпись чужим токеном
	raw = buildSigned(t, "999999:OTHERTOKEN", time.Now(), nil)
	if _, err := ValidateInitData(botToken, raw, time.Hour); err == nil {
		t.Error("подпись чужим токеном прошла — критично")
	}

	// устаревшая
	raw = buildSigned(t, botToken, time.Now().Add(-25*time.Hour), nil)
	if _, err := ValidateInitData(botToken, raw, time.Hour); err == nil {
		t.Error("устаревший initData прошёл")
	}

	// без hash
	if _, err := ValidateInitData(botToken, "user=%7B%7D", time.Hour); err == nil {
		t.Error("отсутствие hash не поймано")
	}
}

func TestHubRegisterAndPair(t *testing.T) {
	h := NewHub()

	d, err := h.Register("mac-1", "MacBook")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if d.Token == "" {
		t.Fatal("пустой device token")
	}
	// повторная регистрация — тот же token
	d2, _ := h.Register("mac-1", "MacBook Pro")
	if d2.Token != d.Token {
		t.Error("token перегенерировался — daemon потеряет WS")
	}

	// код пары
	code, err := h.SetPairCode("mac-1")
	if err != nil {
		t.Fatalf("pair-code: %v", err)
	}
	got, ok := h.PairByCode(code, 111)
	if !ok || got.ID != "mac-1" {
		t.Fatalf("pair failed: %v %v", got, ok)
	}
	// код одноразовый
	if _, ok := h.PairByCode(code, 111); ok {
		t.Error("код не одноразовый")
	}
	// lookup по чату
	if dd := h.DeviceByChat(111); dd == nil || dd.ID != "mac-1" {
		t.Error("DeviceByChat не находит привязку")
	}
	if h.DeviceByChat(222) != nil {
		t.Error("привязка чужого чата существует")
	}
	if h.DeviceByToken("мусор") != nil {
		t.Error("чужой token прошёл")
	}
}
