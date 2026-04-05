package domain

import "html/template"

type PageData struct {
	Trade          Trade
	EntryTime      string
	ExitTime       string
	DirectionLabel string
	WindowSummary  string
	CandlesJSON    template.JS
	EntryIndex     int
	ExitIndex      int
	PriceMin       float64
	PriceMax       float64
	ContextBefore  int
	ContextAfter   int
	WindowStart    string
	WindowEnd      string
	GeneratedAt    string
}
