package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
)

var cert tls.Certificate
var c *Config = &Config{}
var local = false

type Config struct {
	TLSPort       string `json:"tls_port"`
	HTTPPort      string `json:"http_port"`
	CertFile      string `json:"cert_file"`
	KeyFile       string `json:"key_file"`
	Host          string `json:"host"`
	RedisHost     string `json:"redis_host"`
	RedisPassword string `json:"redis_password"`
	RedisDB       int    `json:"redis_db"`
	RedisEnable   bool   `json:"redis_enable"`
}

type Priority struct {
	Weight    int `json:"weight"`
	DependsOn int `json:"depends_on"`
	Exclusive int `json:"exclusive"`
}

type GoAway struct {
	LastStreamID uint32
	ErrCode      uint32
	DebugData    []byte
}

type ParsedFrame struct {
	Type      string    `json:"frame_type,omitempty"`
	Stream    uint32    `json:"stream_id,omitempty"`
	Length    uint32    `json:"length,omitempty"`
	Payload   []byte    `json:"payload,omitempty"`
	Headers   []string  `json:"headers,omitempty"`
	Settings  []string  `json:"settings,omitempty"`
	Increment uint32    `json:"increment,omitempty"`
	Flags     []string  `json:"flags,omitempty"`
	Priority  *Priority `json:"priority,omitempty"`
	GoAway    *GoAway   `json:"goaway,omitempty"`
}

func init() {
	err := c.LoadFromFile()
	if err != nil {
		log.Fatal(err)
	}
}

func (c *Config) MakeDefault() {
	c.Host = ""
	c.TLSPort = "443"
	c.HTTPPort = "80"
	c.CertFile = "certs/chain.pem"
	c.KeyFile = "certs/key.pem"
}

func (c *Config) WriteToFile(file string) error {
	j, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		log.Println("Error marshalling config", err)
		return err
	}
	return WriteToFile(file, j)
}

func (c *Config) LoadFromFile() error {
	data, err := ReadFile("config.json")
	fmt.Println(string(data))
	if err != nil {
		fmt.Println("No config file found: generating one", err)
		c.MakeDefault()
		return c.WriteToFile("config.json")
	}
	var tmp Config
	err = json.Unmarshal(data, &tmp)
	if err != nil {
		return err
	}

	c.Host = tmp.Host
	c.TLSPort = tmp.TLSPort
	c.HTTPPort = tmp.HTTPPort
	c.CertFile = tmp.CertFile
	c.KeyFile = tmp.KeyFile
	c.RedisHost = tmp.RedisHost
	c.RedisPassword = tmp.RedisPassword
	c.RedisDB = tmp.RedisDB
	c.RedisEnable = tmp.RedisEnable

	return nil
}

func redirect(w http.ResponseWriter, r *http.Request) {
	/*
		target := "https://" + r.Host + r.URL.Path
		if len(r.URL.RawQuery) > 0 {
			target += "?" + r.URL.RawQuery
		}

		http.Redirect(w, r, target, http.StatusTemporaryRedirect)
	*/
	w.Write([]byte("OK"))
}

func StartRedirectServer(host, port string) {
	// Starts an HTTP server on port 80 that redirects to the HTTPS server on port 443

	local = host == "" && port != "443"

	log.Println("Starting Redirect Server")
	log.Println("Listening on", host+":"+port)

	http.HandleFunc("/", redirect)
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	})
	err := http.ListenAndServe(host+":"+port, nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}

func StartRedisServer(host, pass string, db int) {
	log.Println("Starting Redis Server")
	log.Println("Listening on", host)

	// Create a Redis client
	client := redis.NewClient(&redis.Options{
		Addr:     host, // Redis server address
		Password: pass, // No password
		DB:       db,   // Default DB

	})

	// Close the connection when the main function exits
	defer func() {
		if err := client.Close(); err != nil {
			log.Fatal(err)
		}
	}()
}

func main() {

	log.Println("Starting server...")
	log.Println("Listening on " + c.Host + ":" + c.TLSPort)

	// Load the TLS certificates
	var err error
	cert, err = tls.LoadX509KeyPair(c.CertFile, c.KeyFile)
	if err != nil {
		log.Fatal("Error loading TLS certificates", err)
	}
	// Create a TLS configuration
	config := tls.Config{
		ServerName: c.Host,
		NextProtos: []string{
			"h2",
		},
		Certificates: []tls.Certificate{cert},
	}

	listener, err := tls.Listen("tcp", c.Host+":"+c.TLSPort, &config)

	if err != nil {
		log.Fatal("Error starting tcp listener", err)
	}

	defer listener.Close()
	go StartRedirectServer(c.Host, c.HTTPPort)

	if c.RedisEnable {
		go StartRedisServer(c.RedisHost, c.RedisPassword, c.RedisDB)
	}

	for {
		conn, err := listener.Accept()

		if err != nil {
			log.Println("Error accepting connection", err)
		}

		go func() {
			success := timeoutHandleTLSConnection(conn)
			if !success {
				conn.Close()
			}
		}()
	}

}

func timeoutHandleTLSConnection(conn net.Conn) bool {
	result := make(chan bool)
	go func() {
		result <- HandleTLSConnection(conn)
	}()
	select {
	case <-time.After(15 * time.Second):
		return false
	case tmp := <-result:
		return tmp
	}
}
