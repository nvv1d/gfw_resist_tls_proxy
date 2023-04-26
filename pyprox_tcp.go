package main

import (
"fmt"
"io"
"math/rand"
"net"
"os"
"strconv"
"strings"
"sync"
"time"

"github.com/go-ini/ini"
)

var (
listenPort        int
cloudflareIPs     []string
cloudflarePort    int
lFragment         int
fragmentSleep     time.Duration
socketTimeout     time.Duration
firstTimeSleep    time.Duration
acceptTimeSleep   time.Duration
ipMutex           sync.Mutex
bufferPool        sync.Pool
)

func main() {
loadConfig("config.ini")

bufferPool = sync.Pool{
New: func() interface{} {
return make([]byte, 16384)
},
}

rand.Seed(time.Now().UnixNano())
listener, err := net.Listen("tcp", fmt.Sprintf(":%d", listenPort))
if err != nil {
fmt.Printf("Error listening on port %d: %v\n", listenPort, err)
os.Exit(1)
}
defer listener.Close()

fmt.Printf("Proxy server listening on 127.0.0.1:%d\n", listenPort)

for {
clientConn, err := listener.Accept()
if err != nil {
fmt.Printf("Error accepting connection: %v\n", err)
continue
}

go handleClient(clientConn)
}
}

func handleClient(clientConn net.Conn) {
defer clientConn.Close()
clientConn.SetDeadline(time.Now().Add(socketTimeout))
time.Sleep(acceptTimeSleep)

data := bufferPool.Get().([]byte)
defer bufferPool.Put(data)

n, err := clientConn.Read(data)
if err != nil {
fmt.Printf("Error reading from client: %v\n", err)
return
}

backendIP := getNextBackendIP()
fmt.Printf("Using backend IP: %s\n", backendIP)

backendConn, err := net.DialTimeout("tcp", net.JoinHostPort(backendIP, strconv.Itoa(cloudflarePort)), socketTimeout)
if err != nil {
fmt.Printf("Error connecting to backend: %v\n", err)
return
}
defer backendConn.Close()

go func() {
buf := bufferPool.Get().([]byte)
defer bufferPool.Put(buf)

io.CopyBuffer(clientConn, backendConn, buf)
}()

sendDataInFragments(data[:n], backendConn)
}

func sendDataInFragments(data []byte, conn net.Conn) {
for i := 0; i < len(data); i += lFragment {
end := i + lFragment
if end > len(data) {
end = len(data)
}
fragmentData := data[i:end]
conn.Write(fragmentData)
time.Sleep(fragmentSleep)
}
}

func getNextBackendIP() string {
ipMutex.Lock()
defer ipMutex.Unlock()

selectedIP := cloudflareIPs[rand.Intn(len(cloudflareIPs))]
cloudflareIPs = append(cloudflareIPs[1:], selectedIP)
return selectedIP
}

func loadConfig(configPath string) {
cfg, err := ini.Load(configPath)
if err != nil {
fmt.Printf("Error loading config file: %v\n", err)
os.Exit(1)
}

listenPort = cfg.Section("settings").Key("listen_PORT").MustInt(2500)
cloudflareIPs = strings.Split(cfg.Section("settings").Key("Cloudflare_IP").String(), ",")
for i := range cloudflareIPs {
cloudflareIPs[i] = strings.TrimSpace(cloudflareIPs[i])
}
cloudflarePort = cfg.Section("settings").Key("Cloudflare_port").MustInt(443)
lFragment = cfg.Section("settings").Key("L_fragment").MustInt(77)
fragmentSleep = time.Duration(cfg.Section("settings").Key("fragment_sleep").MustFloat64(0.2) * float64(time.Second))
socketTimeout = time.Duration(cfg.Section("settings").Key("my_socket_timeout").MustInt(60)) * time.Second
firstTimeSleep = time.Duration(cfg.Section("settings").Key("first_time_sleep").MustFloat64(0.01) * float64(time.Second))
acceptTimeSleep = time.Duration(cfg.Section("settings").Key("accept_time_sleep").MustFloat64(0.01) * float64(time.Second))
}
