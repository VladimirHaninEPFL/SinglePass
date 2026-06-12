package main

import (
	"bytes"
	"fmt"
	"log"
	"math/rand"
	"time"

	"checklist/pir"
)

type demoConfig struct {
	name       string
	numEntries int
	entrySize  int
}

func main() {

	franceConfigs := [...]demoConfig{
		{
			name:       "node0",
			numEntries: 5_196_479,
			entrySize:  28,
		},
		{
			name:       "node1",
			numEntries: 5_196_479,
			entrySize:  (1 + 4) * 28,
		},
		{
			name:       "node2",
			numEntries: 5_196_479,
			entrySize:  (1 + 4 + 4*4) * 28,
		},
		{
			name:       "node3",
			numEntries: 5_196_479,
			entrySize:  (1 + 4 + 4*4 + 4*4*4) * 28,
		},
		{
			name:       "block0.1",
			numEntries: 6810,
			entrySize:  12_745 * 48,
		},
		{
			name:       "block0.25",
			numEntries: 1170,
			entrySize:  63_245 * 48,
		},
		{
			name:       "block0.5",
			numEntries: 326,
			entrySize:  149_848 * 48,
		},
		{
			name:       "block1",
			numEntries: 96,
			entrySize:  241_882 * 48,
		},
	}

	config := franceConfigs[0]

	params, err := pir.EstimateSinglePassParams(config.numEntries, config.entrySize)
	if err != nil {
		log.Fatalf("failed to derive SinglePass parameters: %v", err)
	}

	// Demo RNG for reproducible behavior.
	random := rand.New(rand.NewSource(42))

	// Build one logical database, pad with dummy rows if needed, then replicate it to two PIR servers.
	rows := makeDemoRows(random, params.NumRows, params.PaddedRows, params.RowLen)
	leftServer := *pir.StaticDBFromRows(rows)
	rightServer := *pir.StaticDBFromRows(rows)
	fmt.Printf("finished created db\n")

	// ===== OFFLINE PHASE =====

	// Client asks one server for the SinglePass preprocessing hint.
	hintReq := pir.NewHintReq(random, pir.SinglePass, params.SetSize)

	var hintResp pir.HintResp
	if err := leftServer.Hint(hintReq, &hintResp); err != nil {
		log.Fatalf("hint generation failed: %v", err)
	}

	// Client stores the offline hint/state locally.
	client := hintResp.InitClient(random)

	// ===== ONLINE PHASE =====

	// Client creates one query for each real database entry.
	target := random.Intn(params.NumRows)
	queries, reconstruct := client.Query(target)

	// Each server answers only its own query.
	responses := make([]interface{}, 2)
	if err := leftServer.Answer(queries[pir.Left], &responses[pir.Left]); err != nil {
		log.Fatalf("left server answer failed: %v", err)
	}
	if err := rightServer.Answer(queries[pir.Right], &responses[pir.Right]); err != nil {
		log.Fatalf("right server answer failed: %v", err)
	}

	// Client reconstructs the requested row.
	row, err := reconstruct(responses)
	if err != nil {
		log.Fatalf("reconstruction failed: %v", err)
	}

	// Check correctness.
	if !bytes.Equal(row, leftServer.Row(target)) {
		log.Fatalf("wrong row returned")
	}
}

func makeDemoRows(src *rand.Rand, numRows, paddedRows, rowLen int) []pir.Row {
	rows := pir.MakeRows(src, numRows, rowLen)

	for len(rows) < paddedRows {
		rows = append(rows, make(pir.Row, rowLen))
	}

	return rows
}

func maxDuration(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}

func meanDurationSeconds(durations []time.Duration) float64 {
	if len(durations) == 0 {
		return 0
	}

	var total time.Duration
	for _, d := range durations {
		total += d
	}

	return total.Seconds() / float64(len(durations))
}

func singlePassResponsePayloadBytes(responses []interface{}) int {
	total := 0

	for _, response := range responses {
		singlePassResp, ok := response.(*pir.SinglePassQueryResp)
		if !ok {
			continue
		}
		total += len(singlePassResp.Answer)
	}

	return total
}
