package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/joho/godotenv"
)

type Block struct {
	Index     int    // 데이터 레코드 위치
	Timestamp string // 데이터 기록되는 시간
	BPM       int    // business process management
	Hash      string // 해당 블록 sha256 해쉬값
	PrevHash  string // 이전 블록의 sha256 해쉬값
}

var bcServer chan []Block
var Blockchain []Block // 체인 선언

var mutex = &sync.Mutex{}

func main() {
	err := godotenv.Load("../.env")
	if err != nil {
		log.Fatal(err)
	}

	bcServer = make(chan []Block)

	t := time.Now()
	genesisBlock := Block{}
	genesisBlock = Block{0, t.String(), 0, calculateHash(genesisBlock), ""} // 첫 블록 생성
	Blockchain = append(Blockchain, genesisBlock)
	spew.Dump(genesisBlock)

	server, err := net.Listen("tcp", ":"+os.Getenv("PORT")) // tcp 통신 서버 오픈
	if err != nil {
		log.Fatal(err)
	}
	defer server.Close()

	for {
		conn, err := server.Accept()
		if err != nil {
			log.Fatal(err)
		}
		go handleConn(conn)
	}
}

func handleConn(conn net.Conn) { // tcp에 통신한 클라이언트의 블록 생성
	defer conn.Close()

	io.WriteString(conn, "Enter a new BPM:")
	scanner := bufio.NewScanner(conn)

	go func() {
		for scanner.Scan() {
			bpm, err := strconv.Atoi(scanner.Text())
			if err != nil {
				io.WriteString(conn, fmt.Sprintf("%v not a number: %s\nEnter a new BPM:", scanner.Text(), err))
				continue
			}
			newBlock, err := generateBlock(Blockchain[len(Blockchain)-1], bpm)
			if err != nil {
				log.Println(err)
				continue
			}
			if isBlockValid(newBlock, Blockchain[len(Blockchain)-1]) {
				newBlockchain := append(Blockchain, newBlock)
				replaceChain(newBlockchain)
			}

			spew.Dump(Blockchain)
			bcServer <- Blockchain
			spew.Dump(bcServer)

			io.WriteString(conn, "\nEnter a new BPM:")
		}
	}()

	for _ = range bcServer {
		spew.Dump(Blockchain)
	}
}

func calculateHash(block Block) string { // 해쉬 생성
	record := strconv.Itoa(block.Index) + block.Timestamp + strconv.Itoa(block.BPM) + block.PrevHash
	h := sha256.New()
	h.Write([]byte(record))
	hashed := h.Sum(nil)
	return hex.EncodeToString(hashed)
}

func isBlockValid(newBlock, oldBlock Block) bool { // 체인 및 블록 변조 체크
	if oldBlock.Index+1 != newBlock.Index &&
		oldBlock.Hash != newBlock.PrevHash &&
		calculateHash(newBlock) != newBlock.Hash {
		return false
	} else {
		return true
	}
}

func generateBlock(oldBlock Block, BPM int) (Block, error) { // BPM을 입력받아 블록 생성
	var newBlock Block

	t := time.Now()

	newBlock.Index = oldBlock.Index + 1
	newBlock.Timestamp = t.String()
	newBlock.BPM = BPM
	newBlock.PrevHash = oldBlock.Hash
	newBlock.Hash = calculateHash(newBlock)

	return newBlock, nil
}

// fork(분기) 되었을 때, 어느 블록들이 신뢰성을 띄는 지 비교 -> 51% 공격에 무력화 될 수 있음
func replaceChain(newBlocks []Block) {
	fmt.Println(len(newBlocks), len(Blockchain))
	if len(newBlocks) > len(Blockchain) {
		Blockchain = newBlocks
	}
}
