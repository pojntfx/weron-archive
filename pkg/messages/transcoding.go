package messages

import "encoding/json"

func Encode(message interface{}) ([]byte, error) {
	return json.Marshal(message)
}

func Decode(encoded []byte, decoded interface{}) error {
	return json.Unmarshal(encoded, decoded)
}
