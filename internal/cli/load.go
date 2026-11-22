package cli

import (
	"fmt"
	"net"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"gostman/internal/collection"
	"gostman/internal/expect"
	"gostman/internal/httpclient"
)

const (
	maxLoadRPS        = 100000
	maxLoadClients    = 20000
	maxLoadDuration   = 30000 * time.Second
	maxRemoteRPS      = 10000
	maxRemoteClients  = 2000
	maxRemoteDuration = 10000 * time.Second
)

type loadStats struct {
	requests     int
	errors       int
	checksPassed int
	checksFailed int
	statuses     map[int]int
	totalLatency time.Duration
	minLatency   time.Duration
	maxLatency   time.Duration
}

func runLoad(cmd *cobra.Command, cases []collection.Case, suite *expect.Suite, options *sendOptions, hasChecks bool) error {
	remote := false
	for _, testCase := range cases {
		local, err := isLocalTarget(testCase.Request.URL)
		if err != nil {
			return err
		}
		remote = remote || !local
	}
	maxRPS, maxClients, maxDuration := maxLoadRPS, maxLoadClients, maxLoadDuration
	if remote {
		maxRPS, maxClients, maxDuration = maxRemoteRPS, maxRemoteClients, maxRemoteDuration
	}
	if options.rps < 1 || options.rps > maxRPS {
		return fmt.Errorf("--rps must be between 1 and %d for this target", maxRPS)
	}
	if options.clients < 1 || options.clients > maxClients {
		return fmt.Errorf("--clients must be between 1 and %d for this target", maxClients)
	}
	if options.duration <= 0 || options.duration > maxDuration {
		return fmt.Errorf("--duration must be between 1ns and %s for this target", maxDuration)
	}

	client := httpclient.NewWithoutRedirects(options.timeout)
	jobs := make(chan int)
	workerStats := make(chan loadStats, options.clients)
	var workers sync.WaitGroup
	workers.Add(options.clients)
	for range options.clients {
		go func() {
			defer workers.Done()
			stats := loadStats{statuses: make(map[int]int)}
			for index := range jobs {
				started := time.Now()
				response, err := client.Do(cmd.Context(), cases[index].Request)
				latency := time.Since(started)
				stats.requests++
				stats.totalLatency += latency
				if stats.minLatency == 0 || latency < stats.minLatency {
					stats.minLatency = latency
				}
				if latency > stats.maxLatency {
					stats.maxLatency = latency
				}
				if err != nil {
					stats.errors++
					continue
				}
				stats.statuses[response.StatusCode]++
				if hasChecks {
					for _, check := range suite.Check(index, response) {
						if check.Passed {
							stats.checksPassed++
						} else {
							stats.checksFailed++
						}
					}
				}
			}
			workerStats <- stats
		}()
	}

	start := time.Now()
	timer := time.NewTimer(options.duration)
	ticker := time.NewTicker(time.Second / time.Duration(options.rps))
	defer timer.Stop()
	defer ticker.Stop()
	skipped, next := 0, 0
	select {
	case jobs <- next:
		next = (next + 1) % len(cases)
	case <-cmd.Context().Done():
	}
schedule:
	for {
		select {
		case <-cmd.Context().Done():
			break schedule
		case <-timer.C:
			break schedule
		case <-ticker.C:
			select {
			case jobs <- next:
				next = (next + 1) % len(cases)
			default:
				skipped++
			}
		}
	}
	close(jobs)
	workers.Wait()
	close(workerStats)

	combined := loadStats{statuses: make(map[int]int)}
	for stats := range workerStats {
		combined.requests += stats.requests
		combined.errors += stats.errors
		combined.checksPassed += stats.checksPassed
		combined.checksFailed += stats.checksFailed
		combined.totalLatency += stats.totalLatency
		if stats.minLatency > 0 && (combined.minLatency == 0 || stats.minLatency < combined.minLatency) {
			combined.minLatency = stats.minLatency
		}
		if stats.maxLatency > combined.maxLatency {
			combined.maxLatency = stats.maxLatency
		}
		for code, count := range stats.statuses {
			combined.statuses[code] += count
		}
	}
	elapsed := time.Since(start)
	average := time.Duration(0)
	if combined.requests > 0 {
		average = combined.totalLatency / time.Duration(combined.requests)
	}
	out := cmd.OutOrStdout()
	if _, err := fmt.Fprintf(out, "Load: target=%d rps, clients=%d, elapsed=%s\nRequests: %d completed, %d skipped, %d errors\nThroughput: %.1f requests/s\nLatency: min=%s avg=%s max=%s\n", options.rps, options.clients, elapsed.Round(time.Millisecond), combined.requests, skipped, combined.errors, float64(combined.requests)/elapsed.Seconds(), combined.minLatency.Round(time.Millisecond), average.Round(time.Millisecond), combined.maxLatency.Round(time.Millisecond)); err != nil {
		return err
	}
	codes := make([]int, 0, len(combined.statuses))
	for code := range combined.statuses {
		codes = append(codes, code)
	}
	sort.Ints(codes)
	for _, code := range codes {
		if _, err := fmt.Fprintf(out, "Status %d: %d\n", code, combined.statuses[code]); err != nil {
			return err
		}
	}
	if hasChecks {
		if _, err := fmt.Fprintf(out, "Checks: %d passed, %d failed\n", combined.checksPassed, combined.checksFailed); err != nil {
			return err
		}
	}
	if combined.errors > 0 || combined.checksFailed > 0 {
		return fmt.Errorf("%w: %d request errors, %d failed checks", ErrRunFailed, combined.errors, combined.checksFailed)
	}
	return cmd.Context().Err()
}

func isLocalTarget(rawURL string) (bool, error) {
	target, err := url.Parse(rawURL)
	if err != nil || (target.Scheme != "http" && target.Scheme != "https") || target.Hostname() == "" {
		return false, fmt.Errorf("load mode requires an HTTP URL: %q", rawURL)
	}
	host := target.Hostname()
	if strings.EqualFold(host, "localhost") {
		return true, nil
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return true, nil
	}
	return false, nil
}
