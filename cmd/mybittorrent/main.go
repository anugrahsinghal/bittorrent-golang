package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"github.com/jackpal/bencode-go"
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
		// use std lib to read file's contents as a string
		file, err := os.ReadFile(fileNameOrPath)
		if err != nil {
			fmt.Println(err)
			return
		}

		var metaInfo MetaInfo
		if err := bencode.Unmarshal(bytes.NewReader(file), &metaInfo); err != nil {
			fmt.Println("Error unmarshalling JSON:", err)
			return
		}

		fmt.Printf("Tracker URL: %v\n", metaInfo.Announce)
		fmt.Printf("Length: %v\n", metaInfo.Info.Length)

		var buffer_ bytes.Buffer
		if err := bencode.Marshal(&buffer_, metaInfo.Info); err != nil {
			fmt.Println("Error marshalling BEncode:", err)
			return
		}
		sum := sha1.Sum(buffer_.Bytes())
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

	} else {
		fmt.Println("Unknown command: " + command)
		os.Exit(1)
	}
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
