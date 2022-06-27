package P2P

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	mrand "math/rand"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/host"
	net "github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/peerstore"
	ma "github.com/multiformats/go-multiaddr"
)

type Block struct {
	Index     int
	Timestamp string
	BPM       int
	Hash      string
	PrevHash  string
}

var Blockchain []Block

var mutex = &sync.Mutex{}

func Start(port int, secio bool, target string /* 노드(호스트)에 접속하기 위한 피어 입력칸 */) {
	t := time.Now()
	genesisBlock := Block{}
	genesisBlock = Block{0, t.String(), 0, calculateHash(genesisBlock), ""}

	Blockchain = append(Blockchain, genesisBlock)

	seed := flag.Int64("seed", 0, "set random seed for id generation")
	flag.Parse() // flag 받은 값 세팅

	if port == 0 {
		log.Fatal("Please provide a port to bind on with -l")
	}

	ha, err := makeBasicHost(port, secio, *seed) // p2p 인스턴스 생성
	if err != nil {
		log.Fatal("makeBasicHost", err)
	}

	if target == "" { // target 플래그가 빈칸이면(= 첫 노드[호스트]라면) 연결 대기
		log.Println("listening for connections")
		ha.SetStreamHandler("/p2p/1.0.0", handleStream)

		select {}
	} else { // 노드에 접속한 피어라면, 다른 피어도 접속할 수 있도록 접속한 피어의 p2p 노드 오픈
		ha.SetStreamHandler("/p2p/1.0.0", handleStream)

		ipfsaddr, err := ma.NewMultiaddr(target)
		if err != nil {
			log.Fatalln("NewMultiaddr", err)
		}

		pid, err := ipfsaddr.ValueForProtocol(ma.P_IPFS) // pid 값 가져오기
		if err != nil {
			log.Fatalln("ValueForProtocol", err)
		}

		peerid, err := peer.Decode(pid)
		if err != nil {
			log.Fatalln(err)
		}

		targetPeerAddr, _ := ma.NewMultiaddr(
			fmt.Sprintf("/ipfs/%s", peer.Encode(peerid)))
		targetAddr := ipfsaddr.Decapsulate(targetPeerAddr) // pid값이 제거 된 '/ip4/사설아이피/tcp/포트넘버' String 할당

		ha.Peerstore().AddAddr(peerid, targetAddr, peerstore.PermanentAddrTTL) // 노드(타겟)의 주소를 피어 저장소에 저장

		log.Println("opening stream")
		s, err := ha.NewStream(context.Background(), peerid, "/p2p/1.0.0") // 피어와 노드의 통신 Stream 생성
		if err != nil {
			log.Fatalln("NewStream", err)
		}

		rw := bufio.NewReadWriter(bufio.NewReader(s), bufio.NewWriter(s)) // 읽기 및 쓰기 모두 사용하기 위한 객체 선언

		go writeData(rw)
		go readData(rw)

		select {}
	}
}

func handleStream(s net.Stream) { // 피어가 노드에 연결했을 때, 노드가 Stream을 처리하는 함수
	fmt.Println()
	log.Println("Got a new stream!")

	rw := bufio.NewReadWriter(bufio.NewReader(s), bufio.NewWriter(s)) // 읽기 및 쓰기 모두 사용하기 위한 객체 선언

	go readData(rw)
	go writeData(rw)
}

func readData(rw *bufio.ReadWriter) { // 다른 노드로부터 값(블록체인)을 읽어오는 함수
	for {
		str, err := rw.ReadString('\n') // 방금 블록이 추가된 블록체인을 읽어옴
		if err != nil {
			log.Fatal(err)
		}

		if str == "" {
			return
		}
		if str != "\n" {
			chain := make([]Block, 0)
			if err := json.Unmarshal([]byte(str), &chain); err != nil {
				log.Fatal(err)
			}

			mutex.Lock()
			if len(chain) > len(Blockchain) { // 들어오는 체인이 기존 블록보다 길면 최신 네트워크 상태로 변경
				Blockchain = chain
				bytes, err := json.MarshalIndent(Blockchain, "", "  ")
				if err != nil {
					log.Fatal(err)
				}
				fmt.Printf("\n\x1b[32m%s\x1b[0m\n> ", string(bytes)) // 호스트 콘솔에 색상으로 블록체인 출력
			} else {
				fmt.Print("\nApplying Blockchain Length...\n> ")
			}
			mutex.Unlock()
		}
	}
}

