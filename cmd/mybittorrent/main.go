package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"github.com/jackpal/bencode-go"
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
		fmt.Printf("Piece Hashes: \n")
		for i := 0; i < len(metaInfo.Info.Pieces)/20; i++ {
			piece := metaInfo.Info.Pieces[i*20 : (i*20)+20]
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
		handshake(metaInfo, peer)

	} else {
		fmt.Println("Unknown command: " + command)
		os.Exit(1)
	}
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

func printPeers(metaInfo MetaInfo) {
	response, _ := makeGetRequest(metaInfo)

	var trackerResponse TrackerResponse
	bencode.Unmarshal(bytes.NewReader(response), &trackerResponse)
	//fmt.Printf("trackerResponse %v\n", trackerResponse)

	numPeers := len(trackerResponse.Peers) / 6
	//fmt.Printf("numPeers %v\n", numPeers)
	for i := 0; i < numPeers; i++ {
		start := i * 6
		end := start + 6
		peer := trackerResponse.Peers[start:end]
		ip := net.IP(peer[0:4])
		port := binary.BigEndian.Uint16([]byte(peer[4:6]))
		fmt.Printf("%s:%d\n", ip, port)
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

func handshake(metaInfo MetaInfo, peer string) {

	// Connect to a TCP server
	conn, err := net.Dial("tcp", peer)
	if err != nil {
		fmt.Println(err)
		panic(err)
	}
	defer conn.Close()

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
	_, err = conn.Write(myBytes)
	if err != nil {
		fmt.Println(err)
		panic(err)
	}

	fmt.Println("Message Sent, waiting for message myself")

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
