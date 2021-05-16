package api

type Exchange struct {
	Message
	Mac     string `json:"mac"`
	Payload []byte `json:"payload"`
}

func NewOffer(mac string, payload []byte) *Exchange {
	return &Exchange{
		Message: Message{TypeOffer},
		Mac:     mac,
		Payload: payload,
	}
}

func NewAnswer(mac string, payload []byte) *Exchange {
	return &Exchange{
		Message: Message{TypeAnswer},
		Mac:     mac,
		Payload: payload,
	}
}

func NewCandidate(mac string, payload []byte) *Exchange {
	return &Exchange{
		Message: Message{TypeCandidate},
		Mac:     mac,
		Payload: payload,
	}
}
