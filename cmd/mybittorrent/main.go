package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"github.com/jackpal/bencode-go"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"unicode"
)

// Example:
// - 5:hello -> hello
// - 10:hello12345 -> hello12345
// - i52e -> 52
func decodeBencode(bencodedString string) (interface{}, error) {
	if unicode.IsDigit(rune(bencodedString[0])) {
		var firstColonIndex int

		for i := 0; i < len(bencodedString); i++ {
			if bencodedString[i] == ':' {
				firstColonIndex = i
				break
			}
		}

		lengthStr := bencodedString[:firstColonIndex]

		length, err := strconv.Atoi(lengthStr)
		if err != nil {
			return "", err
		}

		return bencodedString[firstColonIndex+1 : firstColonIndex+1+length], nil
	} else if bencodedString[0] == 'i' {
		var indexOfEndMarker int

		for i := 0; i < len(bencodedString); i++ {
			if bencodedString[i] == 'e' {
				indexOfEndMarker = i
				break
			}
		}

		return strconv.Atoi(bencodedString[1:indexOfEndMarker])
	} else {
		return "", fmt.Errorf("Only strings are supported at the moment")
	}
}

const BITFIELD = 5
const INTERESTED = 2
const UNCHOKE = 1
const REQUEST = 6
const PIECE = 7
const BLOCK_SIZE = 16 * 1024

func main() {
	// You can use print statements as follows for debugging, they'll be visible when running tests.
	//fmt.Println("Logs from your program will appear here!")

	command := os.Args[1]

	if command == "decode" {
		bencodedValue := os.Args[2]
		decoded, err := bencode.Decode(bytes.NewReader([]byte(bencodedValue)))

		if err != nil {
			fmt.Println(err)
			return
		}

		jsonOutput, _ := json.Marshal(decoded)
		fmt.Println(string(jsonOutput))
	} else if command == "info" {
		// read the file
		fileNameOrPath := os.Args[2]
		metaInfo, err := getMetaInfo(fileNameOrPath)
		if err != nil {
			fmt.Println(err)
			return
		}

		fmt.Printf("Tracker URL: %v\n", metaInfo.Announce)
		fmt.Printf("Length: %v\n", metaInfo.Info.Length)

		sum := createInfoHash(metaInfo)
		// %x for hex formatting
		fmt.Printf("Info Hash: %x\n", sum)

		//Piece Length: 262144
		fmt.Printf("Piece Length: %v\n", metaInfo.Info.PiecesLen)
		//Piece Hashes:
		// split metaInfo.Info.Pieces for each 20 bytes
		// each 20 bytes is a SHA1 hash

		//fmt.Printf("numberOfPieces %v\n", numberOfPieces)
		pieces := getPieces(metaInfo)
		fmt.Printf("Piece Hashes: \n")
		for _, piece := range pieces {
			fmt.Printf("%x\n", piece)
		}

	} else if command == "peers" {
		// read the file
		fileNameOrPath := os.Args[2]
		metaInfo, err := getMetaInfo(fileNameOrPath)
		if err != nil {
			fmt.Println(err)
			return
		}

		printPeers(metaInfo)

	} else if command == "handshake" {
		fileNameOrPath := os.Args[2]
		metaInfo, err := getMetaInfo(fileNameOrPath)
		if err != nil {
			fmt.Println(err)
			return
		}

		peer := os.Args[3]
		connection := createConnection(peer)
		handshake(metaInfo, connection)
		connection.Close()

	} else if command == "download_piece" {
		fileNameOrPath := os.Args[4]
		pieceId, err := strconv.Atoi(os.Args[5])
		handleErr(err)
		metaInfo, err := getMetaInfo(fileNameOrPath)
		if err != nil {
			fmt.Println(err)
			return
		}

		peers := getPeers(metaInfo)
		connections := map[string]net.Conn{}
		defer closeAllConnections(connections)
		//for _, peerObj := range peers {
		// since for this problem all peer will have the full file
		peerObj := peers[0]
		peer := fmt.Sprintf("%s:%d", peerObj.IP, peerObj.Port)
		connections[peer] = createConnection(peer)

		handshake(metaInfo, connections[peer])

		waitFor(connections[peer], BITFIELD)

		_, err = connections[peer].Write(createPeerMessage(INTERESTED, []byte{}))
		handleErr(err)
		fmt.Printf("Sent INTERESTED message\n")

		waitFor(connections[peer], UNCHOKE)

		pieceHash := getPieces(metaInfo)[pieceId]
		fmt.Printf("PieceHash for id: %d --> %x\n", pieceId, pieceHash)

		// say 256 KB
		// for each block
		count := 0
		for byteOffset := 0; byteOffset < int(metaInfo.Info.PiecesLen); byteOffset = byteOffset + BLOCK_SIZE {
			payload := make([]byte, 12)
			binary.BigEndian.PutUint32(payload[0:4], uint32(pieceId))
			binary.BigEndian.PutUint32(payload[4:8], uint32(byteOffset))
			binary.BigEndian.PutUint32(payload[8:], BLOCK_SIZE)

			_, err := connections[peer].Write(createPeerMessage(REQUEST, payload))
			handleErr(err)
			count++
		}
		combinedBlockToPiece := make([]byte, metaInfo.Info.PiecesLen)
		for i := 0; i < count; i++ {
			data := waitFor(connections[peer], PIECE)

			index := binary.BigEndian.Uint32(data[0:4])
			if index != uint32(pieceId) {
				panic(fmt.Sprintf("something went wrong [expected: %d -- actual: %d]", pieceId, index))
			}
			begin := binary.BigEndian.Uint32(data[4:8])
			block := data[8:]
			copy(combinedBlockToPiece[begin:], block)
		}
		sum := sha1.Sum(combinedBlockToPiece)
		if string(sum[:]) == pieceHash {
			err := os.WriteFile(os.Args[3], combinedBlockToPiece, os.ModePerm)
			handleErr(err)
			return
		} else {
			panic("unequal pieces")
		}

		//}
	} else {
		fmt.Println("Unknown command: " + command)
		os.Exit(1)
	}
}

