package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"time"
)

type Event struct {
	Limit int `json:"limit"`
}

type Response struct {
	Limit     int     `json:"limit"`
	Count     int     `json:"count"`
	Last10    []int   `json:"last_10"`
	ElapsedMs int64   `json:"elapsed_ms"`
}

func isPrime(n int) bool {
	if n < 2 {
		return false
	}
	sqrt := int(math.Sqrt(float64(n)))
	for i := 2; i <= sqrt; i++ {
		if n%i == 0 {
			return false
		}
	}
	return true
}

func handler(event Event) Response {
	limit := event.Limit
	if limit == 0 {
		limit = 10000
	}

	start := time.Now()

	var primes []int
	for n := 2; n <= limit; n++ {
		if isPrime(n) {
			primes = append(primes, n)
		}
	}

	elapsed := time.Since(start).Milliseconds()

	last10 := primes
	if len(primes) > 10 {
		last10 = primes[len(primes)-10:]
	}

	return Response{
		Limit:     limit,
		Count:     len(primes),
		Last10:    last10,
		ElapsedMs: elapsed,
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
