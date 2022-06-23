package main

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

func main() {
	t := time.Now()
	genesisBlock := Block{}
	genesisBlock = Block{0, t.String(), 0, calculateHash(genesisBlock), ""}

	Blockchain = append(Blockchain, genesisBlock)

	listenF := flag.Int("l", 0, "wait for incoming connections")
	target := flag.String("d", "", "target peer to dial")
	secio := flag.Bool("secio", false, "enable secio")
	seed := flag.Int64("seed", 0, "set random seed for id generation")
	flag.Parse()

	if *listenF == 0 {
		log.Fatal("Please provide a port to bind on with -l")
	}

	ha, err := makeBasicHost(*listenF, *secio, *seed)
	if err != nil {
		log.Fatal(err)
	}

	if *target == "" {
		log.Println("listening for connections")
		ha.SetStreamHandler("/p2p/1.0.0", handleStream)

		select {}
	} else {
		ha.SetStreamHandler("/p2p/1.0.0", handleStream)

		ipfsaddr, err := ma.NewMultiaddr(*target)
		if err != nil {
			log.Fatalln(err)
		}

		pid, err := ipfsaddr.ValueForProtocol(ma.P_IPFS)
		if err != nil {
			log.Fatalln(err)
		}

		peerid, err := peer.Decode(pid)
		if err != nil {
			log.Fatalln(err)
		}

		targetPeerAddr, _ := ma.NewMultiaddr(
			fmt.Sprintf("/ipfs/%s", peer.Encode(peerid)))
		targetAddr := ipfsaddr.Decapsulate(targetPeerAddr)

		ha.Peerstore().AddAddr(peerid, targetAddr, peerstore.PermanentAddrTTL)

		log.Println("opening stream")
		s, err := ha.NewStream(context.Background(), peerid, "/p2p/1.0.0")
		if err != nil {
			log.Fatalln(err)
		}

		rw := bufio.NewReadWriter(bufio.NewReader(s), bufio.NewWriter(s))

		go writeData(rw)
		go readData(rw)

		select {}
	}
}

func handleStream(s net.Stream) { // 호스트가 들어오는 스트림 처리
	log.Println("Got a new stream!")

	rw := bufio.NewReadWriter(bufio.NewReader(s), bufio.NewWriter(s)) // 읽기 및 쓰기 모두 사용하기 위한 객체 선언

	go readData(rw)
	go writeData(rw)
}

func readData(rw *bufio.ReadWriter) { // 다른 노드로부터 값(블록체인)을 읽어오는 함수
	for {
		str, err := rw.ReadString('\n') // 158줄 데이터 받아옴
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
			fmt.Printf("받아왔다\n%s", chain, len(chain), len(Blockchain))
			if len(chain) > len(Blockchain) { // 들어오는 체인이 기존 블록보다 길면 최신 네트워크 상태로 변경
				Blockchain = chain
				bytes, err := json.MarshalIndent(Blockchain, "", "  ")
				if err != nil {
					log.Fatal(err)
				}
				fmt.Printf("\n\x1b[32m%s\x1b[0m\n> ", string(bytes)) // 호스트 콘솔에 색상으로 블록체인 출력
			}
			mutex.Unlock()
		}
	}
}

func writeData(rw *bufio.ReadWriter) { // 다른 노드에 값(블록체인)을 전송하는 함수
	prev, _ := json.Marshal(Blockchain)

	go func() {
		for { // 5초마다 현재 블록체인을 모든 노드에게 보여줌
			time.Sleep(5 * time.Second)

			mutex.Lock()
			curr, err := json.Marshal(Blockchain)
			fmt.Printf("\n5초 지났으니 전송한다\n%s\n", curr)
			if err != nil {
				log.Print(err)
			}
			mutex.Unlock()

			mutex.Lock()
			if bytes.Compare(prev, curr) != 0 {
				rw.WriteString(fmt.Sprintf("%s\n", string(curr))) // 122줄로 이동
				rw.Flush()                                        // 연결된 모든 노드에 블록체인 전송
				prev = curr
			}
			mutex.Unlock()

		}
	}()

	stdReader := bufio.NewReader(os.Stdin)

	for {
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
		rw.WriteString(fmt.Sprintf("%s\n", string(bytes))) // 122번째 줄로 이동
		rw.Flush()                                         // 연결된 모든 노드에 블록체인 정보 전송
		mutex.Unlock()
	}

}

// 서버에서 수신 대기하며, 임의의 피어로 송수신할 p2p 호스트 생성
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

	file, _ := os.Executable()
	if secio {
		log.Printf("Now run \"%s -l %d -d %s -secio\" on a different terminal\n", file, listenPort+1, fullAddr)
	} else {
		log.Printf("Now run \"%s -l %d -d %s\" on a different terminal\n", file, listenPort+1, fullAddr)
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