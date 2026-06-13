package main

import (
	"bytes"
	"checklist/pir"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"os"
	"strconv"
)

const (
	CmdGenerateHintReq uint8 = 1
	CmdInitHintResp    uint8 = 2
	CmdGenerateQuery   uint8 = 3
	CmdReconstructRow  uint8 = 4
)

func main() {
	gob.Register(&pir.SinglePassHintReq{})
	gob.Register(&pir.SinglePassHintResp{})
	gob.Register(&pir.SinglePassQueryReq{})
	gob.Register(&pir.SinglePassQueryResp{})

	if len(os.Args) != 3 {
		log.Fatalf("usage: client <setSize> <socket-path>")
	}

	setSize, err := strconv.Atoi(os.Args[1])
	if err != nil || setSize <= 0 {
		log.Fatalf("invalid setsize value %q", os.Args[1])
	}
	clientSocketPath := os.Args[2]

	os.Remove(clientSocketPath)
	ln, err := net.Listen("unix", clientSocketPath)
	if err != nil {
		log.Fatalf("failed to listen for rust client: %v", err)
	}
	defer ln.Close()
	fmt.Printf("client listening on %s\n", clientSocketPath)

	conn, err := ln.Accept()
	if err != nil {
		log.Fatalf("failed to accept rust connection: %v", err)
	}
	defer conn.Close()
	fmt.Println("rust client connected")

	randSource := rand.New(rand.NewSource(42))
	var client pir.Client
	var reconstruct pir.ReconstructFunc

	// listen to as many messages from the rust client
	for {

		var cmd uint8
		if err := binary.Read(conn, binary.LittleEndian, &cmd); err != nil {
			fmt.Printf("rust connection closed: %v\n", err)
			return
		}

		switch cmd {
		case CmdGenerateHintReq:
			hintReq := pir.NewHintReq(randSource, pir.SinglePass, setSize)
			data, err := encodeGob(hintReq)
			if err != nil {
				log.Fatalf("failed to serialize hint request: %v", err)
			}
			if err := writeBytes(conn, data); err != nil {
				log.Fatalf("failed to send hint request: %v", err)
			}

		case CmdInitHintResp:
			respData, err := readBytes(conn)
			if err != nil {
				log.Fatalf("failed to read hint response: %v", err)
			}
			var hintResp pir.SinglePassHintResp
			if err := decodeGob(respData, &hintResp); err != nil {
				log.Fatalf("failed to decode hint response: %v", err)
			}
			client = hintResp.InitClient(randSource)

		case CmdGenerateQuery:
			if client == nil {
				log.Fatalf("client is not initialized; must receive hint response first")
			}

			var target int32
			if err := binary.Read(conn, binary.LittleEndian, &target); err != nil {
				log.Fatalf("failed to read query target: %v", err)
			}
			queries, fn := client.Query(int(target))
			reconstruct = fn

			leftBytes, err := encodeGob(queries[pir.Left])
			if err != nil {
				log.Fatalf("failed to serialize left query: %v", err)
			}
			if err := writeBytes(conn, leftBytes); err != nil {
				log.Fatalf("failed to send left query: %v", err)
			}

			rightBytes, err := encodeGob(queries[pir.Right])
			if err != nil {
				log.Fatalf("failed to serialize right query: %v", err)
			}
			if err := writeBytes(conn, rightBytes); err != nil {
				log.Fatalf("failed to send right query: %v", err)
			}

		case CmdReconstructRow:
			if reconstruct == nil {
				log.Fatalf("reconstruction closure is not available; generate query first")
			}

			leftRespData, err := readBytes(conn)
			if err != nil {
				log.Fatalf("failed to read left response: %v", err)
			}
			var leftResp pir.SinglePassQueryResp
			if err := decodeGob(leftRespData, &leftResp); err != nil {
				log.Fatalf("failed to decode left response: %v", err)
			}

			rightRespData, err := readBytes(conn)
			if err != nil {
				log.Fatalf("failed to read right response: %v", err)
			}
			var rightResp pir.SinglePassQueryResp
			if err := decodeGob(rightRespData, &rightResp); err != nil {
				log.Fatalf("failed to decode right response: %v", err)
			}

			row, err := reconstruct([]interface{}{&leftResp, &rightResp})
			if err != nil {
				log.Fatalf("failed to reconstruct row: %v", err)
			}
			if err := writeBytes(conn, row); err != nil {
				log.Fatalf("failed to send reconstructed row: %v", err)
			}
			reconstruct = nil

		default:
			log.Fatalf("unknown command from rust client: %d", cmd)
		}
	}
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
	length := int32(0)
	if data != nil {
		length = int32(len(data))
	}
	if err := binary.Write(conn, binary.LittleEndian, length); err != nil {
		return err
	}
	if length > 0 {
		_, err := conn.Write(data)
		return err
	}
	return nil
}

func readBytes(conn net.Conn) ([]byte, error) {
	var length int32
	if err := binary.Read(conn, binary.LittleEndian, &length); err != nil {
		return nil, err
	}
	if length < 0 {
		return nil, fmt.Errorf("invalid length %d", length)
	}
	data := make([]byte, length)
	if length > 0 {
		if _, err := io.ReadFull(conn, data); err != nil {
			return nil, err
		}
	}
	return data, nil
}
