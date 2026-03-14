package config

import "time"

type Glossary struct {
	Terms []Term `json:"terms"`
}

type Term struct {
	Term       string    `json:"term"`
	Definition string    `json:"definition"`
	AddedBy    string    `json:"added_by"`
	AddedAt    time.Time `json:"added_at"`
}
