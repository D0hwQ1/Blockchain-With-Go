package pos

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
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
	Validator string // staking 한 토큰의 수량
}

var Blockchain []Block // 체인 선언
var tempBlocks []Block // Blockchain에 추가 될 블록을 경쟁하여 정해지기 전까지 담아두는 임시 변수

var candidateBlocks = make(chan Block) // 각 노드(클라이언트)가 제안하는 새 블록이 담기는 곳
var announcements = make(chan string)  // 최신 블록을 접속한 모든 클라이언트에게 브로드 캐스트 전송
var validators = make(map[string]int)  // 노드(클라이언트)의 맵과 staking한 수량

var mutex = &sync.Mutex{}

func Start() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal(err)
	}

	t := time.Now()
	genesisBlock := Block{}
	genesisBlock = Block{0, t.String(), 0, calculateHash(genesisBlock), "", ""} // 첫 블록 생성
	Blockchain = append(Blockchain, genesisBlock)
	spew.Dump(genesisBlock)

	server, err := net.Listen("tcp", ":"+os.Getenv("PORT")) // tcp 통신 서버 오픈
	if err != nil {
		log.Fatal(err)
	}
	defer server.Close()

	go func() { // 노드가 블록을 생성할 경우, tempBlocks에 담음
		for candidate := range candidateBlocks {
			mutex.Lock()
			tempBlocks = append(tempBlocks, candidate)
			mutex.Unlock()
		}
	}()

	go func() {
		for {
			pickWinner()
		}
	}()

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

	go func() {
		for {
			msg := <-announcements
			io.WriteString(conn, msg)
		}
	}()

	var addr string

	go func() {
		addr = randAddress()
		io.WriteString(conn, "현재 블록\n"+spew.Sdump(Blockchain))
		io.WriteString(conn, "\nYou are Address: "+addr)
		io.WriteString(conn, "\nEnter token balance: ")

		for {
			scanBalance := bufio.NewScanner(conn)
			scanBalance.Scan()

			balance, err := strconv.Atoi(scanBalance.Text())
			if err != nil {
				io.WriteString(conn, fmt.Sprintf("%v not a number: %s\nEnter token balance: ", scanBalance.Text(), err))
				continue
			}

			validators[addr] = balance
			fmt.Println(validators)

			io.WriteString(conn, "Enter a new BPM: ")

			scanBPM := bufio.NewScanner(conn)
			scanBPM.Scan()

			bpm, err := strconv.Atoi(scanBPM.Text())
			if err != nil {
				io.WriteString(conn, fmt.Sprintf("%v not a number: %s\nEnter a new BPM: ", scanBPM.Text(), err))
				delete(validators, addr)
				conn.Close()
			}

			mutex.Lock()
			oldLastIndex := Blockchain[len(Blockchain)-1]
			mutex.Unlock()

			newBlock, err := generateBlock(oldLastIndex, bpm, addr)
			if err != nil {
				log.Println(err)
				io.WriteString(conn, "\nEnter a new BPM: ")
				continue
			}
			if isBlockValid(newBlock, oldLastIndex) {
				candidateBlocks <- newBlock
			}

			io.WriteString(conn, "\nEnter token balance: ")
		}
	}()

	prev, err := json.Marshal(Blockchain)
	if err != nil {
		log.Fatal(err)
	}

	for {
		time.Sleep(10 * time.Second)
		mutex.Lock()
		output, err := json.Marshal(Blockchain)
		mutex.Unlock()
		if err != nil {
			log.Fatal(err)
		}

		if !bytes.Equal(prev, output) {
			io.WriteString(conn, spew.Sdump(Blockchain))
			prev = output
		}
	}
}

func pickWinner() { // PoS 알고리즘 적용
	time.Sleep(10 * time.Second)
	mutex.Lock()
	temp := tempBlocks
	mutex.Unlock()

	lotteryPool := []string{}

	if len(temp) > 0 {

	OUTER:
		// 블록은 생성한 노드 참가자 중 누가 많은 지분을 staking 했는지 체크
		for _, block := range temp {
			for _, node := range lotteryPool {
				if block.Validator == node {
					continue OUTER
				}
			}

			mutex.Lock()
			setValidators := validators
			mutex.Unlock()

			k, ok := setValidators[block.Validator] // balance 조회
			if ok {
				for i := 0; i < k; i++ {
					lotteryPool = append(lotteryPool, block.Validator)
				}
			}
		}

		s := rand.NewSource(time.Now().Unix())
		r := rand.New(s)
		mutex.Lock()
		lotteryWinner := lotteryPool[r.Intn(len(lotteryPool))]
		mutex.Unlock()

		for _, block := range temp {
			if block.Validator == lotteryWinner {
				Blockchain = append(Blockchain, block)

				for _ = range validators {
					announcements <- "\nwinning validator: " + lotteryWinner + "\n"
				}
				break
			}
		}
	}

	mutex.Lock()
	tempBlocks = []Block{}
	mutex.Unlock()
}

func calculateHash(block Block) string { // 해쉬 생성
	record := strconv.Itoa(block.Index) + block.Timestamp + strconv.Itoa(block.BPM) + block.PrevHash
	h := sha256.New()
	h.Write([]byte(record))
	hashed := h.Sum(nil)
	return hex.EncodeToString(hashed)
}

func isBlockValid(newBlock, oldBlock Block) bool { // 추가할 블록 변조 체크
	if oldBlock.Index+1 != newBlock.Index {
		return false
	}
	if oldBlock.Hash != newBlock.PrevHash {
		return false
	}
	if calculateHash(newBlock) != newBlock.Hash {
		return false
	}

	return true
}
func isBlockchainValid() error { // 전체 체인 변조 체크
	currBlockIdx := len(Blockchain) - 1
	prevBlockIdx := len(Blockchain) - 2

	for prevBlockIdx >= 0 {
		currBlock := Blockchain[currBlockIdx]
		prevBlock := Blockchain[prevBlockIdx]

		if currBlock.PrevHash != prevBlock.Hash {
			return errors.New("blockchain has inconsistent hashes")
		}

		if currBlock.Timestamp <= prevBlock.Timestamp {
			return errors.New("blockchain has inconsistent timestamps")
		}

		if calculateHash(currBlock) != currBlock.Hash {
			return errors.New("blockchain has inconsistent hash generation")
		}

		currBlockIdx--
		prevBlockIdx--
	}
	return nil
}

func generateBlock(oldBlock Block, BPM int, addr string) (Block, error) { // BPM을 입력받아 블록 생성
	if err := isBlockchainValid(); err != nil {
		validators[addr] -= 5
		return oldBlock, err
	}

	var newBlock Block

	t := time.Now()

	newBlock.Index = oldBlock.Index + 1
	newBlock.Timestamp = t.String()
	newBlock.BPM = BPM
	newBlock.PrevHash = oldBlock.Hash
	newBlock.Hash = calculateHash(newBlock)
	newBlock.Validator = addr

	return newBlock, nil
}

func randAddress() string { // 지갑 생성
	b := make([]byte, 20)
	rand.Seed(time.Now().UnixNano())
	rand.Read(b)
	return fmt.Sprintf("0x%x", b)
}
