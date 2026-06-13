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
	"strconv"
)

// Message type tags — must match server
const (
	MsgHintReq   uint8 = 1
	MsgAnswerReq uint8 = 2
)

func sendMsg(conn net.Conn, msgType uint8, enc *gob.Encoder, payload interface{}) error {
	if err := binary.Write(conn, binary.LittleEndian, msgType); err != nil {
		return fmt.Errorf("failed to write msg type: %w", err)
	}
	if err := enc.Encode(payload); err != nil {
		return fmt.Errorf("failed to encode payload: %w", err)
	}
	return nil
}

func main() {
	gob.Register(pir.SinglePassHintReq{})
	gob.Register(pir.SinglePassHintResp{})
	gob.Register(pir.SinglePassQueryReq{})
	gob.Register(pir.SinglePassQueryResp{})

	// * parse command line argmuents
	if len(os.Args) != 3 {
		log.Fatalf("usage: client <setSize> <socket-path>")
	}

	setSize, err := strconv.Atoi(os.Args[1])
	if err != nil || setSize <= 0 {
		log.Fatalf("invalid setsize value %q", os.Args[1])
	}
	socketToRustClientPath := os.Args[2]

	// * Connect to both servers
	leftConn, err := net.Dial("unix", "/tmp/SinglePass-server-left.sock")
	if err != nil {
		log.Fatalf("failed to connect to left server: %v", err)
	}
	defer leftConn.Close()

	rightConn, err := net.Dial("unix", "/tmp/SinglePass-server-right.sock")
	if err != nil {
		log.Fatalf("failed to connect to right server: %v", err)
	}
	defer rightConn.Close()

	leftEnc := gob.NewEncoder(leftConn)
	leftDec := gob.NewDecoder(leftConn)
	rightEnc := gob.NewEncoder(rightConn)
	rightDec := gob.NewDecoder(rightConn)
	// ===== OFFLINE PHASE SINGLEPASS =====

	random := rand.New(rand.NewSource(42))
	hintReq := pir.NewHintReq(random, pir.SinglePass, setSize)

	fmt.Println("sending hint request to left server...")
	if err := sendMsg(leftConn, MsgHintReq, leftEnc, hintReq); err != nil {
		log.Fatalf("failed to send hint request: %v", err)
	}

	var hintResp pir.SinglePassHintResp
	if err := leftDec.Decode(&hintResp); err != nil {
		log.Fatalf("failed to receive hint response: %v", err)
	}
	fmt.Println("received hint response, initializing client state...")

	client := hintResp.InitClient(random)
	fmt.Println("offline phase complete, listening for rust queries...")

	// ===== ONLINE PHASE SINGLE PASS =====
	socketPath := "/tmp/SinglePass-client.sock"
	os.Remove(socketPath) // clean up stale socket if any

	rustLn, err := net.Listen("unix", socketPath)
	if err != nil {
		log.Fatalf("failed to listen for rust: %v", err)
	}
	rustConn, err := rustLn.Accept()
	if err != nil {
		log.Fatalf("failed to accept rust connection: %v", err)
	}
	fmt.Println("rust connected")

	// receive as many messsages from that rust client
	for {

		var target int32
		if err := binary.Read(rustConn, binary.LittleEndian, &target); err != nil {
			break
		}
		fmt.Printf("received query for index %d\n", target)

		// Generate PIR queries for both servers
		queries, reconstruct := client.Query(int(target))

		// Send query to left server
		if err := sendMsg(leftConn, MsgAnswerReq, leftEnc, queries[pir.Left]); err != nil {
			log.Fatalf("failed to send left query: %v", err)
		}
		// Send query to right server
		if err := sendMsg(rightConn, MsgAnswerReq, rightEnc, queries[pir.Right]); err != nil {
			log.Fatalf("failed to send right query: %v", err)
		}

		// Receive answers from both servers
		var leftResp pir.SinglePassQueryResp
		var rightResp pir.SinglePassQueryResp
		if err := leftDec.Decode(&leftResp); err != nil {
			log.Fatalf("failed to receive left answer: %v", err)
		}
		if err := rightDec.Decode(&rightResp); err != nil {
			log.Fatalf("failed to receive right answer: %v", err)
		}
		responses := []interface{}{&leftResp, &rightResp}

		// Reconstruct the row
		row, err := reconstruct(responses)
		if err != nil {
			log.Fatalf("reconstruction failed: %v", err)
		}

		// Send row back to Rust
		fmt.Printf("sending %d bytes back to rust\n", len(row))
		binary.Write(rustConn, binary.LittleEndian, int32(len(row)))
		rustConn.Write(row)
	}
}
