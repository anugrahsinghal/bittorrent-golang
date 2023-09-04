package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"github.com/jackpal/bencode-go"
	"log"
	"net"
	"os"
	"strconv"
)

func main() {
	// You can use print statements as follows for debugging, they'll be visible when running tests.
	//fmt.Println("Logs from your program will appear here!")

	command := os.Args[1]

	if command == "decode" {
		bencodedValue := os.Args[2]
		decoded, err := bencode.Decode(bytes.NewReader([]byte(bencodedValue)))
		handleErr(err)

		jsonOutput, _ := json.Marshal(decoded)
		fmt.Println(string(jsonOutput))
	} else if command == "info" {
		// read the file
		fileNameOrPath := os.Args[2]
		metaInfo, err := getMetaInfo(fileNameOrPath)
		handleErr(err)

		fmt.Printf("Tracker URL: %v\n", metaInfo.Announce)
		fmt.Printf("Length: %v\n", metaInfo.Info.Length)

		sum := createInfoHash(metaInfo)
		// %x for hex formatting
		fmt.Printf("Info Hash: %x\n", sum)

		//Piece Length: 262144
		fmt.Printf("Piece Length: %v\n", metaInfo.Info.PieceLength)
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
		handleErr(err)

		printPeers(metaInfo)

	} else if command == "handshake" {
		fileNameOrPath := os.Args[2]
		metaInfo, err := getMetaInfo(fileNameOrPath)
		handleErr(err)

		peer := os.Args[3]
		connection := createConnection(peer)
		handshake(metaInfo, connection)
		connection.Close()

	} else if command == "download_piece" {
		fileNameOrPath := os.Args[4]
		pieceId, err := strconv.Atoi(os.Args[5])
		handleErr(err)
		metaInfo, err := getMetaInfo(fileNameOrPath)
		handleErr(err)

		peers := getPeers(metaInfo)
		connections := map[string]net.Conn{}
		defer closeAllConnections(connections)
		//for _, peerObj := range peers {
		// since for this problem all peer will have the full file
		peerObj := peers[0]
		peer := fmt.Sprintf("%s:%d", peerObj.IP, peerObj.Port)
		connections[peer] = createConnection(peer)

		preDownload(metaInfo, connections[peer])

		pieces := getPieces(metaInfo)

		piece := downloadPiece(pieceId, int(metaInfo.Info.PieceLength), connections[peer], pieces)
		err = os.WriteFile(os.Args[3], piece, os.ModePerm)
		handleErr(err)
		//}
	} else if command == "download" {
		outPutFileName := os.Args[3]
		fileNameOrPath := os.Args[4]
		metaInfo, err := getMetaInfo(fileNameOrPath)
		handleErr(err)

		peers := getPeers(metaInfo)
		connections := map[string]net.Conn{}
		defer closeAllConnections(connections)

		peerObj := peers[0]
		peer := fmt.Sprintf("%s:%d", peerObj.IP, peerObj.Port)

		connections[peer] = createConnection(peer)

		preDownload(metaInfo, connections[peer])

		pieces := getPieces(metaInfo)
		fmt.Printf("--------Total Pieces To Download: %d, Total Size: %d--------\n", len(pieces), metaInfo.Info.Length)
		fullFile := make([]byte, metaInfo.Info.Length)
		curr := 0
		for pieceIndex, _ := range pieces {
			fmt.Printf("Start download for piece %d\n", pieceIndex)
			pieceLength := pieceLength(pieceIndex, pieces, metaInfo)
			piece := downloadPiece(pieceIndex, pieceLength, connections[peer], pieces)
			copy(fullFile[curr:], piece)
			curr += len(piece)
		}

		err = os.WriteFile(outPutFileName, fullFile, os.ModePerm)
		handleErr(err)
		return

	} else {
		fmt.Println("Unknown command: " + command)
		os.Exit(1)
	}
}

func pieceLength(pieceIndex int, pieces []string, metaInfo MetaInfo) int {
	if pieceIndex != len(pieces)-1 {
		return int(metaInfo.Info.PieceLength)
	} else { // last piece
		lastPieceSize := metaInfo.Info.Length - (metaInfo.Info.PieceLength * int64(pieceIndex))
		fmt.Printf("Last Piece Size [%d - (%d*%d) = %d]\n", metaInfo.Info.Length, metaInfo.Info.PieceLength, pieceIndex, lastPieceSize)
		return int(lastPieceSize)
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

func handleErr(err error) {
	if err != nil {
		err := fmt.Errorf("error reading from connection: %v", err)
		//err = fmt.Errorf("read failed: %w", err)
		log.Print(err)
		panic(err)
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

type MetaInfo struct {
	Announce string `json:"announce" bencode:"announce"`
	Info     Info   `json:"info" bencode:"info"`
}
type Info struct {
	Length      int64  `json:"length" bencode:"length"`
	Name        string `json:"name" bencode:"name"`
	PieceLength int64  `json:"piece length" bencode:"piece length"`
	Pieces      string `json:"pieces" bencode:"pieces"`
}
type TrackerResponse struct {
	Interval int64  `json:"interval" bencoded:"interval"`
	Peers    string `json:"peers" bencoded:"peers"`
}
type Peer struct {
	IP   net.IP
	Port int
}
