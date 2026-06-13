package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"math/rand"
	"net"

	"checklist/pir"
)

type demoConfig struct {
	name       string
	numEntries int
	entrySize  int
}

func main() {

	config := demoConfig{
		name:       "node0",
		numEntries: 55_000,
		entrySize:  28,
	}

	params, err := pir.EstimateSinglePassParams(config.numEntries, config.entrySize)
	if err != nil {
		log.Fatalf("failed to derive SinglePass parameters: %v", err)
	}

	// Demo RNG for reproducible behavior.
	random := rand.New(rand.NewSource(42))

	// ==== server side

	// Build one logical database, pad with dummy rows if needed, then replicate it to two PIR servers.
	rows := makeDemoRows(random, params.NumRows, params.PaddedRows, params.RowLen)
	leftServer := *pir.StaticDBFromRows(rows)
	rightServer := *pir.StaticDBFromRows(rows)
	fmt.Printf("finished created db\n")

	// ===== OFFLINE PHASE client side =====

	// Client asks one server for the SinglePass preprocessing hint.
	hintReq := pir.NewHintReq(random, pir.SinglePass, params.SetSize)

	var hintResp pir.HintResp
	if err := leftServer.Hint(hintReq, &hintResp); err != nil {
		log.Fatalf("hint generation failed: %v", err)
	}

	// Client stores the offline hint/state locally.
	client := hintResp.InitClient(random)

	// ===== ONLINE PHASE =====

	fmt.Println("waiting for connection")
	ln, _ := net.Listen("unix", "/tmp/SinglePass.sock")
	conn, _ := ln.Accept() // one persistent Rust connection

	for {

		// read index from Rust
		var target int32
		err := binary.Read(conn, binary.LittleEndian, &target)
		if err != nil {
			// Rust disconnected (or read error), stop the loop
			fmt.Println("connection closed, waiting for new connection")
			conn, _ = ln.Accept()
			continue
		}

		queries, reconstruct := client.Query(int(target))

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
		if !bytes.Equal(row, leftServer.Row(int(target))) {
			log.Fatalf("wrong row returned")
		}

		// send row bytes back to Rust
		fmt.Printf("Sending result back to rust\n")
		binary.Write(conn, binary.LittleEndian, int32(len(row)))
		conn.Write(row)
	}
}

func makeDemoRows(src *rand.Rand, numRows, paddedRows, rowLen int) []pir.Row {
	rows := pir.MakeRows(src, numRows, rowLen)

	for len(rows) < paddedRows {
		rows = append(rows, make(pir.Row, rowLen))
	}

	return rows
}
