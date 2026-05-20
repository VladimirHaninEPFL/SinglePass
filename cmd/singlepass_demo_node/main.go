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

	// switzerlandConfigs := [...]demoConfig{
	// 	{
	// 		name:       "node0",
	// 		numEntries: 416_799,
	// 		entrySize:  28,
	// 	},
	// 	{
	// 		name:       "node1",
	// 		numEntries: 416_799,
	// 		entrySize:  (1 + 4) * 28,
	// 	},
	// 	{
	// 		name:       "node2",
	// 		numEntries: 416_799,
	// 		entrySize:  (1 + 4 + 4*4) * 28,
	// 	},
	// 	{
	// 		name:       "node3",
	// 		numEntries: 416_799,
	// 		entrySize:  (1 + 4 + 4*4 + 4*4*4) * 28,
	// 	},
	// 	{
	// 		name:       "block0.1",
	// 		numEntries: 533,
	// 		entrySize:  306_480,
	// 	},
	// 	{
	// 		name:       "block0.25",
	// 		numEntries: 108,
	// 		entrySize:  1_128_000,
	// 	},
	// 	{
	// 		name:       "block0.5",
	// 		numEntries: 36,
	// 		entrySize:  2_426_928,
	// 	},
	// 	{
	// 		name:       "block1",
	// 		numEntries: 13,
	// 		entrySize:  5_670_960,
	// 	},
	// }

	franceConfigs := [...]demoConfig{
		// {
		// 	name:       "node0",
		// 	numEntries: 11_370_599,
		// 	entrySize:  28,
		// },
		// {
		// 	name:       "node1",
		// 	numEntries: 11_370_599,
		// 	entrySize:  (1 + 4) * 28,
		// },
		// {
		// 	name:       "node2",
		// 	numEntries: 11_370_599,
		// 	entrySize:  (1 + 4 + 4*4) * 28,
		// },
		{
			name:       "node3",
			numEntries: 11_370_599,
			entrySize:  (1 + 4 + 4*4 + 4*4*4) * 28,
		},
		// {
		// 	name:       "block0.1",
		// 	numEntries: 6810,
		// 	entrySize:  12_745 * 48,
		// },
		// {
		// 	name:       "block0.25",
		// 	numEntries: 1170,
		// 	entrySize:  63_245 * 48,
		// },
		// {
		// 	name:       "block0.5",
		// 	numEntries: 326,
		// 	entrySize:  149_848 * 48,
		// },
		// {
		// 	name:       "block1",
		// 	numEntries: 96,
		// 	entrySize:  241_882 * 48,
		// },
	}

	const numTrials = 1

	for _, config := range franceConfigs {

		params, err := pir.EstimateSinglePassParams(config.numEntries, config.entrySize)
		if err != nil {
			log.Fatalf("failed to derive SinglePass parameters: %v", err)
		}

		// fmt.Printf(
		// 	"singlepass_params = {num_entries: %d, padded_entries: %d, padding_entries: %d, entry_size: %d, setSize: %d, nHints: %d, offline_hint_bytes: %d, approx_client_state_bytes: %d, approx_online_bandwidth_bytes: %d}\n",
		// 	params.NumRows,
		// 	params.PaddedRows,
		// 	params.PaddingRows,
		// 	params.RowLen,
		// 	params.SetSize,
		// 	params.NHints,
		// 	params.OfflineHintBytes,
		// 	params.ClientStateBytes,
		// 	params.OnlineBandwidthBytes,
		// )

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

		trialDurations := make([]time.Duration, numTrials)
		onlineBytesPerQuery := 0

		for trial := 0; trial < numTrials; trial++ {

			// ===== ONLINE PHASE =====
			queryStart := time.Now()

			// Client creates one query for each real database entry.
			target := random.Intn(params.NumRows)
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
			onlineBytesPerQuery = singlePassResponsePayloadBytes(responses)

			// Client reconstructs the requested row.
			reconstructStart := time.Now()
			row, err := reconstruct(responses)
			if err != nil {
				log.Fatalf("reconstruction failed: %v", err)
			}
			reconstructDuration := time.Since(reconstructStart)

			// Check correctness.
			if !bytes.Equal(row, leftServer.Row(target)) {
				log.Fatalf("wrong row returned on trial %d", trial)
			}

			serverDuration := maxDuration(leftDuration, rightDuration)
			totalOnlineDuration := queryDuration + serverDuration + reconstructDuration
			trialDurations[trial] = totalOnlineDuration
		}

		fmt.Printf("approach: %s\n", config.name)
		fmt.Printf("mean_online_time_seconds = %.9f\n", meanDurationSeconds(trialDurations))
		fmt.Printf("server_to_client_online_bytes_per_query = %d\n", onlineBytesPerQuery)
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