func getPieces(metaInfo MetaInfo) []string {
	pieces := make([]string, len(metaInfo.Info.Pieces)/20)
	for i := 0; i < len(metaInfo.Info.Pieces)/20; i++ {
		piece := metaInfo.Info.Pieces[i*20 : (i*20)+20]
		pieces[i] = piece
	}
	return pieces
}

func waitFor(connection net.Conn, expectedMessageId uint8) []byte {
	fmt.Printf("Waiting for %d\n", expectedMessageId)
	for {
		messageLengthPrefix := make([]byte, 4)
		_, err := connection.Read(messageLengthPrefix)
		handleErr(err)
		messageLength := binary.BigEndian.Uint32(messageLengthPrefix)
		fmt.Printf("messageLength %v\n", messageLength)

		receivedMessageId := make([]byte, 1)
		_, err = connection.Read(receivedMessageId)
		handleErr(err)

		var messageId uint8
		binary.Read(bytes.NewReader(receivedMessageId), binary.BigEndian, &messageId)
		fmt.Printf("MessageId: %d\n", messageId)

		payload := make([]byte, messageLength-1) // remove message id offset
		size, err := io.ReadFull(connection, payload)
		handleErr(err)
		fmt.Printf("Payload: %d, size = %d\n", len(payload), size)

		if messageId == expectedMessageId {
			fmt.Printf("Return for MessageId: %d\n", messageId)
			return payload
		}
	}
}

func handleErr(err error) {
	if err != nil {
		fmt.Println(err)
		panic(err)
	}
}

