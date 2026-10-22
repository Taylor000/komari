package telegram

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/komari-monitor/komari/config"
	"github.com/komari-monitor/komari/database/models"
	"github.com/komari-monitor/komari/utils/messageSender/factory"
)

const (
	maxDisplayedClients = 8
	maxDetailRunes      = 500
	maxMessageRunes     = 1200
)

type eventPresentation struct {
	title    string
	emoji    string
	severity string
	code     string
}

var eventPresentations = map[string]eventPresentation{
	"offline": {title: "服务器离线", emoji: "🔴", severity: "严重", code: "OFFLINE"},
	"online":  {title: "服务器恢复在线", emoji: "🟢", severity: "恢复", code: "RECOVERED"},
	"alert":   {title: "服务器资源告警", emoji: "🚨", severity: "告警", code: "RESOURCE"},
	"traffic": {title: "服务器流量告警", emoji: "📡", severity: "告警", code: "TRAFFIC"},
	"expire":  {title: "服务器即将到期", emoji: "⏳", severity: "提醒", code: "EXPIRING"},
	"renew":   {title: "服务器自动续费成功", emoji: "🔄", severity: "成功", code: "RENEWED"},
	"login":   {title: "管理后台登录", emoji: "🔐", severity: "安全", code: "LOGIN"},
	"test":    {title: "Telegram 通知测试", emoji: "🧪", severity: "测试", code: "TEST"},
}

type TelegramSender struct {
	Addition
}

func (t *TelegramSender) GetName() string {
	return "telegram"
}

func (t *TelegramSender) GetConfiguration() factory.Configuration {
	return &t.Addition
}

func (t *TelegramSender) Init() error {
	// 初始化逻辑，如果需要的话
	return nil
}

func (t *TelegramSender) Destroy() error {
	// 清理逻辑，如果需要的话
	return nil
}

func (t *TelegramSender) SendTextMessage(message, title string) error {
	fullMessage := message
	if title != "" {
		fullMessage = fmt.Sprintf("<b>%s</b>\n%s", html.EscapeString(title), message)
	}
	return t.sendHTMLMessage(fullMessage)
}

// SendEvent renders Telegram notifications as compact diagnostic cards. Other
// providers continue to use the configurable generic text template.
func (t *TelegramSender) SendEvent(event models.EventMessage) error {
	siteName, _ := config.GetAs[string](config.SitenameKey, "Komari")
	return t.sendHTMLMessage(formatEventMessageForSite(event, siteName))
}

