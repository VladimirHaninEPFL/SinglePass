package main

import (
	"checklist/pir"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
)

type demoConfig struct {
	name       string
	numEntries int
	entrySize  int
}

// Message type tags sent over the socket
const (
	MsgHintReq   uint8 = 1
	MsgAnswerReq uint8 = 2
)

func main() {
	gob.Register(pir.SinglePassHintReq{})
	gob.Register(pir.SinglePassHintResp{})
	gob.Register(pir.SinglePassQueryReq{})
	gob.Register(pir.SinglePassQueryResp{})

	if len(os.Args) < 2 {
		log.Fatalf("usage: server [left|right]")
	}
	role := os.Args[1] // "left" or "right"
	if role != "left" && role != "right" {
		log.Fatalf("role must be 'left' or 'right', received %s", role)
	}

	config := demoConfig{
		name:       "node0",
		numEntries: 55_000,
		entrySize:  28,
	}
	params, err := pir.EstimateSinglePassParams(config.numEntries, config.entrySize)
	if err != nil {
		log.Fatalf("failed to derive SinglePass parameters: %v", err)
	}

	// Build the database (same seed on both sides so they hold identical data)
	random := rand.New(rand.NewSource(42))
	rows := makeDemoRows(random, params.NumRows, params.PaddedRows, params.RowLen)
	db := *pir.StaticDBFromRows(rows)
	fmt.Printf("[%s] database ready (%d rows x %d bytes)\n", role, params.NumRows, params.RowLen)

	socketPath := fmt.Sprintf("/tmp/SinglePass-%s.sock", role)
	os.Remove(socketPath) // clean up stale socket if any

	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		log.Fatalf("failed to listen on %s: %v", socketPath, err)
	}
	fmt.Printf("[%s] listening on %s\n", role, socketPath)

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("[%s] accept failed: %v", role, err)
			continue
		}
		fmt.Printf("[%s] client connected\n", role)

		dec := gob.NewDecoder(conn)
		enc := gob.NewEncoder(conn)

		for {
			// Read the message type tag
			var msgType uint8
			if err := binary.Read(conn, binary.LittleEndian, &msgType); err != nil {
				fmt.Printf("[%s] connection closed: %v\n", role, err)
				break
			}

			switch msgType {

			case MsgHintReq:

				var req pir.SinglePassHintReq
				if err := dec.Decode(&req); err != nil {
					log.Fatalf("[%s] failed to decode HintReq: %v", role, err)
				}
				fmt.Printf("[%s] processing HintReq\n", role)

				var resp pir.HintResp
				if err := db.Hint(&req, &resp); err != nil {
					log.Fatalf("[%s] hint generation failed: %v", role, err)
				}
				if err := enc.Encode(resp); err != nil {
					log.Fatalf("[%s] failed to send HintResp: %v", role, err)
				}

			case MsgAnswerReq:

				var req pir.SinglePassQueryReq
				if err := dec.Decode(&req); err != nil {
					log.Fatalf("[%s] failed to decode QueryReq: %v", role, err)
				}
				fmt.Printf("[%s] processing AnswerReq\n", role)

				var respIface interface{} = &pir.SinglePassQueryResp{}
				if err := db.Answer(&req, &respIface); err != nil {
					log.Fatalf("[%s] answer failed: %v", role, err)
				}
				resp := respIface.(*pir.SinglePassQueryResp)
				if err := enc.Encode(resp); err != nil {
					log.Fatalf("[%s] failed to send answer: %v", role, err)
				}

			default:
				log.Fatalf("[%s] unknown message type: %d", role, msgType)
			}
		}

		conn.Close()
		fmt.Printf("[%s] client disconnected\n", role)
	}
}

func makeDemoRows(src *rand.Rand, numRows, paddedRows, rowLen int) []pir.Row {
	rows := pir.MakeRows(src, numRows, rowLen)
	for len(rows) < paddedRows {
		rows = append(rows, make(pir.Row, rowLen))
	}
	return rows
}
