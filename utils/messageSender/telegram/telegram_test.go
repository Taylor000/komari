package telegram

import (
	"strings"
	"testing"
	"time"

	"github.com/komari-monitor/komari/database/models"
)

func TestFormatEventMessageIncludesDiagnosticContext(t *testing.T) {
	eventTime := time.Date(2026, 8, 5, 14, 30, 0, 0, time.FixedZone("CST", 8*60*60))
	message := formatEventMessageForSite(models.EventMessage{
		Event: "Alert",
		Clients: []models.Client{{
			Name:   "HK <API> & DB",
			Region: "香港",
			Group:  "生产环境",
			IPv4:   "192.0.2.8",
		}},
		Time: eventTime,
		Details: []models.EventMessageField{
			{Label: "触发条件", Value: "CPU >= 90% & RAM >= 80%"},
		},
		Message: "请检查 <worker> & database",
	}, "Komari")

	checks := []string{
		"🚨 <b>服务器资源告警</b>",
		"<code>告警 · RESOURCE</code>",
		"HK &lt;API&gt; &amp; DB",
		"香港 · 生产环境",
		"<code>192.0.2.8</code>",
		"CPU &gt;= 90% &amp; RAM &gt;= 80%",
		"请检查 &lt;worker&gt; &amp; database",
		"2026-08-05 14:30:00 CST",
	}
	for _, expected := range checks {
		if !strings.Contains(message, expected) {
			t.Errorf("formatted message does not contain %q:\n%s", expected, message)
		}
	}
	if strings.Contains(message, "<API>") || strings.Contains(message, "<worker>") {
		t.Fatalf("formatted message contains unescaped dynamic HTML:\n%s", message)
	}
}

func TestFormatEventMessageLimitsClientList(t *testing.T) {
	clients := make([]models.Client, 10)
	for i := range clients {
		clients[i].Name = "server"
	}

	message := formatEventMessageForSite(models.EventMessage{Event: "Offline", Clients: clients, Time: time.Now()}, "Komari")
	if !strings.Contains(message, "受影响服务器（10）") {
		t.Fatalf("formatted message is missing the total client count:\n%s", message)
	}
	if !strings.Contains(message, "另外 2 台服务器") {
		t.Fatalf("formatted message is missing the truncated client count:\n%s", message)
	}
}
