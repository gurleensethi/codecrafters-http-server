package main

import (
	"fmt"
	"net"
	"os"
	"strings"
)

func main() {
	// You can use print statements as follows for debugging, they'll be visible when running tests.
	fmt.Println("Logs from your program will appear here!")

	l, err := net.Listen("tcp", "0.0.0.0:4221")
	if err != nil {
		fmt.Println("Failed to bind to port 4221", err.Error())
		os.Exit(1)
	}

	conn, err := l.Accept()
	if err != nil {
		fmt.Println("Error accepting connection: ", err.Error())
		os.Exit(1)
	}

	defer conn.Close()

	buffer := make([]byte, 1024)

	n, err := conn.Read(buffer)
	if err != nil {
		fmt.Println("error reading conn: ", err.Error())
		os.Exit(1)
	}

	var statusLine []byte

	for i, c := range buffer[:n] {
		// found first CRLF
		if c == '\r' && i+1 < n && buffer[i+1] == '\n' {
			statusLine = buffer[:i]
			break
		}
	}

	if statusLine != nil {
		parts := strings.Split(string(statusLine), " ")
		if len(parts) == 3 {
			if parts[0] == "GET" && parts[1] == "/" {
				fmt.Println("Found", parts)
				_, err := conn.Write([]byte("HTTP/1.1 200 OK\r\n\r\n"))
				if err != nil {
					fmt.Println("error writing response", err.Error())
				}

				return
			}
		}
	}

	fmt.Println("not found")
	_, err = conn.Write([]byte("HTTP/1.1 404 Not Found\r\n\r\n"))
	if err != nil {
		fmt.Println("error writing 404 response", err.Error())
	}
}
