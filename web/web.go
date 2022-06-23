package web

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
)

type Block struct {
	Index     int    // 데이터 레코드 위치
	Timestamp string // 데이터 기록되는 시간
	BPM       int    // business process management
	Hash      string // 해당 블록 sha256 해쉬값
	PrevHash  string // 이전 블록의 sha256 해쉬값
}

var Blockchain []Block // 체인 선언

type Message struct {
	BPM int
}

var mutex = &sync.Mutex{}

func Start() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal(err)
	}

	t := time.Now()
	genesisBlock := Block{}
	genesisBlock = Block{0, t.String(), 0, calculateHash(genesisBlock), ""} // 첫 블록 생성
	Blockchain = append(Blockchain, genesisBlock)
	spew.Dump(genesisBlock)

	log.Fatal(run())
}

func run() error {
	mux := makeMuxRouter()
	httpPort := os.Getenv("PORT")
	log.Println("HTTP Server Listening on port :", httpPort)
	s := &http.Server{
		Addr:           ":" + httpPort,
		Handler:        mux,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	if err := s.ListenAndServe(); err != nil {
		return err
	}

	return nil
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

func generateBlock(oldBlock Block, BPM int) Block { // BPM을 입력받아 블록 생성
	var newBlock Block

	t := time.Now()

	newBlock.Index = oldBlock.Index + 1
	newBlock.Timestamp = t.String()
	newBlock.BPM = BPM
	newBlock.PrevHash = oldBlock.Hash
	newBlock.Hash = calculateHash(newBlock)

	return newBlock
}

// fork(분기) 되었을 때, 어느 블록들이 신뢰성을 띄는 지 비교 -> 51% 공격에 무력화 될 수 있음
func replaceChain(newBlocks []Block) {
	if len(newBlocks) > len(Blockchain) {
		Blockchain = newBlocks
	}
}

func makeMuxRouter() http.Handler { // 라우터 설정
	muxRouter := mux.NewRouter()
	muxRouter.HandleFunc("/", handleGetBlockchain).Methods("get")
	muxRouter.HandleFunc("/", handleWriteBlock).Methods("POST")
	return muxRouter
}

// GET 메소드로 조회되었을 때, 웹뷰에 json으로 가공된 블록 정보 표시
func handleGetBlockchain(w http.ResponseWriter, r *http.Request) {
	bytes, err := json.MarshalIndent(Blockchain, "" /* prefix */, "  " /* indent */)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	io.WriteString(w, string(bytes))
}

// POST 메소드로 네트워크에 요청하면,
func handleWriteBlock(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var msg Message

	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		respondWithJSON(w, r, http.StatusBadRequest, r.Body)
		return
	}
	defer r.Body.Close()

	mutex.Lock()
	prevBlock := Blockchain[len(Blockchain)-1]
	newBlock := generateBlock(prevBlock, msg.BPM)

	if isBlockValid(newBlock, prevBlock) {
		Blockchain = append(Blockchain, newBlock)
		spew.Dump(Blockchain)
	}
	mutex.Unlock()

	respondWithJSON(w, r, http.StatusCreated, newBlock)
}

// 생성한 블록의 정보를 json으로 클라이언트에 response
func respondWithJSON(w http.ResponseWriter, r *http.Request, code int, payload interface{}) {
	response, err := json.MarshalIndent(payload, "", "  ")

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("HTTP 500: Internal Server Error"))
		return
	}

	w.WriteHeader(code)
	w.Write(response)
}
