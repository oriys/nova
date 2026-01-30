package main

import (
	"encoding/json"
	"fmt"
	"os"
	"syscall"
	"time"
)

type Event struct {
	SizeKB     int `json:"size_kb"`
	Iterations int `json:"iterations"`
}

type Response struct {
	SizeKB             int     `json:"size_kb"`
	Iterations         int     `json:"iterations"`
	WriteTimesMs       []int64 `json:"write_times_ms"`
	ReadTimesMs        []int64 `json:"read_times_ms"`
	AvgWriteMs         float64 `json:"avg_write_ms"`
	AvgReadMs          float64 `json:"avg_read_ms"`
	WriteThroughputMBs float64 `json:"write_throughput_mbps"`
	ReadThroughputMBs  float64 `json:"read_throughput_mbps"`
}

func handler(event Event) Response {
	sizeKB := event.SizeKB
	if sizeKB == 0 {
		sizeKB = 1024 // 1MB default
	}
	iterations := event.Iterations
	if iterations == 0 {
		iterations = 10
	}

	data := make([]byte, sizeKB*1024)
	for i := range data {
		data[i] = 'x'
	}
	testFile := "/tmp/disk_test.bin"

	resp := Response{
		SizeKB:       sizeKB,
		Iterations:   iterations,
		WriteTimesMs: make([]int64, 0, iterations),
		ReadTimesMs:  make([]int64, 0, iterations),
	}

	for i := 0; i < iterations; i++ {
		// Write test
		start := time.Now()
		f, _ := os.Create(testFile)
		f.Write(data)
		f.Sync()
		syscall.Fsync(int(f.Fd()))
		f.Close()
		resp.WriteTimesMs = append(resp.WriteTimesMs, time.Since(start).Milliseconds())

		// Read test
		start = time.Now()
		os.ReadFile(testFile)
		resp.ReadTimesMs = append(resp.ReadTimesMs, time.Since(start).Milliseconds())
	}

	// Cleanup
	os.Remove(testFile)

	// Calculate averages
	var totalWrite, totalRead int64
	for i := 0; i < iterations; i++ {
		totalWrite += resp.WriteTimesMs[i]
		totalRead += resp.ReadTimesMs[i]
	}
	resp.AvgWriteMs = float64(totalWrite) / float64(iterations)
	resp.AvgReadMs = float64(totalRead) / float64(iterations)

	if resp.AvgWriteMs > 0 {
		resp.WriteThroughputMBs = float64(sizeKB) / 1024 / (resp.AvgWriteMs / 1000)
	}
	if resp.AvgReadMs > 0 {
		resp.ReadThroughputMBs = float64(sizeKB) / 1024 / (resp.AvgReadMs / 1000)
	}

	return resp
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
