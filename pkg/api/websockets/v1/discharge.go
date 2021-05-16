package api

type Resignation struct {
	Message

	Mac string `json:"mac"`
}

func NewExited() *Message {
	return &Message{TypeExited}
}

func NewResignation(mac string) *Resignation {
	return &Resignation{
		Message: Message{TypeResignation},

		Mac: mac,
	}
}
