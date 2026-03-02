package model

import "time"

type PredictionSource string

const (
	PredictionSourceSync  PredictionSource = "sync"
	PredictionSourceSleep PredictionSource = "sleep"
)

type PredictionErrorLog struct {
	ID             int64
	Source         PredictionSource
	Metric         string
	PredictedValue float64
	ActualValue    float64
	ErrorValue     float64
	PriorityDelta  int
	CreatedAt      time.Time
}
