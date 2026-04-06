package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/24aysh/toll-calc/types"
	"github.com/gorilla/websocket"
)

type wshandler struct {
}

type DataReceiver struct {
	msgch chan types.OBUData
	conn  *websocket.Conn
}

func main() {
	recv := NewDataReceiver()
	http.HandleFunc("/ws", recv.handleWS)
	http.ListenAndServe(":30000", nil)

}

func (dr *DataReceiver) handleWS(w http.ResponseWriter, r *http.Request) {
	u := websocket.Upgrader{
		ReadBufferSize:  1028,
		WriteBufferSize: 1028,
	}

	conn, err := u.Upgrade(w, r, nil)
	if err != nil {
		log.Fatal(err)
	}
	dr.conn = conn

	go dr.wsReceiveLoop()
}

func NewDataReceiver() *DataReceiver {
	return &DataReceiver{
		msgch: make(chan types.OBUData, 128),
	}
}

func (dr *DataReceiver) wsReceiveLoop() {
	fmt.Println("New OBU Connected")
	for {
		var data types.OBUData
		if err := dr.conn.ReadJSON(&data); err != nil {
			log.Println("read error :", err)
			continue
		}
		fmt.Printf("Received data from [%d] :: <lat %.2f, long %.2f>", data.OBUID, data.Lat, data.Lon)
		dr.msgch <- data
	}
}
