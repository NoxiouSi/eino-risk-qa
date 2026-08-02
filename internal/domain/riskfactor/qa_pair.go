package riskfactor

import "time"

// QAPair 某一轮的问答快照：round 0 为主问题，1~MaxRounds 为追问。
type QAPair struct {
	Round          int
	Question       string
	Answer         string
	Completeness   bool
	Reasonableness bool
	Judgements     []QuestionJudgement
	Answers        []QuestionAnswer
	CreatedAt      time.Time
}
