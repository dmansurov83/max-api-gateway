package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/MrCatchParkington/go-max-client/maxclient"
	qrcode "github.com/skip2/go-qrcode"
	"gopkg.in/yaml.v3"
)

func main() {
	outFile := flag.String("out", "config.yaml", "путь к config.yaml для обновления")
	debug := flag.Bool("debug", true, "печатать сырые ответы сервера")
	flag.Parse()

	client := maxclient.New()
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		fatal("ошибка подключения: %v", err)
	}

	qrLink, trackID, deviceID, err := startQR(ctx, client, *debug)
	if err != nil {
		fatal("QR: %v", err)
	}

	printQR(qrLink, "qr.png")

	auth, err := waitQR(ctx, client, trackID, deviceID, *debug)
	if err != nil {
		fatal("ожидание сканирования: %v", err)
	}

	fmt.Printf("\nToken:    %s\n", auth.token)
	fmt.Printf("DeviceID: %s\n", auth.deviceID)
	fmt.Printf("ChatID:   %d (Избранное)\n", auth.chatID)

	if err := updateConfig(*outFile, auth); err != nil {
		fatal("запись конфига: %v", err)
	}
	fmt.Printf("Конфиг обновлён: %s\n", *outFile)
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

func dump(label string, pkt *maxclient.Packet, debug bool) {
	if !debug {
		return
	}
	fmt.Printf("--- %s (opcode=%d seq=%d) ---\n%s\n", label, pkt.Opcode, pkt.Seq, prettyJSON(pkt.Payload))
}

func prettyJSON(raw json.RawMessage) string {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	out, _ := json.MarshalIndent(v, "", "  ")
	return string(out)
}

func newUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

func startQR(ctx context.Context, client *maxclient.Client, debug bool) (string, string, string, error) {
	deviceID, err := newUUID()
	if err != nil {
		return "", "", "", err
	}
	// hello (opcode 6)
	resp, err := client.InvokeMethod(ctx, 6, map[string]any{
		"userAgent": map[string]any{
			"deviceType":      "WEB",
			"locale":          "ru",
			"deviceLocale":    "ru",
			"osVersion":       "Linux",
			"deviceName":      "Chrome",
			"headerUserAgent": "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36",
			"appVersion":      "26.2.2",
			"screen":          "1080x1920 1.0x",
			"timezone":        "Europe/Moscow",
		},
		"deviceId": deviceID,
	})
	if err != nil {
		return "", "", "", fmt.Errorf("hello: %w", err)
	}
	dump("hello", resp, debug)

	// запрос QR (288)
	qrResp, err := client.InvokeMethod(ctx, 288, map[string]any{})
	if err != nil {
		return "", "", "", fmt.Errorf("get QR: %w", err)
	}
	dump("getQR", qrResp, debug)

	var qr struct {
		QRLink  string `json:"qrLink"`
		TrackID string `json:"trackId"`
	}
	if err := json.Unmarshal(qrResp.Payload, &qr); err != nil {
		return "", "", "", fmt.Errorf("parse QR response: %w", err)
	}
	return qr.QRLink, qr.TrackID, deviceID, nil
}

func waitQR(ctx context.Context, client *maxclient.Client, trackID string, deviceID string, debug bool) (*qrAuth, error) {
	interval := 3 * time.Second
	// опрос статуса (289), пока не отсканируют
	for {
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}

		statusResp, err := client.InvokeMethod(ctx, 289, map[string]any{"trackId": trackID})
		if err != nil {
			return nil, fmt.Errorf("QR status: %w", err)
		}

		var status struct {
			Status struct {
				LoginAvailable bool `json:"loginAvailable"`
			} `json:"status"`
		}
		if err := json.Unmarshal(statusResp.Payload, &status); err != nil {
			continue
		}
		if status.Status.LoginAvailable {
			dump("QR отсканирован (loginAvailable=true)", statusResp, debug)
			break
		}
	}

	// обмен trackId на токен (291)
	loginResp, err := client.InvokeMethod(ctx, 291, map[string]any{"trackId": trackID})
	if err != nil {
		return nil, fmt.Errorf("login by QR: %w", err)
	}
	dump("loginByQR", loginResp, debug)

	// проверяем, не требует ли сервер пароль
	var pwChallenge struct {
		PasswordChallenge *struct {
			Config struct {
				HintMaxLen int `json:"hintMaxLen"`
				PassMaxLen int `json:"passMaxLen"`
				PassMinLen int `json:"passMinLen"`
			} `json:"config"`
			Email   string `json:"email"`
			TrackID string `json:"trackId"`
		} `json:"passwordChallenge"`
		TokenAttrs map[string]any `json:"tokenAttrs"`
	}
	if err := json.Unmarshal(loginResp.Payload, &pwChallenge); err != nil {
		return nil, fmt.Errorf("parse login response: %w", err)
	}

	if pwChallenge.PasswordChallenge != nil && pwChallenge.PasswordChallenge.TrackID != "" {
		return handlePasswordChallenge(ctx, client, loginResp, trackID, deviceID, pwChallenge.PasswordChallenge.TrackID, debug)
	}

	// токен уже может быть в ответе — извлекаем
	token := extractToken(pwChallenge.TokenAttrs)
	if token == "" {
		return nil, fmt.Errorf("токен не найден в ответе (raw: %s)", prettyJSON(loginResp.Payload))
	}

	return finishAuth(ctx, client, loginResp, token, deviceID, debug)
}

