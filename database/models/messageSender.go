package models

import "time"

type MessageSenderProvider struct {
	Name     string `json:"name" gorm:"primaryKey;unique;not null"`
	Addition string `json:"addition" gorm:"type:longtext" default:"{}"`
}

type EventMessage struct {
	Event   string              `json:"event"`
	Clients []Client            `json:"clients"`
	Time    time.Time           `json:"time"`
	Message string              `json:"message"`
	Emoji   string              `json:"emoji"`
	Details []EventMessageField `json:"details,omitempty"`
}

// EventMessageField carries structured diagnostic context for event-aware
// notification providers while remaining backwards compatible with the text
// template used by other providers.
type EventMessageField struct {
	Label string `json:"label"`
	Value string `json:"value"`
}
