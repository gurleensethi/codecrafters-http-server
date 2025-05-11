package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
)

var (
	directoryFlag = flag.String("directory", "", "")
)

func main() {
	flag.Parse()

	// You can use print statements as follows for debugging, they'll be visible when running tests.
	fmt.Println("Logs from your program will appear here!")

	r := router{
		routes: make([]route, 0),
	}

	r.AddRoute("GET", "^/echo/(.+)$", func(r *request, s []string) *response {
		return &response{
			Status:     200,
			StatusText: "OK",
			Headers: map[string]string{
				"Content-Type":   "text/plain",
				"Content-Length": strconv.FormatInt(int64(len(s[0])), 10),
			},
			Body: bytes.NewBuffer([]byte(s[0])),
		}
	})

	r.AddRoute("GET", "^/$", func(r *request, pathMatches []string) *response {
		return &response{
			Status:     200,
			StatusText: "OK",
		}
	})

	r.AddRoute("GET", "^/user-agent$", func(r *request, s []string) *response {
		body := r.Headers["user-agent"]

		return &response{
			Status:     200,
			StatusText: "OK",
			Body:       bytes.NewBuffer([]byte(body)),
			Headers: map[string]string{
				"Content-Type":   "text/plain",
				"Content-Length": strconv.FormatInt(int64(len(body)), 10),
			},
		}
	})

	r.AddRoute("GET", "^/files/(.+)$", func(r *request, s []string) *response {
		filename := path.Join(*directoryFlag, s[0])

		file, err := os.Open(filename)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return &response{
					Status:     404,
					StatusText: "Not Found",
				}
			}

			return &response{
				Status:     500,
				StatusText: "Internal Error",
			}
		}

		stat, _ := file.Stat()

		return &response{
			Status:     200,
			StatusText: "OK",
			Body:       file,
			Headers: map[string]string{
				"Content-Length": strconv.FormatInt(stat.Size(), 10),
				"Content-Type":   "application/octet-stream",
			},
		}
	})

	r.AddRoute("POST", "^/files/(.+)$", func(r *request, s []string) *response {
		fmt.Println(string(r.Body))

		return &response{
			Status:     201,
			StatusText: "Created",
		}
	})

	srv := server{
		router: r,
	}
	srv.Start()
}

type server struct {
	router router
}

func (s *server) Start() {
	fmt.Println("starting server...")

	l, err := net.Listen("tcp", "0.0.0.0:4221")
	if err != nil {
		fmt.Println("Failed to bind to port 4221", err.Error())
		os.Exit(1)
	}

	for {
		conn, err := l.Accept()
		if err != nil {
			fmt.Println("Error accepting connection: ", err.Error())
		}

		go func(conn net.Conn) {
			defer conn.Close()

			req, err := parseHttpRequest(conn)
			if err != nil {
				fmt.Println("error parsing http request", err.Error())
				os.Exit(1)
			}

			err = s.router.HandlerRequest(conn, req)
			if err != nil {
				fmt.Println("error handling request", err.Error())
			}
		}(conn)
	}
}

type route struct {
	method  string
	matcher *regexp.Regexp
	handler func(*request, []string) *response
}

type router struct {
	routes []route
}

func (r *router) AddRoute(method, path string, handler func(*request, []string) *response) {
	r.routes = append(r.routes, route{
		method:  strings.ToLower(method),
		matcher: regexp.MustCompile(path),
		handler: handler,
	})
}

func (r *router) HandlerRequest(conn net.Conn, req *request) error {
	for _, route := range r.routes {
		if req.Method != route.method {
			continue
		}

		matches := route.matcher.FindAllStringSubmatch(req.URL, -1)

		if len(matches) > 0 {
			var matchedPaths []string

			for _, m := range matches {
				if len(m) > 1 {
					matchedPaths = append(matchedPaths, m[1])
				}
			}

			resp := route.handler(req, matchedPaths)

			return resp.WriteToConn(conn)
		}
	}

	return (&response{
		Status:     404,
		StatusText: "Not Found",
	}).WriteToConn(conn)
}

type request struct {
	Method  string
	URL     string
	Headers map[string]string
	Body    []byte
}

type response struct {
	Status     int
	StatusText string
	Headers    map[string]string
	Body       io.Reader
}

func (r *response) WriteToConn(conn net.Conn) error {
	fmt.Fprintf(conn, "HTTP/1.1 %d %s\r\n", r.Status, r.StatusText)

	for key, value := range r.Headers {
		fmt.Fprintf(conn, "%s: %s\r\n", key, value)
	}
	fmt.Fprintf(conn, "\r\n")

	if r.Body != nil {
		_, err := io.Copy(conn, r.Body)
		if err != nil {
			return err
		}
	}

	return nil
}

func parseHttpRequest(conn net.Conn) (*request, error) {
	buffer := make([]byte, 1024)

	n, err := conn.Read(buffer)
	if err != nil {
		fmt.Println("error reading conn: ", err.Error())
		os.Exit(1)
	}

	type Section string

	var (
		SectionStatusLine Section = "status_line"
		SectionHeader     Section = "header"
		SectionBody       Section = "body"
	)

	var body []byte
	var contentLength int
	var statusLine []byte
	currentSection := SectionStatusLine
	lastCLRF := -1

	headers := make(map[string]string)

loop:
	for i, c := range buffer[:n] {
		isCLRF := c == '\r' && i+1 < n && buffer[i+1] == '\n'
		isContinuousCLRF := isCLRF && (i-2 == lastCLRF)

		// we just ended headers
		if isContinuousCLRF {
			currentSection = SectionBody
			continue
		}

		switch currentSection {
		case SectionStatusLine:
			if isCLRF {
				statusLine = buffer[:i]
				currentSection = SectionHeader
			}
		case SectionHeader:
			if isCLRF {
				header := buffer[lastCLRF+2 : i]

				var key string
				var value string

				index := strings.IndexRune(string(header), ':')
				if index == -1 {
					key = strings.ToLower(string(header))
				} else {
					key = strings.ToLower(string(header[:index]))
					value = strings.TrimSpace(string(header[index+1:]))
				}

				headers[key] = value

				if strings.ToLower(key) == "content-length" {
					contentLength, _ = strconv.Atoi(value)
				}
			}
		case SectionBody:
			body = buffer[i : i+contentLength+1]
			break loop
		}

		if isCLRF {
			lastCLRF = i
		}
	}

	parts := strings.Split(string(statusLine), " ")

	return &request{
		Method:  strings.ToLower(parts[0]),
		URL:     parts[1],
		Headers: headers,
		Body:    body,
	}, nil
}