func handlePasswordChallenge(ctx context.Context, client *maxclient.Client, loginResp *maxclient.Packet, trackID, deviceID, pwTrackID string, debug bool) (*qrAuth, error) {
	fmt.Printf("\nТребуется подтверждение паролем для %s\n", emailFromChallenge(loginResp))
	fmt.Print("Введи пароль от аккаунта MAX: ")
	var password string
	fmt.Scanln(&password)

	// opcode 115 AUTH_LOGIN_CHECK_PASSWORD (по исходникам PyMax)
	passResp, err := client.InvokeMethod(ctx, 115, map[string]any{
		"trackId":  pwTrackID,
		"password": password,
	})
	if err != nil {
		return nil, fmt.Errorf("check password: %w", err)
	}
	dump("checkPassword", passResp, debug)

	if hasError(passResp) {
		return nil, fmt.Errorf("check password: %s", prettyJSON(passResp.Payload))
	}

	// ответ 115 уже содержит токен (tokenAttrs.LOGIN.token) — берём его отсюда
	var pwResp struct {
		TokenAttrs map[string]any `json:"tokenAttrs"`
	}
	if err := json.Unmarshal(passResp.Payload, &pwResp); err != nil {
		return nil, fmt.Errorf("parse check password response: %w", err)
	}

	token := extractToken(pwResp.TokenAttrs)
	if token == "" {
		return nil, fmt.Errorf("токен не найден в ответе на пароль (raw: %s)", prettyJSON(passResp.Payload))
	}

	// завершаем: инициализация сессии login_by_token (19)
	return finishAuth(ctx, client, passResp, token, deviceID, debug)
}

func finishAuth(ctx context.Context, client *maxclient.Client, loginResp *maxclient.Packet, token, deviceID string, debug bool) (*qrAuth, error) {
	// chatID Избранное (DIALOG с одним участником)
	chatID := int64(0)
	var fr struct {
		Chats []struct {
			ID           int64          `json:"id"`
			Type         string         `json:"type"`
			Participants map[string]any `json:"participants"`
		} `json:"chats"`
	}
	json.Unmarshal(loginResp.Payload, &fr)
	for _, chat := range fr.Chats {
		if chat.Type == "DIALOG" && len(chat.Participants) == 1 {
			chatID = chat.ID
			break
		}
	}

	// login_by_token (19) для полной инициализации сессии
	lbtResp, err := client.InvokeMethod(ctx, 19, map[string]any{
		"interactive":  true,
		"token":        token,
		"chatsCount":   40,
		"chatsSync":    0,
		"contactsSync": 0,
		"presenceSync": -1,
		"draftsSync":   0,
	})
	if err != nil {
		// токен уже получен — login_by_token нужен только для инициализации сессии
		fmt.Printf("warning: login_by_token: %v (токен всё равно сохранён)\n", err)
	} else {
		dump("login_by_token", lbtResp, debug)
	}

	return &qrAuth{token: token, deviceID: deviceID, chatID: chatID}, nil
}

func extractToken(attrs map[string]any) string {
	for _, v := range attrs {
		if m, ok := v.(map[string]any); ok {
			if t, ok := m["token"].(string); ok && t != "" {
				return t
			}
		}
	}
	return ""
}

func emailFromChallenge(loginResp *maxclient.Packet) string {
	var c struct {
		PasswordChallenge struct {
			Email string `json:"email"`
		} `json:"passwordChallenge"`
	}
	json.Unmarshal(loginResp.Payload, &c)
	return c.PasswordChallenge.Email
}

func hasError(pkt *maxclient.Packet) bool {
	var m map[string]any
	json.Unmarshal(pkt.Payload, &m)
	_, ok := m["error"]
	return ok
}

func printQR(url, pngPath string) {
	png, err := qrcode.Encode(url, qrcode.Medium, 256)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ошибка генерации QR: %v\n", err)
		return
	}
	if err := os.WriteFile(pngPath, png, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "не удалось сохранить %s: %v\n", pngPath, err)
	}

	qr, err := qrcode.New(url, qrcode.Medium)
	if err != nil {
		fmt.Println(url)
		return
	}
	art := qr.ToString(false)
	lines := strings.Split(art, "\n")
	width := 0
	for _, l := range lines {
		if len(l) > width {
			width = len(l)
		}
	}
	pad := 2
	fmt.Println(strings.Repeat("#", width+pad*2))
	for _, l := range lines {
		fmt.Println(strings.Repeat("#", pad) + l + strings.Repeat("#", pad))
	}
	fmt.Println(strings.Repeat("#", width+pad*2))
	fmt.Println("\nОтсканируй QR-код в приложении MAX (Настройки → Другие устройства → Сканировать).")
	fmt.Printf("PNG сохранён: %s\n", pngPath)
}

type qrAuth struct {
	token    string
	deviceID string
	chatID   int64
}

func updateConfig(path string, auth *qrAuth) error {
	type cfg struct {
		Port          int    `yaml:"port"`
		MaxToken      string `yaml:"max_token"`
		MaxDeviceID   string `yaml:"max_device_id"`
		APIToken      string `yaml:"api_token"`
		DefaultChatID int64  `yaml:"default_chat_id"`
	}

	c := cfg{Port: 8000, DefaultChatID: auth.chatID}
	if data, err := os.ReadFile(path); err == nil {
		yaml.Unmarshal(data, &c)
	}
	c.MaxToken = auth.token
	if auth.deviceID != "" {
		c.MaxDeviceID = auth.deviceID
	}
	if c.DefaultChatID == 0 {
		c.DefaultChatID = auth.chatID
	}

	data, err := yaml.Marshal(&c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
