package main

import (
	"checklist/pir"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strconv"
)

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

	// * parse command line argmuents
	if len(os.Args) != 5 {
		log.Fatalf("usage: singlepass-server <db-file> <num-rows> <row-size> <socket-path>")
	}

	dbPath := os.Args[1]
	numRows, err := strconv.Atoi(os.Args[2])
	if err != nil || numRows <= 0 {
		log.Fatalf("invalid num-rows value %q", os.Args[3])
	}
	rowSize, err := strconv.Atoi(os.Args[3])
	if err != nil || rowSize <= 0 {
		log.Fatalf("invalid row-size value %q", os.Args[4])
	}
	socketToRustServerPath := os.Args[4]

	// * load database
	rows, err := loadRowsFromFile(dbPath, numRows, rowSize)
	if err != nil {
		log.Fatalf("failed to load rows from %s: %v", dbPath, err)
	}
	db := *pir.StaticDBFromRows(rows)
	fmt.Printf("loaded database from %s (%d rows x %d bytes)\n", dbPath, numRows, rowSize)

	// * listen for connections
	os.Remove(socketToRustServerPath) // clean up stale socket if any

	ln, err := net.Listen("unix", socketToRustServerPath)
	if err != nil {
		log.Fatalf("failed to listen on %s: %v", socketToRustServerPath, err)
	}
	defer ln.Close()
	fmt.Printf("listening on %s\n", socketToRustServerPath)

	// only accept one rust server connection
	conn, err := ln.Accept()
	if err != nil {
		log.Printf("accept failed: %v", err)

	}
	defer conn.Close()
	fmt.Printf("client connected\n")

	dec := gob.NewDecoder(conn)
	enc := gob.NewEncoder(conn)

	// listen for as many messages from that rust-server until it closes conection
	for {

		var msgType uint8
		if err := binary.Read(conn, binary.LittleEndian, &msgType); err != nil {
			fmt.Printf("connection closed: %v\n", err)
			return
		}

		switch msgType {
		case MsgHintReq:
			var req pir.SinglePassHintReq
			if err := dec.Decode(&req); err != nil {
				log.Fatalf("failed to decode HintReq: %v", err)
			}
			fmt.Printf("processing HintReq\n")

			var resp pir.HintResp
			if err := db.Hint(&req, &resp); err != nil {
				log.Fatalf("hint generation failed: %v", err)
			}
			if err := enc.Encode(resp); err != nil {
				log.Fatalf("failed to send HintResp: %v", err)
			}

		case MsgAnswerReq:
			var req pir.SinglePassQueryReq
			if err := dec.Decode(&req); err != nil {
				log.Fatalf("failed to decode QueryReq: %v", err)
			}
			fmt.Printf("processing AnswerReq\n")

			var respIface interface{} = &pir.SinglePassQueryResp{}
			if err := db.Answer(&req, &respIface); err != nil {
				log.Fatalf("answer failed: %v", err)
			}
			resp := respIface.(*pir.SinglePassQueryResp)
			if err := enc.Encode(resp); err != nil {
				log.Fatalf("failed to send answer: %v", err)
			}

		default:
			log.Fatalf("unknown message type: %d", msgType)
		}
	}
}

func loadRowsFromFile(path string, numRows, rowSize int) ([]pir.Row, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return nil, err
	}

	expected := int64(numRows) * int64(rowSize)
	if stat.Size() != expected {
		return nil, fmt.Errorf("file size %d does not match expected %d (numRows=%d rowSize=%d)", stat.Size(), expected, numRows, rowSize)
	}

	data := make([]byte, expected)
	if _, err := io.ReadFull(f, data); err != nil {
		return nil, err
	}

	rows := make([]pir.Row, numRows)
	for i := 0; i < numRows; i++ {
		start := i * rowSize
		end := start + rowSize
		rows[i] = pir.Row(data[start:end])
	}
	return rows, nil
}
