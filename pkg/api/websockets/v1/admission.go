package api

type Application struct {
	Message
	Community string `json:"community"`
	Mac       string `json:"mac"`
}

type Introduction struct {
	Message
	Mac string `json:"mac"`
}

func NewApplication(community, mac string) *Application {
	return &Application{
		Message:   Message{TypeApplication},
		Community: community,
		Mac:       mac,
	}
}

func NewAcceptance() *Message {
	return &Message{TypeAcceptance}
}

func NewRejection() *Message {
	return &Message{TypeRejection}
}

func NewReady() *Message {
	return &Message{TypeReady}
}

func NewIntroduction(mac string) *Introduction {
	return &Introduction{
		Message: Message{TypeIntroduction},
		Mac:     mac,
	}
}
