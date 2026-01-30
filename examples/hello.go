package main

import (
	"encoding/json"
	"fmt"
	"os"
)

type Event struct {
	Name string `json:"name"`
}

type Response struct {
	Message string `json:"message"`
	Runtime string `json:"runtime"`
}

func handler(event Event) Response {
	name := event.Name
	if name == "" {
		name = "Anonymous"
	}
	return Response{
		Message: fmt.Sprintf("Hello, %s!", name),
		Runtime: "go",
	}
}

func main() {
	inputFile := "/tmp/input.json"
	if len(os.Args) > 1 {
		inputFile = os.Args[1]
	}

	data, err := os.ReadFile(inputFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
		os.Exit(1)
	}

	var event Event
	if err := json.Unmarshal(data, &event); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing input: %v\n", err)
		os.Exit(1)
	}

	result := handler(event)
	output, _ := json.Marshal(result)
	fmt.Println(string(output))
}
