package messages

type Acknowledgement struct {
	Message

	Mac string `json:"mac"`
}

func NewAcknowledgement(mac string) *Acknowledgement {
	return &Acknowledgement{
		Message: Message{
			Type: MessageTypeAcknowledgement,
		},

		Mac: mac,
	}
}
