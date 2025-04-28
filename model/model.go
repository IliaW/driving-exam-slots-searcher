package model

import "time"

type Task struct {
	ExamDate  string
	Address   string
	Found     bool
	Ttl       time.Duration
	UpdatedAt time.Time
}

type Notification struct {
	Topic    string
	Title    string
	Tags     []string
	Message  string
	Filename string
	Priority int
}
