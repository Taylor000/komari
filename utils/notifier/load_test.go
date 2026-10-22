package notifier

import (
	"math"
	"testing"
	"time"

	"github.com/komari-monitor/komari/database/models"
)

func TestEvaluateMetricThresholdReturnsDiagnosticSnapshot(t *testing.T) {
	start := time.Date(2026, 8, 5, 14, 0, 0, 0, time.Local)
	records := []models.Record{
		{Time: models.FromTime(start), Cpu: 70},
		{Time: models.FromTime(start.Add(time.Minute)), Cpu: 90},
		{Time: models.FromTime(start.Add(2 * time.Minute)), Cpu: 100},
	}
	task := models.LoadNotification{Metric: "cpu", Threshold: 80, Ratio: 0.60}

	triggered, snapshot := evaluateMetricThreshold(records, task)
	if !triggered {
		t.Fatal("expected threshold to trigger")
	}
	if snapshot.Latest != 100 || snapshot.Peak != 100 {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
	if math.Abs(float64(snapshot.Average-86.6667)) > 0.01 {
		t.Fatalf("unexpected average: %f", snapshot.Average)
	}
	if math.Abs(float64(snapshot.ExceededRatio-0.6667)) > 0.01 {
		t.Fatalf("unexpected exceeded ratio: %f", snapshot.ExceededRatio)
	}
}
