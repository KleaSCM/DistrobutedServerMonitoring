package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"sync"
	"time"
)

// ServerStats represents the CPU, Memory, and Disk usage for a server
type ServerStats struct {
	CPUUsage    float64 `json:"cpu_usage"`
	MemoryUsage float64 `json:"memory_usage"`
	DiskUsage   float64 `json:"disk_usage"`
}

// StatsMap holds the aggregated stats for all agents
var (
	statsLock sync.Mutex
	statsMap  map[string]ServerStats
)

// startController starts the controller server to collect stats from agents
func startController() {
	http.HandleFunc("/update", updateStats)   // Endpoint for receiving agent updates
	http.HandleFunc("/stats", displayStats)   // Endpoint for showing all stats
	http.HandleFunc("/reset", resetStats)     // Reset all collected stats
	log.Println("Controller started, listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

// updateStats handles incoming updates from agents
func updateStats(w http.ResponseWriter, r *http.Request) {
	var stats ServerStats
	agentID := r.URL.Query().Get("agent")

	// Parse JSON body
	err := json.NewDecoder(r.Body).Decode(&stats)
	if err != nil {
		// ERROR HANDLING: Invalid data from agent
		http.Error(w, "Invalid data format", http.StatusBadRequest)
		log.Printf("Error decoding stats from agent %s: %v\n", agentID, err)
		return
	}
	// ERROR HANDLING ENDS

	// Lock to safely update shared stats map
	statsLock.Lock()
	statsMap[agentID] = stats
	statsLock.Unlock()

	log.Printf("Received stats from %s: %+v\n", agentID, stats)
	w.WriteHeader(http.StatusOK)
}

// displayStats returns aggregated stats from all agents as JSON
func displayStats(w http.ResponseWriter, r *http.Request) {
	statsLock.Lock()
	defer statsLock.Unlock()

	// ERROR HANDLING: If no stats have been collected yet
	if len(statsMap) == 0 {
		http.Error(w, "No stats available", http.StatusNoContent)
		log.Println("No stats available to display")
		return
	}
	// ERROR HANDLING ENDS

	// Return aggregated stats as JSON
	err := json.NewEncoder(w).Encode(statsMap)
	if err != nil {
		// ERROR HANDLING: JSON encoding failure
		http.Error(w, "Failed to encode stats", http.StatusInternalServerError)
		log.Printf("Error encoding stats: %v\n", err)
		return
	}
	// ERROR HANDLING ENDS
}

// resetStats resets all the collected stats (admin endpoint)
func resetStats(w http.ResponseWriter, r *http.Request) {
	statsLock.Lock()
	defer statsLock.Unlock()

	// Clear all stats
	statsMap = make(map[string]ServerStats)

	// Acknowledge reset
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Stats reset"))
	log.Println("All stats have been reset")
}

// startAgent simulates an agent that sends stats to the controller
func startAgent(agentID string) {
	for {
		stats := generateRandomStats()

		// Try to send stats, with retry logic
		err := retry(3, 2*time.Second, func() error {
			return sendStats(agentID, stats)
		})

		// ERROR HANDLING: If sending stats failed after retries
		if err != nil {
			log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
		}
		// ERROR HANDLING ENDS

		// Sleep before sending the next set of stats
		time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
	}
}

// generateRandomStats simulates random CPU, Memory, and Disk usage
func generateRandomStats() ServerStats {
	return ServerStats{
		CPUUsage:    rand.Float64() * 100,
		MemoryUsage: rand.Float64() * 100,
		DiskUsage:   rand.Float64() * 100,
	}
}

// sendStats sends the simulated server stats to the controller
func sendStats(agentID string, stats ServerStats) error {
	// Serialize the stats to JSON
	data, err := json.Marshal(stats)
	if err != nil {
		// ERROR HANDLING: JSON marshaling failure
		log.Printf("Error marshaling stats from %s: %v\n", agentID, err)
		return err
	}
	// ERROR HANDLING ENDS

	// Send HTTP POST request to the controller
	url := fmt.Sprintf("http://localhost:8080/update?agent=%s", agentID)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(data))
	if err != nil {
		// ERROR HANDLING: HTTP request failure
		log.Printf("Error sending stats from %s: %v\n", agentID, err)
		return err
	}
	defer resp.Body.Close()

	// ERROR HANDLING: Non-200 status codes
	if resp.StatusCode != http.StatusOK {
		body, _ := ioutil.ReadAll(resp.Body)
		err := fmt.Errorf("received non-OK response: %d - %s", resp.StatusCode, string(body))
		log.Printf("Error in response from server: %v\n", err)
		return err
	}
	// ERROR HANDLING ENDS

	log.Printf("Successfully sent stats from %s\n", agentID)
	return nil
}

// retry retries a function n times with a delay between each attempt
func retry(attempts int, delay time.Duration, fn func() error) error {
	var err error
	for i := 0; i < attempts; i++ {
		err = fn()
		if err == nil {
			return nil
		}

		// Sleep before the next attempt
		time.Sleep(delay)
	}
	return err
}

// logToFile sets up logging to a file instead of stdout
func logToFile(filename string) {
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("Failed to open log file: %v\n", err)
	}

	// Log to both file and console
	log.SetOutput(file)
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	log.Println("Logging to file:", filename)
}

func main() {
	// Initialize log file
	logToFile("controller.log")

	// Initialize the stats map
	statsMap = make(map[string]ServerStats)

	// Start the controller server
	go startController()

	// Simulate multiple agents
	for i := 1; i <= 5; i++ {
		go startAgent(fmt.Sprintf("Agent-%d", i))
	}

	// Keep the program running indefinitely
	select {}
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// exponentialBackoffRetry retries the function with increasing delays
func exponentialBackoffRetry(attempts int, baseDelay time.Duration, fn func() error) error {
    var err error
    delay := baseDelay
    for i := 0; i < attempts; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        log.Printf("Attempt %d failed, retrying in %v...\n", i+1, delay)
        time.Sleep(delay)
        delay *= 2 // Exponentially increase the delay
    }
    return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

// Updated startAgent function with exponential backoff retry
func startAgentWithBackoff(agentID string) {
    for {
        stats := generateRandomStats()

        // Retry with exponential backoff for up to 5 attempts
        err := exponentialBackoffRetry(5, 1*time.Second, func() error {
            return sendStats(agentID, stats)
        })

        // ERROR HANDLING: If sending stats failed after retries
        if err != nil {
            log.Printf("Failed to send stats from %s after retries: %v\n", agentID, err)
        }
        // ERROR HANDLING ENDS

        // Sleep before sending the next set of stats
        time.Sleep(time.Duration(rand.Intn(5)+5) * time.Second)
    }
}

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code

// Add some random change to the file
// Placeholder code
