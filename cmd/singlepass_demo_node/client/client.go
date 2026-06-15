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

func main() {
	gob.Register(&pir.SinglePassHintReq{})
	gob.Register(&pir.SinglePassHintResp{})
	gob.Register(&pir.SinglePassQueryReq{})
	gob.Register(&pir.SinglePassQueryResp{})

	if len(os.Args) != 3 {
		log.Fatalf("usage: singlepass-client <setSize> <socket-path>")
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
	fmt.Printf("[singlepass-client] listening on %s...\n", clientSocketPath)

	conn, err := ln.Accept()
	if err != nil {
		log.Fatalf("failed to accept rust connection: %v", err)
	}
	defer conn.Close()
	fmt.Println("[singlepass-client] connection made ! starting singlepass protocol...")

	// ------ OFFLINE PHASE OF SINGLEPASS ------

	randSource := rand.New(rand.NewSource(42))

	// generate hint request
	fmt.Println("[singlepass-client] Generating hint request...")
	hintReq := pir.NewHintReq(randSource, pir.SinglePass, setSize)
	data, err := encodeGob(hintReq)
	if err != nil {
		log.Fatalf("failed to serialize hint request: %v", err)
	}
	fmt.Printf("[singlepass-client] sending hint request ...\n")
	if err := writeBytes(conn, data); err != nil {
		log.Fatalf("failed to send hint request: %v", err)
	}

	// receive hint response and generate client
	fmt.Println("[singlepass-client] receiving hint response...")
	respData, err := readBytes(conn)
	if err != nil {
		log.Fatalf("failed to read hint response: %v", err)
	}

	var hintResp pir.SinglePassHintResp
	if err := decodeGob(respData, &hintResp); err != nil {
		log.Fatalf("failed to decode hint response: %v", err)
	}
	client := hintResp.InitClient(randSource)
	fmt.Println("[singlepass-client] client created !")

	// ------ ONLINE PHASE OF SINGLEPASS ------
	// listen to as many db quests from the rust client
	for {

		targetData, err := readBytes(conn)
		if err != nil {
			log.Fatalf("failed to read query target: %v", err)
		}
		if len(targetData) != 4 {
			log.Fatalf("unexpected target length: %d", len(targetData))
		}
		target := binary.LittleEndian.Uint32(targetData)
		queries, reconstruct := client.Query(int(target))

		// write the two requests consecutatively
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

		// wait for the two responses
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

		// reconstruct db row
		row, err := reconstruct([]interface{}{&leftResp, &rightResp})
		if err != nil {
			log.Fatalf("failed to reconstruct row: %v", err)
		}
		if err := writeBytes(conn, row); err != nil {
			log.Fatalf("failed to send reconstructed row: %v", err)
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
