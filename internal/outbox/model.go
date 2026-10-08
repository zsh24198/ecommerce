package outbox

import "time"

// OutboxEvent Outbox 事件（append-only 日志，无软删）。
// published_at 为 NULL 表示未投递，Relay 依据它轮询。
type OutboxEvent struct {
	ID            uint64     `gorm:"primaryKey;autoIncrement" json:"id"`
	AggregateType string     `gorm:"size:32;not null" json:"aggregate_type"`
	AggregateID   string     `gorm:"size:64;not null" json:"aggregate_id"`
	EventType     string     `gorm:"size:64;not null" json:"event_type"`
	Payload       string     `gorm:"type:json;not null" json:"payload"`
	PublishedAt   *time.Time `json:"published_at"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// TableName 显式指定表名。
func (OutboxEvent) TableName() string { return "outbox_events" }