func closeAllConnections(connections map[string]net.Conn) {
	for _, conn := range connections {
		conn.Close()
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

func getMetaInfo(fileNameOrPath string) (MetaInfo, error) {
	// use std lib to read file's contents as a string
	file, err := os.ReadFile(fileNameOrPath)
	if err != nil {
		return MetaInfo{}, err
	}

	var metaInfo MetaInfo
	if err := bencode.Unmarshal(bytes.NewReader(file), &metaInfo); err != nil {
		fmt.Println("Error unmarshalling JSON:", err)
		return MetaInfo{}, err
	}

	return metaInfo, nil
}

func getPeers(metaInfo MetaInfo) []Peer {
	response, _ := makeGetRequest(metaInfo)

	var trackerResponse TrackerResponse
	bencode.Unmarshal(bytes.NewReader(response), &trackerResponse)
	//fmt.Printf("trackerResponse %v\n", trackerResponse)

	numPeers := len(trackerResponse.Peers) / 6
	peers := make([]Peer, numPeers)
	//fmt.Printf("numPeers %v\n", numPeers)
	for i := 0; i < numPeers; i++ {
		start := i * 6
		end := start + 6
		peer := trackerResponse.Peers[start:end]
		ip := net.IP(peer[0:4])
		port := binary.BigEndian.Uint16([]byte(peer[4:6]))
		peers[i] = Peer{IP: ip, Port: int(port)}
	}
	return peers
}

func printPeers(metaInfo MetaInfo) {
	peers := getPeers(metaInfo)
	for i := 0; i < len(peers); i++ {
		fmt.Printf("%s:%d\n", peers[i].IP, peers[i].Port)
	}
}

func createInfoHash(metaInfo MetaInfo) [20]byte {
	var buffer_ bytes.Buffer
	if err := bencode.Marshal(&buffer_, metaInfo.Info); err != nil {
		fmt.Println("Error marshalling BEncode:", err)
		return [20]byte{}
	}
	sum := sha1.Sum(buffer_.Bytes())
	return sum
}

func makeGetRequest(metaInfo MetaInfo) ([]byte, error) {
	baseUrl := metaInfo.Announce
	params := url.Values{}
	infoHash := createInfoHash(metaInfo)
	// took help from code examples for - string(infoHash[:])
	params.Add("info_hash", string(infoHash[:]))
	params.Add("peer_id", "00112233445566778899")
	params.Add("port", "6881")
	params.Add("uploaded", "0")
	params.Add("downloaded", "0")
	params.Add("left", strconv.Itoa(int(metaInfo.Info.Length)))
	params.Add("compact", "1")

	// Escape the params
	escapedParams := params.Encode()

	// Construct full URL
	URI := fmt.Sprintf("%s?%s", baseUrl, escapedParams)
	fmt.Printf("URI %v\n", URI)

	resp, err := http.DefaultClient.Get(URI)

	//fmt.Printf("StatusCode = %v\n", resp.Status)
	if err != nil {
		return []byte{}, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return []byte{}, err
	}

	return body, nil
}

func createConnection(peer string) net.Conn {
	// Connect to a TCP server
	conn, err := net.Dial("tcp", peer)
	if err != nil {
		fmt.Println(err)
		panic(err)
	}
	return conn
}

func handshake(metaInfo MetaInfo, conn net.Conn) {
	infoHash := createInfoHash(metaInfo)
	//messageHolder := make([]byte, 1+19+8+20+20)
	//messageHolder[0] = 19
	//copy(messageHolder[1:1+19], "BitTorrent protocol")
	//copy(messageHolder[20:20+8], make([]byte, 8))
	//copy(messageHolder[28:28+20], infoHash[:])
	//copy(messageHolder[48:48+20], "00112233445566778899")

	myStr :=
		"BitTorrent protocol" + // fixed header
			"00000000" + // reserved bytes
			string(infoHash[:]) +
			"00112233445566778899" // peerId

	// Convert int 19 to byte
	b := make([]byte, 1)
	b[0] = byte(19)

	// Concatenate byte with rest of string
	myBytes := append(b, []byte(myStr)...)

	// issue here is that 19 is encoded as 2 characters instead of 1
	//myStr := "19" + "BitTorrent protocol" + "00000000" + string(infoHash[:]) + "00112233445566778899"
	_, err := conn.Write(myBytes)
	if err != nil {
		fmt.Println(err)
		panic(err)
	}

	fmt.Println("Handshake Message Sent, waiting for handshake message myself")

	// Receive response
	buf := make([]byte, 1+19+8+20+20)
	_, err = conn.Read(buf)
	if err != nil {
		fmt.Println(err)
		panic(err)
	}

	fmt.Printf("Peer ID: %x\n", string(buf[48:]))
}

type MetaInfo struct {
	Announce string `json:"announce" bencode:"announce"`
	Info     Info   `json:"info" bencode:"info"`
}
type Info struct {
	Length    int64  `json:"length" bencode:"length"`
	Name      string `json:"name" bencode:"name"`
	PiecesLen int64  `json:"piece length" bencode:"piece length"`
	Pieces    string `json:"pieces" bencode:"pieces"`
}
type TrackerResponse struct {
	Interval int64  `json:"interval" bencoded:"interval"`
	Peers    string `json:"peers" bencoded:"peers"`
}
type Peer struct {
	IP   net.IP
	Port int
}
