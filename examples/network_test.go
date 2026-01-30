package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

type Event struct {
	URL     string `json:"url"`
	Timeout int    `json:"timeout"`
}

type Response struct {
	URL       string      `json:"url"`
	Status    int         `json:"status"`
	ElapsedMs int64       `json:"elapsed_ms"`
	Response  interface{} `json:"response"`
}

func handler(event Event) Response {
	url := event.URL
	if url == "" {
		url = "https://httpbin.org/get"
	}
	timeout := event.Timeout
	if timeout == 0 {
		timeout = 10
	}

	client := &http.Client{
		Timeout: time.Duration(timeout) * time.Second,
	}

	start := time.Now()

	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "Nova/1.0")

	resp, err := client.Do(req)
	elapsed := time.Since(start).Milliseconds()

	if err != nil {
		return Response{
			URL:       url,
			Status:    0,
			ElapsedMs: elapsed,
			Response:  map[string]string{"error": err.Error()},
		}
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var data interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		// Not JSON, return truncated string
		s := string(body)
		if len(s) > 500 {
			s = s[:500]
		}
		data = s
	}

	return Response{
		URL:       url,
		Status:    resp.StatusCode,
		ElapsedMs: elapsed,
		Response:  data,
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
