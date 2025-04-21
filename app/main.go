package main

import (
	"bytes"
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

	r := router{
		matchers: make([]*regexp.Regexp, 0),
		handlers: make([]func(*request, []string) *response, 0),
	}

	r.AddRoute("^/echo/(.+)$", func(r *request, s []string) *response {
		return &response{
			Status:     200,
			StatusText: "OK",
			Headers: map[string]string{
				"Content-Type":   "text/plain",
				"Content-Length": strconv.FormatInt(int64(len(s[0])), 10),
			},
			Body: []byte(s[0]),
		}
	})

	r.AddRoute("^/$", func(r *request, pathMatches []string) *response {
		return &response{
			Status:     200,
			StatusText: "OK",
		}
	})

	r.AddRoute("^/user-agent$", func(r *request, s []string) *response {
		return &response{
			Status:     200,
			StatusText: "OK",
			Body:       []byte(r.Headers["user-agent"]),
		}
	})

	req, err := parseHttpRequest(conn)
	if err != nil {
		fmt.Println("error parsing http request", err.Error())
		os.Exit(1)
	}

	err = r.HandlerRequest(conn, req)
	if err != nil {
		fmt.Println("error handling request", err.Error())
	}
}

type router struct {
	matchers []*regexp.Regexp
	handlers []func(*request, []string) *response
}

func (r *router) AddRoute(path string, handler func(*request, []string) *response) {
	r.matchers = append(r.matchers, regexp.MustCompile(path))
	r.handlers = append(r.handlers, handler)
}

func (r *router) HandlerRequest(conn net.Conn, req *request) error {
	for i, matcher := range r.matchers {
		matches := matcher.FindAllStringSubmatch(req.URL, -1)
		fmt.Println(matches)

		if len(matches) > 0 {
			var matchedPaths []string

			for _, m := range matches {
				if len(m) > 1 {
					matchedPaths = append(matchedPaths, m[1])
				}
			}

			handler := r.handlers[i]

			resp := handler(req, matchedPaths)

			_, err := conn.Write(resp.Content())
			return err
		}
	}

	_, err := conn.Write((&response{
		Status:     404,
		StatusText: "Not Found",
	}).Content())
	return err
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

	var statusLine []byte
	currentSection := SectionStatusLine
	lastCLRF := -1

	headers := make(map[string]string)

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
			}
		case SectionBody:
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
	}, nil
}
