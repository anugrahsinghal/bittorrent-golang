package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"fmt"
	"io"
	"net"
)

const INTERESTED = 2
const BITFIELD = 5
const UNCHOKE = 1
const REQUEST = 6
const PIECE = 7
const BlockSize = 16 * 1024

func downloadPiece(pieceId, pieceLength int, conn net.Conn, pieces []string) []byte {
	//fmt.Printf("PieceHash for id: %d --> %x\n", pieceId, pieces[pieceId])
	// say 256 KB
	// for each block
	sendRequestForPiece(pieceId, pieceLength, conn)

	fmt.Printf("For Piece : [%d] of possible Size :[%d] Sent Requests for Blocks of size %d\n", pieceId, pieceLength, BlockSize)

	combinedBlockToPiece := downloadRequestedPiece(pieceId, pieceLength, conn)

	ok := verifyPiece(combinedBlockToPiece, pieces, pieceId)

	if !ok {
		panic("unequal pieces")
	}

	return combinedBlockToPiece
}

func sendRequestForPiece(pieceId, pieceLength int, conn net.Conn) {
	count := calculateBlockCount(pieceLength)
	requests := make([]RequestPayload, count)

	for i := range requests {
		begin := uint32(i * BlockSize)
		blockSize := uint32(BlockSize)
		if uint32(pieceLength)-begin < BlockSize {
			blockSize = uint32(pieceLength) - begin
		}
		requests[i] = RequestPayload{
			Index:     uint32(pieceId),
			Begin:     begin,
			BlockSize: blockSize,
		}
	}

	for _, request := range requests {
		payload := make([]byte, 12)
		binary.BigEndian.PutUint32(payload[0:4], request.Index)    // index
		binary.BigEndian.PutUint32(payload[4:8], request.Begin)    // begin
		binary.BigEndian.PutUint32(payload[8:], request.BlockSize) // block size
		_, err := conn.Write(createPeerMessage(REQUEST, payload))
		handleErr(err)
	}
}

func calculateBlockCount(pieceLength int) int {
	var carry int
	if pieceLength%BlockSize > 0 {
		carry = 1
	}
	count := pieceLength/BlockSize + carry
	return count
}

func downloadRequestedPiece(pieceId, pieceLength int, conn net.Conn) []byte {
	blockCount := calculateBlockCount(pieceLength)
	combinedBlockToPiece := make([]byte, pieceLength)
	for i := 0; i < blockCount; i++ {
		data := waitFor(conn, PIECE)

		index := binary.BigEndian.Uint32(data[0:4])
		if index != uint32(pieceId) {
			panic(fmt.Sprintf("something went wrong [expected: %d -- actual: %d]", pieceId, index))
		}
		begin := binary.BigEndian.Uint32(data[4:8])
		block := data[8:]
		copy(combinedBlockToPiece[begin:], block)
	}
	return combinedBlockToPiece
}

func verifyPiece(combinedBlockToPiece []byte, pieces []string, pieceId int) bool {
	sum := sha1.Sum(combinedBlockToPiece)
	return string(sum[:]) == pieces[pieceId]
}

func preDownload(metaInfo MetaInfo, conn net.Conn) {
	handshake(metaInfo, conn)

	waitFor(conn, BITFIELD)

	_, err := conn.Write(createPeerMessage(INTERESTED, []byte{}))
	handleErr(err)
	//fmt.Printf("Sent INTERESTED message\n")

	waitFor(conn, UNCHOKE)
}

func waitFor(connection net.Conn, expectedMessageId uint8) []byte {
	//fmt.Printf("Waiting for %d\n", expectedMessageId)
	for {
		messageLengthPrefix := make([]byte, 4)
		_, err := connection.Read(messageLengthPrefix)
		handleErr(err)
		messageLength := binary.BigEndian.Uint32(messageLengthPrefix)
		//fmt.Printf("messageLength %v\n", messageLength)

		receivedMessageId := make([]byte, 1)
		_, err = connection.Read(receivedMessageId)
		handleErr(err)

		var messageId uint8
		binary.Read(bytes.NewReader(receivedMessageId), binary.BigEndian, &messageId)
		//fmt.Printf("MessageId: %d\n", messageId)

		payload := make([]byte, messageLength-1) // remove message id offset
		_, err = io.ReadFull(connection, payload)
		handleErr(err)
		//fmt.Printf("Payload: %d, size = %d\n", len(payload), size)

		if messageId == expectedMessageId {
			//fmt.Printf("Return for MessageId: %d\n", messageId)
			return payload
		}
	}
}

func createPeerMessage(messageId uint8, payload []byte) []byte {
	// Peer messages consist of a message length prefix (4 bytes), message id (1 byte) and a payload (variable size).
	messageData := make([]byte, 4+1+len(payload))
	binary.BigEndian.PutUint32(messageData[0:4], uint32(1+len(payload)))
	messageData[4] = messageId
	copy(messageData[5:], payload)

	return messageData
}

type RequestPayload struct {
	Index     uint32
	Begin     uint32
	BlockSize uint32
}
