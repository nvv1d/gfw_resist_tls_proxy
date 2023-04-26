package main

import (
"flag"
"fmt"
"io"
"log"
"net"
"os"
"strconv"
"strings"
"sync"
"time"
)

var (
listenPort       int
cloudflareIP     string
cloudflarePort   int
lFragment        int
fragmentSleep    time.Duration
socketTimeout    time.Duration
firstTimeSleep   time.Duration
acceptTimeSleep  time.Duration
)

func init() {
flag.IntVar(&listenPort, "listenPort", 8080, "Listen port")
flag.StringVar(&cloudflareIP, "cloudflareIP", "1.1.1.1", "Cloudflare IP")
flag.IntVar(&cloudflarePort, "cloudflarePort", 80, "Cloudflare port")
flag.IntVar(&lFragment, "lFragment", 1024, "Fragment length")
flag.DurationVar(&fragmentSleep, "fragmentSleep", 10*time.Millisecond, "Fragment sleep duration")
flag.DurationVar(&socketTimeout, "socketTimeout", 10*time.Second, "Socket timeout")
flag.DurationVar(&firstTimeSleep, "firstTimeSleep", 10*time.Millisecond, "First time sleep duration")
flag.DurationVar(&acceptTimeSleep, "acceptTimeSleep", 10*time.Millisecond, "Accept time sleep duration")
}

func sendFragmentedData(data []byte, conn net.Conn) {
for i := 0; i < len(data); i += lFragment {
end := i + lFragment
if end > len(data) {
end = len(data)
}
fragment := data[i:end]
conn.Write(fragment)
time.Sleep(fragmentSleep)
}
}

func handleUpstream(clientConn net.Conn, wg *sync.WaitGroup) {
defer wg.Done()
defer clientConn.Close()

firstFlag := true
backendConn, err := net.DialTimeout("tcp", net.JoinHostPort(cloudflareIP, strconv.Itoa(cloudflarePort)), socketTimeout)
if err != nil {
log.Printf("[UPSTREAM] %v", err)
return
}
defer backendConn.Close()

for {
if firstFlag {
firstFlag = false
time.Sleep(firstTimeSleep)
data := make([]byte, 16384)
n, err := clientConn.Read(data)
if err != nil {
if err != io.EOF {
log.Printf("[UPSTREAM] %v", err)
}
return
}
data = data[:n]
sendFragmentedData(data, backendConn)
} else {
data := make([]byte, 4096)
n, err := clientConn.Read(data)
if err != nil {
if err != io.EOF {
log.Printf("[UPSTREAM] %v", err)
}
return
}
data = data[:n]
backendConn.Write(data)
}
}
}

func handleDownstream(clientConn, backendConn net.Conn, wg *sync.WaitGroup) {
defer wg.Done()

for {
data := make([]byte, 4096)
n, err := backendConn.Read(data)
if err != nil {
if err != io.EOF {
log.Printf("[DOWNSTREAM] %v", err)
}
return
}
data = data[:n]
clientConn.Write(data)
}
}

func main() {
flag.Parse()

listenAddr := net.JoinHostPort("127.0.0.1", strconv.Itoa(listenPort))
ln, err := net.Listen("tcp", listenAddr)
if err != nil {
log.Fatalf("Error listening on %s: %v", listenAddr, err)
}
defer ln.Close()

fmt.Printf("Now listening at: %s, forwarding to %s:%d\n", listenAddr, cloudflareIP, cloudflarePort)

for {
clientConn, err := ln.Accept()
if err != nil {
log.Printf("Error accepting connection: %v", err)
continue
}
clientConn.SetDeadline(time.Now().Add(socketTimeout))
time.Sleep(acceptTimeSleep)

var wg sync.WaitGroup
wg.Add(2)
go handleUpstream(clientConn, &wg)
go handleDownstream(clientConn, backendConn, &wg)
wg.Wait()
}
}
