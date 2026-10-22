package notifier

import (
	"fmt"
	"log"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/komari-monitor/komari/database/clients"
	"github.com/komari-monitor/komari/database/dbcore"
	"github.com/komari-monitor/komari/database/models"
	messageevent "github.com/komari-monitor/komari/database/models/messageEvent"
	"github.com/komari-monitor/komari/database/records"
	"github.com/komari-monitor/komari/utils/messageSender"
)

// LoadNotificationService 管理定时器和任务
type LoadNotificationService struct {
	mu       sync.Mutex
	tickers  map[int]*time.Ticker
	tasks    map[int][]models.LoadNotification
	stopChan chan struct{}
}

var LoadNotificationManager = &LoadNotificationService{
	tickers:  make(map[int]*time.Ticker),
	tasks:    make(map[int][]models.LoadNotification),
	stopChan: make(chan struct{}),
}

type metricSnapshot struct {
	Latest        float32
	Average       float32
	Peak          float32
	ExceededRatio float32
}

// Reload 重载时间表
func (m *LoadNotificationService) Reload(loadNotifications []models.LoadNotification) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 停止所有现有定时器
	for _, ticker := range m.tickers {
		ticker.Stop()
	}
	m.tickers = make(map[int]*time.Ticker)
	m.tasks = make(map[int][]models.LoadNotification)

	// 按Interval分组任务
	taskGroups := make(map[int][]models.LoadNotification)
	for _, task := range loadNotifications {
		taskGroups[task.Interval] = append(taskGroups[task.Interval], task)
	}

	// 为每个唯一的Interval创建定时器
	for interval, tasks := range taskGroups {
		ticker := time.NewTicker(time.Duration(interval) * time.Minute)
		m.tickers[interval] = ticker
		m.tasks[interval] = tasks

		go func(ticker *time.Ticker, tasks []models.LoadNotification) {
			for {
				select {
				case <-ticker.C:
					for _, task := range tasks {
						go executeLoadNotificationTask(task)
					}
				case <-m.stopChan:
					return
				}
			}
		}(ticker, tasks)
	}

	return nil
}

// executeLoadNotificationTask 执行单个LoadNotificationTask
func executeLoadNotificationTask(task models.LoadNotification) {
	// 检查是否在冷却期内
	if shouldSkipNotification(task) {
		return
	}

	now := time.Now()
	windowStart := now.Add(-time.Duration(task.Interval) * time.Minute)
	overloadClients := make([]string, 0)
	snapshots := make(map[string]metricSnapshot)
	for _, clientUUID := range task.Clients {
		// 获取客户端在时间窗口内的记录
		records, err := getRecordsForClient(clientUUID, windowStart, now)
		if err != nil {
			continue
		}

		// 检查指标是否达到阈值
		triggered, snapshot := evaluateMetricThreshold(records, task)
		if triggered {
			overloadClients = append(overloadClients, clientUUID)
			snapshots[clientUUID] = snapshot
		}

	}
	sendLoadNotification(overloadClients, snapshots, task)
	updateLastNotified(task.Id, now)
}

// shouldSkipNotification 检查是否应该跳过通知（冷却期检查）
func shouldSkipNotification(task models.LoadNotification) bool {
	if task.LastNotified.ToTime().IsZero() {
		return false
	}

	// 计算冷却期（使用 interval 作为冷却期）
	cooldownPeriod := time.Duration(task.Interval) * time.Minute
	timeSinceLastNotified := time.Since(task.LastNotified.ToTime())

	return timeSinceLastNotified < cooldownPeriod
}

// getRecordsForClient 获取指定客户端在时间窗口内的记录
func getRecordsForClient(clientUUID string, start, end time.Time) ([]models.Record, error) {
	return records.GetRecordsByClientAndTime(clientUUID, start, end)
}

func evaluateMetricThreshold(records []models.Record, task models.LoadNotification) (bool, metricSnapshot) {
	if len(records) == 0 {
		return false, metricSnapshot{}
	}

	// 计算需要达标的最小记录数
	minRequiredRecords := int(float32(len(records)) * task.Ratio)
	if minRequiredRecords == 0 {
		minRequiredRecords = 1
	}

	exceededCount := 0
	var total float32
	var peak float32
	var latest float32
	var latestTime time.Time

	for _, record := range records {
		metricValue := getMetricValue(record, task.Metric)
		total += metricValue
		if metricValue > peak {
			peak = metricValue
		}
		recordTime := record.Time.ToTime()
		if latestTime.IsZero() || recordTime.After(latestTime) {
			latest = metricValue
			latestTime = recordTime
		}
		if metricValue >= task.Threshold {
			exceededCount++
		}
	}

	snapshot := metricSnapshot{
		Latest:        latest,
		Average:       total / float32(len(records)),
		Peak:          peak,
		ExceededRatio: float32(exceededCount) / float32(len(records)),
	}
	return exceededCount >= minRequiredRecords, snapshot
}

