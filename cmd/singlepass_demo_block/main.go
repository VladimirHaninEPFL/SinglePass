package main

import (
	"bytes"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"time"

	"checklist/pir"
)

func main() {
	const (
		nRows  = 108 // padded from 467344
		rowLen = 799_000

		setSize = 9

		numTrials = 100
	)

	// const (
	// 	nRows   = 467344
	// 	rowLen  = 34
	// 	setSize = 16
	// 	target  = 12345 // any index in [0, 467343]
	// )

	// Demo RNG for reproducible behavior.
	src := rand.New(rand.NewSource(42))

	// Build one logical database, then replicate it to two PIR servers.
	rows := pir.MakeRows(src, nRows, rowLen)
	leftServer := *pir.StaticDBFromRows(rows)
	rightServer := *pir.StaticDBFromRows(rows)

	// ===== OFFLINE PHASE =====

	// Client asks one server for the SinglePass preprocessing hint.
	hintReq := pir.NewHintReq(src, pir.SinglePass, setSize)

	var hintResp pir.HintResp
	if err := leftServer.Hint(hintReq, &hintResp); err != nil {
		log.Fatalf("hint generation failed: %v", err)
	}

	// Client stores the offline hint/state locally.
	client := hintResp.InitClient(src)

	trialDurations := make([]time.Duration, numTrials)

	for trial := 0; trial < numTrials; trial++ {

		// ===== ONLINE PHASE =====
		queryStart := time.Now()

		// Client creates one query for each server.
		target := src.Intn(nRows)
		queries, reconstruct := client.Query(target)

		queryDuration := time.Since(queryStart)

		// Each server answers only its own query.
		responses := make([]interface{}, 2)

		leftStart := time.Now()
		if err := leftServer.Answer(queries[pir.Left], &responses[pir.Left]); err != nil {
			log.Fatalf("left server answer failed: %v", err)
		}
		leftDuration := time.Since(leftStart)

		rightStart := time.Now()
		if err := rightServer.Answer(queries[pir.Right], &responses[pir.Right]); err != nil {
			log.Fatalf("right server answer failed: %v", err)
		}
		rightDuration := time.Since(rightStart)

		// Client reconstructs the requested row.
		reconstructStart := time.Now()
		row, err := reconstruct(responses)
		if err != nil {
			log.Fatalf("reconstruction failed: %v", err)
		}
		reconstructDuration := time.Since(reconstructStart)

		serverDuration := maxDuration(leftDuration, rightDuration)
		totalOnlineDuration := queryDuration + serverDuration + reconstructDuration
		trialDurations[trial] = totalOnlineDuration

		// Check correctness.
		if !bytes.Equal(row, leftServer.Row(target)) {
			log.Fatalf("wrong row returned on trial %d", trial)
		}
	}

	fmt.Printf("online_times_seconds = [%s]\n", formatDurationsSeconds(trialDurations))
}

func maxDuration(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}

func formatDurationsSeconds(durations []time.Duration) string {
	parts := make([]string, len(durations))

	for i, d := range durations {
		parts[i] = fmt.Sprintf("%.9f", d.Seconds())
	}

	return strings.Join(parts, ", ")
}
