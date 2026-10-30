package main

import (
	"encoding/json"

	"google.golang.org/protobuf/proto"
)

// pj renders a proto message as JSON so it can be embedded in composite responses.
func pj(m proto.Message) json.RawMessage {
	b, _ := marshal.Marshal(m)
	return b
}

func pjList[T proto.Message](list []T) []json.RawMessage {
	out := make([]json.RawMessage, 0, len(list))
	for _, m := range list {
		out = append(out, pj(m))
	}
	return out
}
