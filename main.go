package main

import (
	"fmt"
	"strconv"
	"strings"

	P2P "github.com/D0hwQ1/Blockchain-With-Go/p2p"
	"github.com/D0hwQ1/Blockchain-With-Go/pos"
	"github.com/D0hwQ1/Blockchain-With-Go/pow"
	"github.com/D0hwQ1/Blockchain-With-Go/tcp"
	"github.com/D0hwQ1/Blockchain-With-Go/web"
)

func main() {
	var port int

	for {
		fmt.Print("사용할 포트 입력: ")
		fmt.Scanf("%d", &port)

		if port != 0 {
			fmt.Println()
			break
		} else {
			fmt.Println("\n잘못된 포트 입력입니다.\n다시 입력해주세요\n")
			continue
		}
	}

	fmt.Println("web : 블록체인 웹서비스를 구동합니다.")
	fmt.Println("tcp : 블록체인 tcp통신을 구동합니다.")
	fmt.Println("pow : PoW 합의 알고리즘 방식의 블록체인 웹서비스를 구동합니다.")
	fmt.Println("pos : PoS 합의 알고리즘 방식의 블록체인 웹서비스를 구동합니다.")
	fmt.Println("p2p : 중앙 노드 기반의 블록체인 웹서비스를 구동합니다.\n\n")

	for {
		var name string

		fmt.Print("수행할 작업 입력: ")
		fmt.Scanf("%s", &name)
		fmt.Println()

		switch name {
		case "web":
			fmt.Println("링크: http://localhost:" + strconv.Itoa(port))
			fmt.Println("블록을 생성하실 때에는, 링크에 POST 방식으로 {BPM: value(num)}를 입력하시면 됩니다\n")
			web.Start(strconv.Itoa(port))
		case "tcp":
			fmt.Println("접속: nc localhost", port, "\n")
			tcp.Start(strconv.Itoa(port))
		case "pow":
			fmt.Println("링크: http://localhost:" + strconv.Itoa(port))
			fmt.Println("블록을 생성하실 때에는, 링크에 POST 방식으로 {BPM: value(num)}를 입력하시면 됩니다\n")
			pow.Start(strconv.Itoa(port))
		case "pos":
			fmt.Println("접속: nc localhost", port, "\n")
			pos.Start(strconv.Itoa(port))
		case "p2p":
			var yn string

			fmt.Print("노드 접속(y/n): ")
			fmt.Scanf("%s", &yn)

			if strings.ToLower(yn) == "y" || strings.ToLower(yn) == "yes" {
				fmt.Print("주소 입력: ")
				fmt.Scanf("%s", &yn)
				fmt.Println()

				P2P.Start(port, true, yn)
			} else {
				fmt.Print("secio 적용(y/n): ")
				fmt.Scanf("%s", &yn)
				fmt.Println()

				if strings.ToLower(yn) == "y" || strings.ToLower(yn) == "yes" {
					P2P.Start(port, true, "")
				} else {
					P2P.Start(port, false, "")
				}
			}

		default:
			fmt.Printf("'%s'은 잘못된 입력입니다.\n\n\n", name)
		}
	}
}
