package main

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"os"
	"regexp"
	"strconv"
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

	req, err := parseHttpRequest(conn)
	if err != nil {
		fmt.Println("error writing 404 response", err.Error())
	}

	pathRegex := regexp.MustCompile("/echo/(.+)")

	matches := pathRegex.FindAllStringSubmatch(req.URL, -1)
	if len(matches) == 0 {
		err = writeResponse(conn, &response{
			Status:     404,
			StatusText: "Not Found",
		})
		if err != nil {
			fmt.Println("error writing response")
			os.Exit(1)
		}
		return
	}

	str := matches[0][1]

	err = writeResponse(conn, &response{
		Status:     200,
		StatusText: "OK",
		Headers: map[string]string{
			"Content-Type":   "text/plain",
			"Content-Length": strconv.FormatInt(int64(len(str)), 10),
		},
		Body: []byte(str),
	})
	if err != nil {
		fmt.Println("error writing response")
		os.Exit(1)
	}
}

type request struct {
	Method  string
	URL     string
	Headers map[string]string
}

type response struct {
	Status     int
	StatusText string
	Headers    map[string]string
	Body       []byte
}

func (r *response) Content() []byte {
	buffer := bytes.NewBuffer([]byte{})

	fmt.Fprintf(buffer, "HTTP/1.1 %d %s\r\n", r.Status, r.StatusText)

	for key, value := range r.Headers {
		fmt.Fprintf(buffer, "%s: %s\r\n", key, value)
	}
	fmt.Fprintf(buffer, "\r\n")

	if r.Body != nil && len(r.Body) > 0 {
		buffer.Write(r.Body)
	}

	return buffer.Bytes()
}

func writeResponse(conn net.Conn, resp *response) error {
	_, err := conn.Write(resp.Content())
	return err
}

func parseHttpRequest(conn net.Conn) (*request, error) {
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
			return &request{
				Method: strings.ToLower(parts[0]),
				URL:    parts[1],
			}, nil
		}
	}

	return nil, errors.New("invalid http request")
}
