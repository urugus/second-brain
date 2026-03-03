package model

import "time"

type Entity struct {
	ID             int64
	Kind           string
	CanonicalName  string
	NormalizedName string
	Strength       float64
	Salience       float64
	Status         string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
