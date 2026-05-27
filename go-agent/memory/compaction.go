package memory

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"
)

// Compactor负责记忆压缩
type Compactor struct {
	shortTerm ShortTermMemory
	longTerm  LongTermMemory
}

// NewCompactor创建一个新的Compactor
func NewCompactor(shortTerm ShortTermMemory, longTerm LongTermMemory) *Compactor {
	return &Compactor{
		shortTerm: shortTerm,
		longTerm:  longTerm,
	}
}

// Compact压缩过期的会话历史
func (c *Compactor) Compact(ctx context.Context, olderThan time.Duration) error {
	// 找出所有会话
	sessions, err := c.shortTerm.ListSessions(ctx, "")
	if err != nil {
		return fmt.Errorf("listing sessions: %w", err)
	}

	for _, sid := range sessions {
		interactions, err := c.shortTerm.Load(ctx, sid, 0)
		if err != nil {
			slog.Warn("failed to load session", "session", sid, "error", err)
			continue
		}

		// 检查是否过期
		if len(interactions) == 0 {
			continue
		}

		// 获取最后一条消息的时间
		lastInteraction := interactions[len(interactions)-1]
		timestamp, ok := lastInteraction.Metadata["timestamp"].(time.Time)
		if !ok {
			continue
		}

		if time.Since(timestamp) < olderThan {
			continue
		}

		// 生成摘要
		summary := fmt.Sprintf("会话摘要: %d条消息", len(interactions))

		// 存储到长期记忆
		fact := Fact{
			ID:         fmt.Sprintf("session_summary_%s", sid),
			Key:        fmt.Sprintf("session_%s", sid),
			Content:    summary,
			Source:     "derived",
			CreatedAt:  time.Now(),
			DecayScore: 1.0,
		}

		if err := c.longTerm.Store(ctx, fact); err != nil {
			slog.Warn("failed to store summary", "session", sid, "error", err)
			continue
		}

		// 删除原始会话
		if err := c.shortTerm.DeleteSession(ctx, sid); err != nil {
			slog.Warn("failed to delete session", "session", sid, "error", err)
		}

		slog.Info("compacted session", "session", sid)
	}

	return nil
}

// DecayCalculator计算记忆衰减
type DecayCalculator struct {
	halfLife time.Duration // 半衰期，默认30天
}

// Calculate计算给定时间的衰减分数
func (d *DecayCalculator) Calculate(createdAt time.Time) float64 {
	age := time.Since(createdAt)
	// 使用指数衰减公式：score = 0.5^(age/halfLife)
	halfLifeHours := d.halfLife.Hours()
	ageHours := age.Hours()

	if halfLifeHours == 0 {
		return 1.0
	}

	return math.Pow(0.5, ageHours/halfLifeHours)
}
