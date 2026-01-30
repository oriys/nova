package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

type Event struct {
	SleepSeconds int `json:"sleep_seconds"`
}

type Response struct {
	RequestedSleep int     `json:"requested_sleep"`
	ActualSleep    float64 `json:"actual_sleep"`
	Status         string  `json:"status"`
}

func handler(event Event) Response {
	sleepSec := event.SleepSeconds
	if sleepSec == 0 {
		sleepSec = 5
	}

	start := time.Now()
	time.Sleep(time.Duration(sleepSec) * time.Second)
	elapsed := time.Since(start).Seconds()

	return Response{
		RequestedSleep: sleepSec,
		ActualSleep:    float64(int(elapsed*100)) / 100,
		Status:         "completed",
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
	json.Unmarshal(data, &event)

	result := handler(event)
	output, _ := json.Marshal(result)
	fmt.Println(string(output))
}
