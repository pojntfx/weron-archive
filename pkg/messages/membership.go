package messages

type Membership struct {
	Message

	Mac string `json:"mac"`
}

func NewIntroduction(mac string) *Membership {
	return &Membership{
		Message: Message{
			Type: MessageTypeIntroduction,
		},

		Mac: mac,
	}
}

func NewResignation(mac string) *Membership {
	return &Membership{
		Message: Message{
			Type: MessageTypeResignation,
		},

		Mac: mac,
	}
}