func writeData(rw *bufio.ReadWriter) { // 다른 노드에 값(블록체인)을 전송하는 함수
	prev, _ := json.Marshal(Blockchain)

	go func() {
		for {
			time.Sleep(30 * time.Second)

			mutex.Lock()
			curr, err := json.Marshal(Blockchain)
			if err != nil {
				log.Print(err)
			}
			mutex.Unlock()

			mutex.Lock()
			// 기존 블록체인과 30초 이후 조회한 블록체인이 다를 경우(추가되었을 경우가 해당)
			if !bytes.Equal(prev, curr) {
				rw.WriteString(fmt.Sprintf("%s\n", string(curr)))
				rw.Flush() // 연결된 모든 노드에 블록체인 전송
				prev = curr
			}
			mutex.Unlock()
		}
	}()

	stdReader := bufio.NewReader(os.Stdin) // 노드(호스트)가 피어로부터 입력을 받는 객체 선언

	for { // 블록 생성 반복문
		fmt.Print("> ")
		sendData, err := stdReader.ReadString('\n')
		if err != nil {
			log.Fatal(err)
		}

		sendData = strings.Replace(sendData, "\n", "", -1) // 개행 제거
		bpm, err := strconv.Atoi(sendData)
		if err != nil {
			log.Fatal(err)
		}
		newBlock, err := generateBlock(Blockchain[len(Blockchain)-1], bpm) // 블록 생성
		if err != nil {
			log.Fatal(err)
			break
		}

		if isBlockValid(newBlock, Blockchain[len(Blockchain)-1]) { // 블록이 유효한지 확인
			mutex.Lock()
			Blockchain = append(Blockchain, newBlock)
			mutex.Unlock()
		}

		bytes, err := json.Marshal(Blockchain)
		if err != nil {
			log.Println(err)
		}

		spew.Dump(Blockchain)

		mutex.Lock()
		rw.WriteString(fmt.Sprintf("%s\n", bytes)) // 개행으로 인하여 생성된 블록이 readWrite 함수로 이동
		rw.Flush()                                 // 연결된 모든 노드에 블록체인 정보 전송
		mutex.Unlock()
	}

}

// 임의의 피어로 블록체인을 송수신할 p2p 노드(호스트) 생성
func makeBasicHost(listenPort int, secio bool, randseed int64) (host.Host, error) {
	var r io.Reader
	if randseed == 0 { // 시드가 없을 경우, 서버의 로컬 환경에 따라 임의의 시드 생성
		r = rand.Reader
	} else {
		r = mrand.New(mrand.NewSource(randseed))
	}

	priv, _, err := crypto.GenerateKeyPairWithReader(crypto.RSA, 2048, r) // 호스트의 key pair 생성
	if err != nil {
		return nil, err
	}

	opts := []libp2p.Option{ // 다른 피어가 연결할 수 있는 주소 생성
		libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", listenPort)),
		libp2p.Identity(priv),
	}

	basicHost, err := libp2p.New(opts...) // p2p 인스턴스 생성
	if err != nil {
		return nil, err
	}

	hostAddr, _ := ma.NewMultiaddr(fmt.Sprintf("/ipfs/%s", basicHost.ID().Pretty())) // 피어가 접속할 수 있는 주소 생성

	addr := basicHost.Addrs()[0] // 0 : 사설 ip | 1 : 로컬 ip
	fullAddr := addr.Encapsulate(hostAddr)

	if secio {
		log.Printf("RUN\n\tport: %d\n\taddr: %s\n\tsecio: true\non a different terminal\n", listenPort+1, fullAddr)
	} else {
		log.Printf("RUN\n\tport: %d\n\taddr: %s\n\tsecio: false\non a different terminal\n", listenPort+1, fullAddr)
	}

	return basicHost, nil // p2p 인스턴스 반환
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

func generateBlock(oldBlock Block, BPM int) (Block, error) { // BPM을 입력받아 블록 생성
	if err := isBlockchainValid(); err != nil {
		return oldBlock, err
	}

	var newBlock Block

	t := time.Now()

	newBlock.Index = oldBlock.Index + 1
	newBlock.Timestamp = t.String()
	newBlock.BPM = BPM
	newBlock.PrevHash = oldBlock.Hash
	newBlock.Hash = calculateHash(newBlock)

	return newBlock, nil
}
