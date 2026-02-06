package main

import (
	"encoding/json"
	"fmt"
)

type Event struct {
	Name string `json:"name"`
}

type Response struct {
	Message   string `json:"message"`
	Runtime   string `json:"runtime"`
	RequestID string `json:"request_id"`
}

func Handler(event json.RawMessage, ctx Context) (interface{}, error) {
	var e Event
	if err := json.Unmarshal(event, &e); err != nil {
		return nil, err
	}
	name := e.Name
	if name == "" {
		name = "Anonymous"
	}
	return Response{
		Message:   fmt.Sprintf("Hello, %s!", name),
		Runtime:   "go",
		RequestID: ctx.RequestID,
	}, nil
}
