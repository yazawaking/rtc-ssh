package main

import (
	"net/url"
	"os"
	"fmt"
	"log"
	"flag"
	"os/signal"
	"time"
	"path/filepath"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
    ini "github.com/pierrec/go-ini"
)


type Config struct {
	Uuid     string  `ini:"uuid,identify"`
	Host     string  `ini:"host,ssh"`
	Port     int     `ini:"port,ssh"`
}

type Session struct {
	Type string   `json:"type"`
	Sdp  string   `json:"sdp"`	
	Error string  `json:"error"`
}


func reconnect(closeWS <-chan struct{}, query string) *websocket.Conn {
	var u url.URL
	var ws *websocket.Conn
	var err error
	
	u = url.URL{Scheme: "wss", Host: "sqs.io", Path: "/signal", RawQuery: query}
	for {
		ws, _, err = websocket.DefaultDialer.Dial(u.String(), nil)
		if err != nil {
			log.Println(err)
			time.Sleep(30 * time.Second)
			continue
		}
		break
	}

	lastResponse := time.Now()
	ws.SetPongHandler(func(msg string) error {
		lastResponse = time.Now()
		return nil
	})
		
	go func() {
		for {
			err := ws.WriteMessage(websocket.PingMessage, nil)
			if err != nil {
				log.Println(err)
				return 
			}   
			time.Sleep((120 * time.Second)/2)
			if(time.Now().Sub(lastResponse) > (120 * time.Second)) {
				log.Println("Signaling server close connection")
				ws.Close()
				return
			}
		}
	}()
	
	return ws
}


func main() {
	log.SetFlags(0)
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	
	var executablePath string
	var conf Config
	save := false
	
	ex, err := os.Executable()
    	check(err)
	executablePath = filepath.Dir(ex)
	
	newkey := flag.Bool("newkey", false, "new generate key uuid of connection")
	getkey := flag.Bool("getkey", false, "display key uuid")
	setport := flag.Int("port", defaultPort, "port SSH server connection")
	sethost := flag.String("host", defaultHost, "address host SSH server connection")
	flag.Parse()
	
	loadConf := func(){
		file, err := os.Open(executablePath + "/config.ini") 
		if err == nil {
			err = ini.Decode(file, &conf)
			check(err)
		}
		if os.IsNotExist(err) && !*newkey {
			log.Println("File config.ini not found, using option '-newkey'")
			os.Exit(0)
		} 
		defer file.Close()	
	
	}
	saveConf := func(){
		if conf.Host == "" { conf.Host = defaultHost }
		if conf.Port == 0 { conf.Port = defaultPort }
		file, err := os.Create(executablePath + "/config.ini")
		check(err)
		defer file.Close()
		err = ini.Encode(file, &conf)
		check(err)
	
	}	
	
	loadConf()
	if *setport != defaultPort {
		conf.Port = *setport 
		save = true
	}
	if *sethost != defaultHost {
		conf.Host = *sethost 
		save = true
	}
	if *newkey {
		conf.Uuid = uuid.New().String()
		save = true
		fmt.Println("uuid:", conf.Uuid)	
	} 
	if save { saveConf() }
	if *getkey { 
		fmt.Println("uuid:", conf.Uuid)
	}
	
	done := make(chan struct{})
	closeWS := make(chan struct{})
	var ws *websocket.Conn	
	go func() {
		for {
			select {
			case <-done:
				return
			case <-interrupt:
				// Cleanly close the connection by sending a close message and then
				// waiting (with timeout) for the server to close the connection.
				fmt.Println("interrupt")
				err := ws.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
				if err != nil {
					log.Println("write close:", err)
				}
				select {
					case <-done:
					case <-time.After(time.Second):
				}
				os.Exit(0)
			}
		}
	}()
	
	for {
		query := "localUser=" + conf.Uuid
		ws = reconnect(closeWS, query)
		hub(ws, conf)
		time.Sleep(30 * time.Second)
		log.Println("Reconnect with the signaling server")
	}


}



func hub(ws *websocket.Conn, conf Config) {
	var msg Session
	for {
		err := ws.ReadJSON(&msg)
		if err != nil {
			_,ok:= err.(*websocket.CloseError) 
			if !ok {log.Println("websocket", err)}
			break
		} 
		err = interpreter(ws, msg, conf)
		if err != nil {log.Println(err)}
	}
}

func check(e error) {
    if e != nil {
        log.Println(e)
    }
}
