package api

type application struct {
	message
	Community string `json:"community"`
	Mac       string `json:"mac"`
}

type introduction struct {
	message
	Mac string `json:"mac"`
}

func NewApplication(community, mac string) *application {
	return &application{
		message:   message{TypeApplication},
		Community: community,
		Mac:       mac,
	}
}

func NewAcceptance() *message {
	return &message{TypeAcceptance}
}

func NewRejection() *message {
	return &message{TypeRejection}
}

func NewReady() *message {
	return &message{TypeReady}
}

func NewIntroduction(mac string) *introduction {
	return &introduction{
		message: message{TypeIntroduction},
		Mac:     mac,
	}
}
