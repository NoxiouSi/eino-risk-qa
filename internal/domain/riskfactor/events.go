package riskfactor

import "time"

// DomainEvent 领域事件的公共标记接口，供审计/扩展使用（当前仅作为可选能力保留，不参与核心状态机流转）。
type DomainEvent interface {
	OccurredAt() time.Time
}

// SessionCleared 会话已排除合理怀疑。
type SessionCleared struct {
	SessionID string
	At        time.Time
}

func (e SessionCleared) OccurredAt() time.Time { return e.At }

// SessionNotCleared 会话最终未排除合理怀疑。
type SessionNotCleared struct {
	SessionID string
	Reason    TerminationReason
	At        time.Time
}

func (e SessionNotCleared) OccurredAt() time.Time { return e.At }

// FollowUpRequested 生成了一次新的追问。
type FollowUpRequested struct {
	SessionID string
	Round     int
	Question  string
	At        time.Time
}

func (e FollowUpRequested) OccurredAt() time.Time { return e.At }
