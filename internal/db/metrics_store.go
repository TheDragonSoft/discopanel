package db

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Metric sample history operations

// InsertMetricSamples batch-inserts metric sample records
func (s *Store) InsertMetricSamples(ctx context.Context, samples []*MetricSampleRecord) error {
	if len(samples) == 0 {
		return nil
	}
	return s.db.WithContext(ctx).CreateInBatches(samples, 500).Error
}

// ListMetricSamples returns samples for a server since the given time, ordered
// by timestamp ascending. If maxPoints > 0 and there are more samples than
// maxPoints, they are downsampled by averaging over evenly sized buckets.
func (s *Store) ListMetricSamples(ctx context.Context, serverID string, since time.Time, maxPoints int) ([]*MetricSampleRecord, error) {
	var samples []*MetricSampleRecord
	err := s.db.WithContext(ctx).
		Where("server_id = ? AND timestamp >= ?", serverID, since).
		Order("timestamp ASC").
		Find(&samples).Error
	if err != nil {
		return nil, err
	}

	if maxPoints > 0 && len(samples) > maxPoints {
		samples = downsampleMetricSamples(samples, maxPoints)
	}
	return samples, nil
}

// downsampleMetricSamples averages samples over evenly sized buckets to at
// most maxPoints entries, keeping the timestamp of the last sample per bucket.
func downsampleMetricSamples(samples []*MetricSampleRecord, maxPoints int) []*MetricSampleRecord {
	out := make([]*MetricSampleRecord, 0, maxPoints)
	bucketSize := float64(len(samples)) / float64(maxPoints)
	for i := 0; i < maxPoints; i++ {
		start := int(float64(i) * bucketSize)
		end := int(float64(i+1) * bucketSize)
		if end <= start {
			end = start + 1
		}
		if end > len(samples) {
			end = len(samples)
		}
		bucket := samples[start:end]

		avg := *bucket[len(bucket)-1] // keep identity + timestamp of the last sample
		n := float64(len(bucket))
		var (
			cpu, memUsed, memTotal, tps float64
			players                     int
			diskUsed, diskTotal         int64
		)
		for _, s := range bucket {
			cpu += s.CPUPercent
			memUsed += s.MemoryUsage
			memTotal += s.MemoryTotal
			tps += s.TPS
			players += s.PlayersOnline
			diskUsed += s.DiskUsage
			diskTotal += s.DiskTotal
		}
		avg.CPUPercent = cpu / n
		avg.MemoryUsage = memUsed / n
		avg.MemoryTotal = memTotal / n
		avg.TPS = tps / n
		avg.PlayersOnline = players / len(bucket)
		avg.DiskUsage = diskUsed / int64(len(bucket))
		avg.DiskTotal = diskTotal / int64(len(bucket))
		out = append(out, &avg)
	}
	return out
}

// PruneMetricSamples deletes samples older than the given time and returns how many were removed
func (s *Store) PruneMetricSamples(ctx context.Context, olderThan time.Time) (int64, error) {
	res := s.db.WithContext(ctx).Where("timestamp < ?", olderThan).Delete(&MetricSampleRecord{})
	return res.RowsAffected, res.Error
}

// Alert rule operations

// ListAlertRules returns alert rules. When serverID is non-empty, rules scoped
// to that server plus global rules (empty server_id) are returned.
func (s *Store) ListAlertRules(ctx context.Context, serverID string) ([]*AlertRule, error) {
	var rules []*AlertRule
	query := s.db.WithContext(ctx)
	if serverID != "" {
		query = query.Where("server_id = ? OR server_id = ''", serverID)
	}
	err := query.Order("created_at ASC").Find(&rules).Error
	return rules, err
}

// GetAlertRule returns a single alert rule by ID
func (s *Store) GetAlertRule(ctx context.Context, id string) (*AlertRule, error) {
	var rule AlertRule
	err := s.db.WithContext(ctx).First(&rule, "id = ?", id).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("alert rule not found")
		}
		return nil, err
	}
	return &rule, nil
}

// CreateAlertRule inserts a new alert rule
func (s *Store) CreateAlertRule(ctx context.Context, rule *AlertRule) error {
	if rule.ID == "" {
		rule.ID = uuid.New().String()
	}
	return s.db.WithContext(ctx).Create(rule).Error
}

// UpdateAlertRule saves changes to an existing alert rule
func (s *Store) UpdateAlertRule(ctx context.Context, rule *AlertRule) error {
	return s.db.WithContext(ctx).Save(rule).Error
}

// DeleteAlertRule removes an alert rule by ID
func (s *Store) DeleteAlertRule(ctx context.Context, id string) error {
	return s.db.WithContext(ctx).Delete(&AlertRule{}, "id = ?", id).Error
}

// Alert event operations

// InsertAlertEvent persists an alert firing/resolution record
func (s *Store) InsertAlertEvent(ctx context.Context, event *AlertEventRecord) error {
	if event.ID == "" {
		event.ID = uuid.New().String()
	}
	return s.db.WithContext(ctx).Create(event).Error
}

// ListAlertEvents returns recent alert events, newest first
func (s *Store) ListAlertEvents(ctx context.Context, serverID string, limit int) ([]*AlertEventRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	var events []*AlertEventRecord
	query := s.db.WithContext(ctx).Order("created_at DESC")
	if serverID != "" {
		query = query.Where("server_id = ?", serverID)
	}
	err := query.Limit(limit).Find(&events).Error
	return events, err
}
