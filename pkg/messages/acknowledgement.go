package messages

type Acknowledgement struct {
	Message

	Mac      string `json:"mac"`
	Rejected bool   `json:"rejected"`
}

func NewAcknowledgement(mac string, rejected bool) *Acknowledgement {
	return &Acknowledgement{
		Message: Message{
			Type: MessageTypeAcknowledgement,
		},

		Mac:      mac,
		Rejected: rejected,
	}
}
