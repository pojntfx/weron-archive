package messages

type Application struct {
	Message

	Community string
}

func NewApplication(community string) *Application {
	return &Application{
		Message: Message{
			Type: MessageTypeApplication,
		},
		Community: community,
	}
}