// getMetricValue 根据指标名称获取记录中的对应值
func getMetricValue(record models.Record, metric string) float32 {
	switch metric {
	case "cpu":
		return record.Cpu
	case "gpu":
		return record.Gpu
	case "net_in", "netin":
		return bytesPerSecondToMbps(record.NetIn)
	case "net_out", "netout":
		return bytesPerSecondToMbps(record.NetOut)
	case "ram":
		client, err := clients.GetClientByUUID(record.Client) // 确保客户端信息已加载
		if err != nil {
			log.Printf("Failed to get client info for %s: %v", record.Client, err)
			return 0
		}
		if record.RamTotal > 0 {
			return float32(record.Ram) / float32(client.MemTotal) * 100
		}
		return 0
	case "swap":
		client, err := clients.GetClientByUUID(record.Client) // 确保客户端信息已加载
		if err != nil {
			log.Printf("Failed to get client info for %s: %v", record.Client, err)
			return 0
		}
		if record.SwapTotal > 0 {
			return float32(record.Swap) / float32(client.SwapTotal) * 100
		}
		return 0
	case "load":
		return record.Load
	case "temp":
		return record.Temp
	case "disk":
		client, err := clients.GetClientByUUID(record.Client) // 确保客户端信息已加载
		if err != nil {
			log.Printf("Failed to get client info for %s: %v", record.Client, err)
			return 0
		}
		if record.DiskTotal > 0 {
			return float32(record.Disk) / float32(client.DiskTotal) * 100
		}
		return 0
	default:
		// 尝试通过反射获取字段值
		v := reflect.ValueOf(record)
		field := v.FieldByName(metric)
		if field.IsValid() && field.CanInterface() {
			switch field.Kind() {
			case reflect.Float32:
				return float32(field.Float())
			case reflect.Float64:
				return float32(field.Float())
			case reflect.Int, reflect.Int32, reflect.Int64:
				return float32(field.Int())
			}
		}
		return 0
	}
}

func bytesPerSecondToMbps(bytesPerSecond int64) float32 {
	if bytesPerSecond <= 0 {
		return 0
	}

	// 采用十进制 Mbps：1 Mbps = 1,000,000 bit/s
	return float32(float64(bytesPerSecond) * 8 / 1_000_000)
}

// sendLoadNotification 发送负载通知
func sendLoadNotification(clientUUIDs []string, snapshots map[string]metricSnapshot, task models.LoadNotification) {
	ex_clients := []models.Client{}
	var diagnostics strings.Builder
	for _, clientUUID := range clientUUIDs {
		cl, err := clients.GetClientByUUID(clientUUID)
		if err == nil {
			ex_clients = append(ex_clients, cl)
			if snapshot, ok := snapshots[clientUUID]; ok {
				fmt.Fprintf(&diagnostics, "• %s：最新 %s｜平均 %s｜峰值 %s｜超限 %.0f%%\n",
					cl.Name,
					formatMetricValue(task.Metric, snapshot.Latest),
					formatMetricValue(task.Metric, snapshot.Average),
					formatMetricValue(task.Metric, snapshot.Peak),
					snapshot.ExceededRatio*100,
				)
			}
		}
	}
	if len(ex_clients) == 0 {
		return
	}
	go func() {
		messageSender.SendEvent(models.EventMessage{
			Event:   messageevent.Alert,
			Clients: ex_clients,
			Time:    time.Now(),
			Emoji:   "⚠️",
			Details: []models.EventMessageField{
				{Label: "告警规则", Value: task.Name},
				{Label: "监控指标", Value: metricDisplayName(task.Metric)},
				{Label: "触发条件", Value: fmt.Sprintf("指标 ≥ %s，且窗口内超限占比 ≥ %.0f%%", formatMetricValue(task.Metric, task.Threshold), task.Ratio*100)},
				{Label: "检测窗口", Value: fmt.Sprintf("最近 %d 分钟", task.Interval)},
				{Label: "触发数量", Value: fmt.Sprintf("%d 台服务器", len(ex_clients))},
			},
			Message: strings.TrimSpace(diagnostics.String()),
		})
	}()
}

func metricDisplayName(metric string) string {
	switch strings.ToLower(metric) {
	case "cpu":
		return "CPU 使用率"
	case "gpu":
		return "GPU 使用率"
	case "ram":
		return "内存使用率"
	case "swap":
		return "Swap 使用率"
	case "disk":
		return "磁盘使用率"
	case "net_in", "netin":
		return "入站带宽"
	case "net_out", "netout":
		return "出站带宽"
	case "load":
		return "系统负载"
	case "temp":
		return "温度"
	default:
		return metric
	}
}

func formatMetricValue(metric string, value float32) string {
	switch strings.ToLower(metric) {
	case "cpu", "gpu", "ram", "swap", "disk":
		return fmt.Sprintf("%.1f%%", value)
	case "net_in", "netin", "net_out", "netout":
		return fmt.Sprintf("%.2f Mbps", value)
	case "temp":
		return fmt.Sprintf("%.1f°C", value)
	default:
		return fmt.Sprintf("%.2f", value)
	}
}

// updateLastNotified 更新最后通知时间
func updateLastNotified(taskId uint, notifyTime time.Time) {
	db := dbcore.GetDBInstance()
	if err := db.Model(&models.LoadNotification{}).Where("id = ?", taskId).Update("last_notified", notifyTime).Error; err != nil {
		log.Printf("Failed to update last_notified for task %d: %v", taskId, err)
	}
}

// ReloadLoadNotificationSchedule 加载或重载时间表
func ReloadLoadNotificationSchedule(loadNotifications []models.LoadNotification) error {
	return LoadNotificationManager.Reload(loadNotifications)
}
