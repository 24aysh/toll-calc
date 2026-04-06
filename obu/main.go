package main

import (
	"log"
	"math"
	"math/rand"
	"time"

	"github.com/24aysh/toll-calc/types"
	"github.com/gorilla/websocket"
)

const wsEndpoint = "ws://127.0.0.1:30000/ws"
const sendInterval = 5

func genOBUIDS(n int) []int {
	ids := make([]int, n)
	for i := 0; i < n; i++ {
		ids[i] = rand.Intn(math.MaxInt)
	}
	return ids
}

func genLocation() (float64, float64) {
	return genCoord(), genCoord()
}

func genCoord() float64 {
	n := float64(rand.Intn(100))
	f := rand.Float64()
	return n + f
}

func main() {
	obuIDs := genOBUIDS(20)

	conn, _, err := websocket.DefaultDialer.Dial(wsEndpoint, nil)
	if err != nil {
		log.Fatal(err)
	}
	for {
		for i := 0; i < len(obuIDs); i++ {
			lat, long := genLocation()
			data := types.OBUData{
				OBUID: obuIDs[i],
				Lat:   lat,
				Lon:   long,
			}

			if err = conn.WriteJSON(data); err != nil {
				log.Fatal(err)
			}
		}

		time.Sleep(sendInterval * time.Second)
	}
}
