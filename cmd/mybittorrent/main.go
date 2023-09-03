package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/jackpal/bencode-go"
	"os"
	"strconv"
	"unicode"
	// bencode "github.com/jackpal/bencode-go" // Available if you need it!
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
		decoded, err := bencode.Decode(bytes.NewReader(file))
		if err != nil {
			fmt.Println(err)
			return
		}

		jsonOutput, _ := json.Marshal(decoded)

		var metaInfo MetaInfo
		if err := json.Unmarshal(jsonOutput, &metaInfo); err != nil {
			fmt.Println("Error unmarshalling JSON:", err)
			return
		}

		fmt.Printf("Tracker URL: %v\n", metaInfo.Announce)
		fmt.Printf("Length: %v\n", metaInfo.Info.Length)

	} else {
		fmt.Println("Unknown command: " + command)
		os.Exit(1)
	}
}

type MetaInfo struct {
	Announce string `json:"announce"`
	Info     Info   `json:"info"`
}
type Info struct {
	Length    int64  `json:"length"`
	Name      string `json:"name"`
	PiecesLen int64  `json:"piece length"`
	Pieces    string `json:"pieces"`
}