func (t *TelegramSender) sendHTMLMessage(fullMessage string) error {

	if fullMessage == "" {
		return errors.New("message is empty")
	}

	endpoint := t.Addition.Endpoint + t.Addition.BotToken + "/sendMessage"

	data := url.Values{}
	data.Set("chat_id", t.Addition.ChatID)
	data.Set("text", fullMessage)
	data.Set("parse_mode", "HTML")

	// Add message_thread_id if provided
	if t.Addition.MessageThreadID != "" {
		data.Set("message_thread_id", t.Addition.MessageThreadID)
	}

	resp, err := http.PostForm(endpoint, data)
	if err != nil {
		return fmt.Errorf("failed to send message: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram API returned non-OK status: %d", resp.StatusCode)
	}

	var result struct {
		Ok          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	if !result.Ok {
		return fmt.Errorf("telegram API error: %s", result.Description)
	}

	return nil
}

func formatEventMessageForSite(event models.EventMessage, siteName string) string {
	presentation := presentationForEvent(event)
	var b strings.Builder

	fmt.Fprintf(&b, "%s <b>%s</b>\n", presentation.emoji, html.EscapeString(presentation.title))
	fmt.Fprintf(&b, "<code>%s · %s</code>\n", presentation.severity, presentation.code)

	if len(event.Clients) > 0 {
		b.WriteString("\n")
		writeClients(&b, event.Clients)
	}

	if len(event.Details) > 0 {
		b.WriteString("\n📋 <b>事件详情</b>\n")
		for _, detail := range event.Details {
			label := strings.TrimSpace(detail.Label)
			value := strings.TrimSpace(detail.Value)
			if label == "" || value == "" {
				continue
			}
			fmt.Fprintf(&b, "• <b>%s：</b>%s\n",
				html.EscapeString(label),
				escapeAndLimit(value, maxDetailRunes),
			)
		}
	}

	if message := strings.TrimSpace(event.Message); message != "" {
		b.WriteString("\n💬 <b>诊断信息</b>\n")
		b.WriteString(escapeAndLimit(message, maxMessageRunes))
		b.WriteString("\n")
	}

	when := event.Time
	if when.IsZero() {
		when = time.Now()
	}
	fmt.Fprintf(&b, "\n🕒 <b>时间：</b><code>%s</code>\n", html.EscapeString(when.Format("2006-01-02 15:04:05 MST")))
	siteName = strings.TrimSpace(siteName)
	if siteName == "" {
		siteName = "Komari"
	}
	fmt.Fprintf(&b, "<i>%s · Komari Monitor</i>", html.EscapeString(siteName))

	return strings.TrimSpace(b.String())
}

func presentationForEvent(event models.EventMessage) eventPresentation {
	if presentation, ok := eventPresentations[strings.ToLower(strings.TrimSpace(event.Event))]; ok {
		return presentation
	}

	emoji := strings.TrimSpace(event.Emoji)
	if emoji == "" {
		emoji = "🔔"
	}
	title := strings.TrimSpace(event.Event)
	if title == "" {
		title = "Komari 通知"
	}
	return eventPresentation{title: title, emoji: emoji, severity: "通知", code: "EVENT"}
}

func writeClients(b *strings.Builder, clients []models.Client) {
	if len(clients) == 1 {
		client := clients[0]
		fmt.Fprintf(b, "🖥 <b>服务器：</b>%s\n", html.EscapeString(clientDisplayName(client)))
		if location := clientLocation(client); location != "" {
			fmt.Fprintf(b, "📍 <b>位置：</b>%s\n", html.EscapeString(location))
		}
		if address := clientAddress(client); address != "" {
			fmt.Fprintf(b, "🌐 <b>IP：</b><code>%s</code>\n", html.EscapeString(address))
		}
		return
	}

	fmt.Fprintf(b, "🖥 <b>受影响服务器（%d）</b>\n", len(clients))
	limit := len(clients)
	if limit > maxDisplayedClients {
		limit = maxDisplayedClients
	}
	for _, client := range clients[:limit] {
		suffixParts := make([]string, 0, 2)
		if location := clientLocation(client); location != "" {
			suffixParts = append(suffixParts, location)
		}
		if address := clientAddress(client); address != "" {
			suffixParts = append(suffixParts, address)
		}
		suffix := ""
		if len(suffixParts) > 0 {
			suffix = " — " + strings.Join(suffixParts, " · ")
		}
		fmt.Fprintf(b, "• %s%s\n", html.EscapeString(clientDisplayName(client)), html.EscapeString(suffix))
	}
	if remaining := len(clients) - limit; remaining > 0 {
		fmt.Fprintf(b, "• … 另外 %d 台服务器\n", remaining)
	}
}

func clientDisplayName(client models.Client) string {
	if name := strings.TrimSpace(client.Name); name != "" {
		return name
	}
	if uuid := strings.TrimSpace(client.UUID); uuid != "" {
		return uuid
	}
	return "未命名服务器"
}

func clientLocation(client models.Client) string {
	parts := make([]string, 0, 2)
	if region := strings.TrimSpace(client.Region); region != "" {
		parts = append(parts, region)
	}
	if group := strings.TrimSpace(client.Group); group != "" && group != client.Region {
		parts = append(parts, group)
	}
	return strings.Join(parts, " · ")
}

func clientAddress(client models.Client) string {
	if ipv4 := strings.TrimSpace(client.IPv4); ipv4 != "" {
		return ipv4
	}
	return strings.TrimSpace(client.IPv6)
}

func escapeAndLimit(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	if utf8.RuneCountInString(value) > maxRunes {
		runes := []rune(value)
		value = string(runes[:maxRunes]) + "…"
	}
	return html.EscapeString(value)
}

func init() {
	factory.RegisterMessageSender(func() factory.IMessageSender {
		return &TelegramSender{}
	})
}

// 确保实现了 IMessageSender 接口
var _ factory.IMessageSender = (*TelegramSender)(nil)
var _ factory.IEventMessageSender = (*TelegramSender)(nil)
