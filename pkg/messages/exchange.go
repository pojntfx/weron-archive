package messages

type Exchange struct {
	Message

	Src     string `json:"src"`
	Dst     string `json:"dst"`
	Payload string `json:"payload"`
}

func NewOffer(src string, dst string, payload string) *Exchange {
	return &Exchange{
		Message: Message{
			Type: MessageTypeOffer,
		},

		Src:     src,
		Dst:     dst,
		Payload: payload,
	}
}

func NewAnswer(src string, dst string, payload string) *Exchange {
	return &Exchange{
		Message: Message{
			Type: MessageTypeAnswer,
		},

		Src:     src,
		Dst:     dst,
		Payload: payload,
	}
}

func NewCandidate(src string, dst string, payload string) *Exchange {
	return &Exchange{
		Message: Message{
			Type: MessageTypeCandidate,
		},

		Src:     src,
		Dst:     dst,
		Payload: payload,
	}
}
