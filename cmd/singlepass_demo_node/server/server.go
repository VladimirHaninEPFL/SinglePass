package main

import (
	"bytes"
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
	MsgHintReq uint8 = 1
	MsgDBQuery uint8 = 2
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
	// fmt.Printf("loaded database from %s (%d rows x %d bytes)\n", dbPath, numRows, rowSize)

	// * connect to rust server
	os.Remove(socketToRustServerPath) // clean up stale socket if any

	ln, err := net.Listen("unix", socketToRustServerPath)
	if err != nil {
		log.Fatalf("failed to listen on %s: %v", socketToRustServerPath, err)
	}
	defer ln.Close()
	// fmt.Printf("listening on %s\n", socketToRustServerPath)

	conn, err := ln.Accept()
	if err != nil {
		log.Printf("accept failed: %v", err)

	}
	defer conn.Close()
	// fmt.Println("[singlepass-server] connection made ! starting singlepass protocol...")

	// listen for as many messages from that rust-server until it closes conection
	for {

		// read message type as a length-prefixed single byte
		msgTypeData, err := readBytes(conn)
		if err != nil {
			// fmt.Printf("connection closed: %v\n", err)
			return
		}
		if len(msgTypeData) == 0 {
			log.Fatalf("empty message type")
		}
		msgType := msgTypeData[0]

		switch msgType {

		case MsgHintReq:
			var req pir.SinglePassHintReq
			reqData, err := readBytes(conn)
			if err != nil {
				log.Fatalf("failed to read HintReq: %v", err)
			}
			if err := decodeGob(reqData, &req); err != nil {
				log.Fatalf("failed to decode HintReq: %v", err)
			}
			// fmt.Printf("processing HintReq\n")

			var resp pir.HintResp
			if err := db.Hint(&req, &resp); err != nil {
				log.Fatalf("hint generation failed: %v", err)
			}
			respData, err := encodeGob(resp)
			if err != nil {
				log.Fatalf("failed to serialize HintResp: %v", err)
			}
			if err := writeBytes(conn, respData); err != nil {
				log.Fatalf("failed to send HintResp: %v", err)
			}

		case MsgDBQuery:
			var req pir.SinglePassQueryReq
			reqData, err := readBytes(conn)
			if err != nil {
				log.Fatalf("failed to read QueryReq: %v", err)
			}
			if err := decodeGob(reqData, &req); err != nil {
				log.Fatalf("failed to decode QueryReq: %v", err)
			}
			// fmt.Printf("processing AnswerReq\n")

			var respIface interface{} = &pir.SinglePassQueryResp{}
			if err := db.Answer(&req, &respIface); err != nil {
				log.Fatalf("answer failed: %v", err)
			}
			resp := respIface.(*pir.SinglePassQueryResp)
			respData, err := encodeGob(resp)
			if err != nil {
				log.Fatalf("failed to serialize answer: %v", err)
			}
			if err := writeBytes(conn, respData); err != nil {
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

func encodeGob(value interface{}) ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(value); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func decodeGob(data []byte, value interface{}) error {
	buf := bytes.NewBuffer(data)
	dec := gob.NewDecoder(buf)
	return dec.Decode(value)
}

func writeBytes(conn net.Conn, data []byte) error {
	length := uint32(len(data))
	if err := binary.Write(conn, binary.LittleEndian, length); err != nil {
		return err
	}
	_, err := conn.Write(data)
	return err
}

func readBytes(conn net.Conn) ([]byte, error) {
	var length uint32
	if err := binary.Read(conn, binary.LittleEndian, &length); err != nil {
		return nil, err
	}

	data := make([]byte, length)
	if _, err := io.ReadFull(conn, data); err != nil {
		return nil, err
	}
	return data, nil
}
